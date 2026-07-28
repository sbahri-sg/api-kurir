package rajaongkir

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/emisell/api-kurir/internal/providercredentials"
	"github.com/emisell/api-kurir/internal/rates"
	"github.com/emisell/api-kurir/internal/tracking"
)

type CredentialResolver interface {
	ResolveProviderCredential(
		ctx context.Context,
		providerCode string,
	) (secret, credentialAlias string, dailyLimit int64, err error)
}

type dynamicClientFactory struct {
	baseURL  string
	timeout  time.Duration
	interval time.Duration
	mu       sync.Mutex
	clients  map[string]*Client
}

func newDynamicClientFactory(
	baseURL string,
	timeout, interval time.Duration,
) *dynamicClientFactory {
	return &dynamicClientFactory{
		baseURL: baseURL, timeout: timeout, interval: interval,
		clients: make(map[string]*Client),
	}
}

func (f *dynamicClientFactory) client(alias, secret string) (*Client, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if client := f.clients[alias]; client != nil {
		return client, nil
	}
	client, err := NewClient(
		f.baseURL,
		secret,
		NewPacedHTTPClient(f.timeout, f.interval),
	)
	if err != nil {
		return nil, err
	}
	f.clients[alias] = client
	return client, nil
}

type DynamicProvider struct {
	resolver    CredentialResolver
	clients     *dynamicClientFactory
	mappings    LocationMappingStore
	quota       rates.QuotaRepository
	snapshotTTL time.Duration
}

func NewDynamicProvider(
	resolver CredentialResolver,
	baseURL string,
	timeout, interval time.Duration,
	mappings LocationMappingStore,
	quota rates.QuotaRepository,
	snapshotTTL time.Duration,
) *DynamicProvider {
	return &DynamicProvider{
		resolver:    resolver,
		clients:     newDynamicClientFactory(baseURL, timeout, interval),
		mappings:    mappings,
		quota:       quota,
		snapshotTTL: snapshotTTL,
	}
}

func (p *DynamicProvider) Code() string {
	return "rajaongkir"
}

func (p *DynamicProvider) Quote(
	ctx context.Context,
	request rates.Request,
) ([]rates.ProviderQuote, error) {
	secret, alias, limit, err := p.resolver.ResolveProviderCredential(ctx, p.Code())
	if errors.Is(err, providercredentials.ErrNoActiveCredential) {
		return nil, rates.ErrRateNotAvailable
	}
	if errors.Is(err, providercredentials.ErrAllCredentialsExhausted) {
		return nil, rates.ErrProviderQuotaExhausted
	}
	if err != nil {
		return nil, err
	}
	client, err := p.clients.client(alias, secret)
	if err != nil {
		return nil, err
	}
	return NewProvider(
		client,
		p.mappings,
		p.quota,
		alias,
		limit,
		p.snapshotTTL,
	).Quote(ctx, request)
}

type CredentialValidator struct {
	clients *dynamicClientFactory
}

func NewCredentialValidator(
	baseURL string,
	timeout, interval time.Duration,
) *CredentialValidator {
	return &CredentialValidator{
		clients: newDynamicClientFactory(baseURL, timeout, interval),
	}
}

func (v *CredentialValidator) Validate(
	ctx context.Context,
	providerCode, secret string,
) error {
	if providerCode != "rajaongkir" {
		return providercredentials.ErrUnsupportedProvider
	}
	fingerprint := sha256.Sum256([]byte(secret))
	client, err := v.clients.client(
		"validation:"+hex.EncodeToString(fingerprint[:8]),
		secret,
	)
	if err != nil {
		return err
	}
	_, err = client.SearchDestinations(ctx, "Jakarta", 1, 0)
	return err
}

type DynamicTrackingAdapter struct {
	resolver     CredentialResolver
	clients      *dynamicClientFactory
	quota        rates.QuotaRepository
	recorder     ProviderCallRecorder
	courierCodes []string
}

func NewDynamicTrackingAdapter(
	resolver CredentialResolver,
	baseURL string,
	timeout, interval time.Duration,
	quota rates.QuotaRepository,
	recorder ProviderCallRecorder,
	courierCodes []string,
) *DynamicTrackingAdapter {
	if len(courierCodes) == 0 {
		courierCodes = defaultTrackingCouriers
	}
	return &DynamicTrackingAdapter{
		resolver:     resolver,
		clients:      newDynamicClientFactory(baseURL, timeout, interval),
		quota:        quota,
		recorder:     recorder,
		courierCodes: append([]string(nil), courierCodes...),
	}
}

func (a *DynamicTrackingAdapter) Code() string {
	return "rajaongkir"
}

func (a *DynamicTrackingAdapter) CourierCodes() []string {
	return append([]string(nil), a.courierCodes...)
}

func (a *DynamicTrackingAdapter) Track(
	ctx context.Context,
	request tracking.Request,
) (tracking.Result, error) {
	secret, alias, limit, err := a.resolver.ResolveProviderCredential(ctx, a.Code())
	if errors.Is(err, providercredentials.ErrNoActiveCredential) {
		return tracking.Result{}, tracking.ErrProviderUnavailable
	}
	if errors.Is(err, providercredentials.ErrAllCredentialsExhausted) {
		return tracking.Result{}, tracking.ErrProviderQuota
	}
	if err != nil {
		return tracking.Result{}, err
	}
	client, err := a.clients.client(alias, secret)
	if err != nil {
		return tracking.Result{}, err
	}
	return NewTrackingAdapter(
		client,
		a.quota,
		a.recorder,
		alias,
		limit,
		a.courierCodes,
	).Track(ctx, request)
}
