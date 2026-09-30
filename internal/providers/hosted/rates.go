package hosted

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strconv"
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
	ResolveProviderDistrict(ctx context.Context, locationPublicID, providerCode string) (string, error)
}

type RateProvider struct {
	providerCode string
	client       *Client
	credentials  RateCredentialResolver
	mappings     LocationMappingResolver
	quota        rates.QuotaRepository
	snapshotTTL  time.Duration
	now          func() time.Time
}

func NewRateProvider(
	providerCode string,
	client *Client,
	credentials RateCredentialResolver,
	mappings LocationMappingResolver,
	quota rates.QuotaRepository,
	snapshotTTL time.Duration,
) *RateProvider {
	return &RateProvider{
		providerCode: providerCode, client: client, credentials: credentials,
		mappings: mappings, quota: quota, snapshotTTL: snapshotTTL, now: time.Now,
	}
}

func (p *RateProvider) Code() string { return p.providerCode }

func (p *RateProvider) Quote(ctx context.Context, request rates.Request) ([]rates.ProviderQuote, error) {
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
	origin, err := p.mappings.ResolveProviderDistrict(ctx, request.Origin, p.providerCode)
	if err != nil {
		if errors.Is(err, locations.ErrProviderMappingNotFound) {
			return nil, rates.ErrProviderLocationMapping
		}
		return nil, err
	}
	destination, err := p.mappings.ResolveProviderDistrict(ctx, request.Destination, p.providerCode)
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
	if err := p.client.Do(ctx, http.MethodPost, "rates", "key", secret, input, &response); err != nil {
		return nil, mapRateError(err)
	}
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
