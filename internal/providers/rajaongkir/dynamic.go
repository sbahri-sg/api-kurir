package rajaongkir

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
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
	clients          *dynamicClientFactory
	deliveryBaseURLs map[string]string
	deliveryClient   *http.Client
}

func NewCredentialValidator(
	baseURL string,
	timeout, interval time.Duration,
	deliveryBaseURLs ...string,
) *CredentialValidator {
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	liveBaseURL := "https://api.collaborator.komerce.id"
	sandboxBaseURL := "https://api-sandbox.collaborator.komerce.id"
	if len(deliveryBaseURLs) > 0 && strings.TrimSpace(deliveryBaseURLs[0]) != "" {
		liveBaseURL = deliveryBaseURLs[0]
	}
	if len(deliveryBaseURLs) > 1 && strings.TrimSpace(deliveryBaseURLs[1]) != "" {
		sandboxBaseURL = deliveryBaseURLs[1]
	}
	return &CredentialValidator{
		clients: newDynamicClientFactory(baseURL, timeout, interval),
		deliveryBaseURLs: map[string]string{
			providercredentials.EnvironmentLive:    strings.TrimRight(strings.TrimSpace(liveBaseURL), "/"),
			providercredentials.EnvironmentSandbox: strings.TrimRight(strings.TrimSpace(sandboxBaseURL), "/"),
		},
		deliveryClient: &http.Client{Timeout: timeout},
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

func (v *CredentialValidator) ValidateCredentials(
	ctx context.Context,
	providerCode string,
	credentialType string,
	values map[string]string,
) error {
	if providerCode != "rajaongkir" {
		return providercredentials.ErrUnsupportedProvider
	}
	shippingKey := strings.TrimSpace(values["shipping_api_key"])
	if shippingKey == "" {
		shippingKey = strings.TrimSpace(values["api_key"])
	}
	deliveryKey := strings.TrimSpace(values["delivery_api_key"])
	if shippingKey == "" && deliveryKey == "" {
		return providercredentials.ErrInvalidSecret
	}
	if shippingKey != "" {
		if err := v.Validate(ctx, providerCode, shippingKey); err != nil {
			return fmt.Errorf("validate shipping_api_key: %w", err)
		}
	}
	if deliveryKey != "" {
		if err := v.validateDeliveryCredential(ctx, deliveryKey); err != nil {
			return fmt.Errorf("validate delivery_api_key: %w", err)
		}
	}
	return nil
}

func (v *CredentialValidator) validateDeliveryCredential(
	ctx context.Context,
	secret string,
) error {
	environment := providercredentials.ExecutionEnvironment(ctx)
	baseURL := v.deliveryBaseURLs[environment]
	if baseURL == "" || strings.TrimSpace(secret) == "" {
		return providercredentials.ErrInvalidSecret
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		baseURL+"/tariff/api/v1/destination/search?keyword=53131",
		nil,
	)
	if err != nil {
		return err
	}
	request.Header.Set("x-api-key", secret)
	request.Header.Set("Accept", "application/json")
	response, err := v.deliveryClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("Shipping Delivery returned HTTP %d", response.StatusCode)
	}
	return nil
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
