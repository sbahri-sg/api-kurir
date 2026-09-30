package hosted

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/emisell/api-kurir/internal/couriers"
	"github.com/emisell/api-kurir/internal/locations"
	"github.com/emisell/api-kurir/internal/providercredentials"
	"github.com/emisell/api-kurir/internal/rates"
)

var etdNumbers = regexp.MustCompile(`\d+`)

type RateCredentialResolver interface {
	ResolveProviderCredentialForCapability(
		ctx context.Context,
		providerCode, capability string,
	) (secret, alias string, dailyLimit int64, err error)
}

type LocationMappingResolver interface {
	ResolveProviderLocation(ctx context.Context, locationPublicID, providerCode string) (string, error)
}

type ProviderCallRecorder interface {
	RecordProviderAPICall(
		ctx context.Context,
		providerCode string,
		credentialAlias string,
		endpoint string,
		requestFingerprint string,
		httpStatus int,
		outcome string,
		duration time.Duration,
		quotaCost int,
		errorCode string,
	) error
}

type RateProvider struct {
	providerCode           string
	client                 *Client
	credentials            RateCredentialResolver
	mappings               LocationMappingResolver
	quota                  rates.QuotaRepository
	recorder               ProviderCallRecorder
	platformBypassCouriers map[string]struct{}
	snapshotTTL            time.Duration
	now                    func() time.Time
}

// WithPlatformBypassCouriers skips known slow or unsupported couriers only for
// the shared Emisell credential. The rate service then asks its fallback for
// those missing couriers. BYOK requests remain untouched.
func (p *RateProvider) WithPlatformBypassCouriers(codes []string) *RateProvider {
	p.platformBypassCouriers = make(map[string]struct{}, len(codes))
	for _, code := range codes {
		code = couriers.NormalizeCode(code)
		if code != "" {
			p.platformBypassCouriers[code] = struct{}{}
		}
	}
	return p
}

func NewRateProvider(
	providerCode string,
	client *Client,
	credentials RateCredentialResolver,
	mappings LocationMappingResolver,
	quota rates.QuotaRepository,
	snapshotTTL time.Duration,
	recorders ...ProviderCallRecorder,
) *RateProvider {
	provider := &RateProvider{
		providerCode: providerCode, client: client, credentials: credentials,
		mappings: mappings, quota: quota, snapshotTTL: snapshotTTL, now: time.Now,
	}
	if len(recorders) > 0 {
		provider.recorder = recorders[0]
	}
	return provider
}

func (p *RateProvider) Code() string { return p.providerCode }

func (p *RateProvider) Quote(ctx context.Context, request rates.Request) ([]rates.ProviderQuote, error) {
	if request.ProviderCredentialID == "" && len(p.platformBypassCouriers) > 0 {
		filtered := make([]string, 0, len(request.Couriers))
		for _, code := range request.Couriers {
			if _, bypass := p.platformBypassCouriers[couriers.NormalizeCode(code)]; !bypass {
				filtered = append(filtered, code)
			}
		}
		request.Couriers = filtered
		if len(request.Couriers) == 0 {
			return nil, rates.ErrRateNotAvailable
		}
	}
	secret, alias, limit, err := p.credentials.ResolveProviderCredentialForCapability(
		ctx, p.providerCode, "rates:read",
	)
	if errors.Is(err, providercredentials.ErrNoActiveCredential) {
		return nil, rates.ErrRateNotAvailable
	}
	if errors.Is(err, providercredentials.ErrAllCredentialsExhausted) {
		return nil, rates.ErrProviderQuotaExhausted
	}
	if err != nil {
		return nil, err
	}
	if p.quota == nil || p.client == nil || p.mappings == nil {
		return nil, rates.ErrProviderUnavailable
	}
	if err := p.quota.ConsumeProviderHit(ctx, p.providerCode, alias, limit); err != nil {
		return nil, err
	}
	origin, err := p.mappings.ResolveProviderLocation(ctx, request.Origin, p.providerCode)
	if err != nil {
		if errors.Is(err, locations.ErrProviderMappingNotFound) {
			return nil, rates.ErrProviderLocationMapping
		}
		return nil, err
	}
	destination, err := p.mappings.ResolveProviderLocation(ctx, request.Destination, p.providerCode)
	if err != nil {
		if errors.Is(err, locations.ErrProviderMappingNotFound) {
			return nil, rates.ErrProviderLocationMapping
		}
		return nil, err
	}
	input := map[string]any{
		"origin":         map[string]string{"district_id": origin},
		"destination":    map[string]string{"district_id": destination},
		"weight_grams":   request.ActualWeightGrams,
		"courier_codes":  request.Couriers,
		"service_groups": []string{"regular", "next_day", "economy", "cargo"},
		"price":          request.PriceFilter,
	}
	var response struct {
		Data struct {
			Quotes []struct {
				ProviderCode string `json:"provider_code"`
				CourierCode  string `json:"courier_code"`
				CourierName  string `json:"courier_name"`
				ServiceCode  string `json:"service_code"`
				ServiceName  string `json:"service_name"`
				ServiceGroup string `json:"service_group"`
				Price        int64  `json:"price"`
				ETD          string `json:"etd"`
			} `json:"quotes"`
		} `json:"data"`
	}
	fingerprint := hostedRateFingerprint(request)
	startedAt := p.now()
	if err := p.client.Do(ctx, http.MethodPost, "rates", "key", secret, input, &response); err != nil {
		mapped := mapRateError(err)
		p.recordRateCall(ctx, alias, fingerprint, p.now().Sub(startedAt), err, mapped)
		return nil, mapped
	}
	p.recordRateCall(ctx, alias, fingerprint, p.now().Sub(startedAt), nil, nil)
	fetchedAt := p.now().UTC()
	expiresAt := fetchedAt.Add(p.snapshotTTL)
	quotes := make([]rates.ProviderQuote, 0, len(response.Data.Quotes))
	for _, quote := range response.Data.Quotes {
		minimum, maximum := parseETD(quote.ETD)
		quotes = append(quotes, rates.ProviderQuote{
			ProviderCode: p.providerCode, CourierCode: couriers.NormalizeCode(quote.CourierCode),
			CourierName: quote.CourierName, ServiceCode: quote.ServiceCode,
			ServiceName: quote.ServiceName, Description: quote.ServiceName,
			ServiceGroup: quote.ServiceGroup, Cost: quote.Price,
			ETDMinDays: minimum, ETDMaxDays: maximum,
			VerificationStatus: "observed", FetchedAt: fetchedAt, ExpiresAt: expiresAt,
		})
	}
	if len(quotes) == 0 {
		return nil, rates.ErrRateNotAvailable
	}
	return quotes, nil
}

