package rates

import (
	"context"
	"errors"
	"time"
)

var (
	ErrProviderUnauthorized    = errors.New("provider unauthorized")
	ErrProviderQuotaExhausted  = errors.New("provider quota exhausted")
	ErrProviderRateLimited     = errors.New("provider rate limited")
	ErrProviderUnavailable     = errors.New("provider unavailable")
	ErrProviderLocationMapping = errors.New("provider location mapping not found")
)

type ProviderQuote struct {
	ProviderCode       string
	CourierCode        string
	CourierName        string
	ServiceCode        string
	ServiceName        string
	Description        string
	Cost               int64
	ETDMinDays         *int
	ETDMaxDays         *int
	VerificationStatus string
	FetchedAt          time.Time
	ExpiresAt          time.Time
}

type QuoteProvider interface {
	Code() string
	Quote(ctx context.Context, request Request) ([]ProviderQuote, error)
}

type SnapshotRepository interface {
	FindFreshProviderQuotes(
		ctx context.Context,
		request Request,
		providerCode string,
	) ([]ProviderQuote, error)
	SaveProviderQuotes(
		ctx context.Context,
		request Request,
		quotes []ProviderQuote,
	) error
}

type QuotaRepository interface {
	ConsumeProviderHit(
		ctx context.Context,
		providerCode string,
		credentialAlias string,
		dailyLimit int64,
	) error
}

func resultFromProviderQuote(request Request, quote ProviderQuote) Result {
	minimum := int64(0)
	return Result{
		Card: RateCard{
			CourierCode:        quote.CourierCode,
			CourierName:        quote.CourierName,
			ServiceCode:        quote.ServiceCode,
			ServiceName:        quote.ServiceName,
			ServiceType:        "unknown",
			PricingModel:       "provider_quote",
			Currency:           "IDR",
			ETDMinDays:         quote.ETDMinDays,
			ETDMaxDays:         quote.ETDMaxDays,
			VerificationStatus: quote.VerificationStatus,
			SourceProvider:     quote.ProviderCode,
			EffectiveFrom:      quote.FetchedAt,
			FetchedAt:          quote.FetchedAt,
			ExpiresAt:          &quote.ExpiresAt,
		},
		Weight: WeightBreakdown{
			ActualGrams:     request.ActualWeightGrams,
			ChargeableGrams: request.ActualWeightGrams,
			RoundedGrams:    request.ActualWeightGrams,
			BillingGrams:    request.ActualWeightGrams,
			MinimumGrams:    minimum,
			RoundingProfile: "provider-quote-not-disclosed",
		},
		Cost: CostBreakdown{
			Shipping: quote.Cost,
			Total:    quote.Cost,
		},
		SourceType: "provider_quote",
	}
}