func (p *RateProvider) recordRateCall(
	ctx context.Context,
	alias string,
	fingerprint string,
	duration time.Duration,
	providerErr error,
	mappedErr error,
) {
	if p.recorder == nil {
		return
	}
	// The upstream request may have exhausted or cancelled its context. Keep a
	// short independent audit window so provider outages remain observable.
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 500*time.Millisecond)
	defer cancel()
	status := http.StatusOK
	outcome := "success"
	errorCode := ""
	if providerErr != nil {
		status = 0
		outcome = "network_error"
		errorCode = hostedRateErrorCode(mappedErr)
		var upstream *HTTPError
		switch {
		case errors.Is(providerErr, context.DeadlineExceeded):
			outcome = "timeout"
		case errors.As(providerErr, &upstream):
			status = upstream.Status
			outcome = "provider_error"
			if upstream.Status >= 400 && upstream.Status < 500 &&
				upstream.Status != http.StatusTooManyRequests {
				outcome = "client_error"
			}
		}
	}
	_ = p.recorder.RecordProviderAPICall(
		auditCtx,
		p.providerCode,
		alias,
		"partner/v1/rates",
		fingerprint,
		status,
		outcome,
		duration,
		1,
		errorCode,
	)
}

func hostedRateErrorCode(err error) string {
	switch {
	case errors.Is(err, rates.ErrProviderUnauthorized):
		return "PROVIDER_UNAUTHORIZED"
	case errors.Is(err, rates.ErrProviderRateLimited):
		return "PROVIDER_RATE_LIMITED"
	case errors.Is(err, rates.ErrProviderQuotaExhausted):
		return "PROVIDER_QUOTA_EXHAUSTED"
	case errors.Is(err, rates.ErrRateNotAvailable):
		return "RATE_NOT_AVAILABLE"
	case errors.Is(err, rates.ErrProviderLocationMapping):
		return "PROVIDER_LOCATION_NOT_MAPPED"
	default:
		return "PROVIDER_UNAVAILABLE"
	}
}

func hostedRateFingerprint(request rates.Request) string {
	courierCodes := append([]string(nil), request.Couriers...)
	sort.Strings(courierCodes)
	payload := strings.Join([]string{
		request.TenantID,
		request.Origin,
		request.Destination,
		request.Granularity,
		strings.Join(courierCodes, ","),
		strconv.FormatInt(request.ActualWeightGrams, 10),
		request.PriceFilter,
	}, "|")
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func mapRateError(err error) error {
	var upstream *HTTPError
	if errors.As(err, &upstream) {
		switch upstream.Status {
		case http.StatusUnauthorized, http.StatusForbidden:
			return rates.ErrProviderUnauthorized
		case http.StatusTooManyRequests:
			return rates.ErrProviderRateLimited
		case http.StatusBadRequest, http.StatusNotFound, http.StatusUnprocessableEntity:
			return rates.ErrRateNotAvailable
		default:
			return rates.ErrProviderUnavailable
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return rates.ErrProviderUnavailable
	}
	return rates.ErrProviderUnavailable
}

func parseETD(value string) (*int, *int) {
	values := etdNumbers.FindAllString(value, 2)
	if len(values) == 0 {
		return nil, nil
	}
	minimum, _ := strconv.Atoi(values[0])
	maximum := minimum
	if len(values) > 1 {
		maximum, _ = strconv.Atoi(values[1])
	}
	return &minimum, &maximum
}
