package rates

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/emisell/api-kurir/internal/servicecatalog"
)

var (
	ErrProviderUnauthorized    = errors.New("provider unauthorized")
	ErrProviderQuotaExhausted  = errors.New("provider quota exhausted")
	ErrProviderRateLimited     = errors.New("provider rate limited")
	ErrProviderUnavailable     = errors.New("provider unavailable")
	ErrProviderLocationMapping = errors.New("provider location mapping not found")
)

type ProviderQuote struct {
	ProviderCode            string
	CourierCode             string
	CourierName             string
	ServiceCode             string
	ServiceName             string
	Description             string
	CanonicalServiceCode    string
	ServiceGroup            string
	ServiceType             string
	ServiceVariantCode      string
	ClassificationSource    string
	ClassificationReference string
	Cost                    int64
	ETDMinDays              *int
	ETDMaxDays              *int
	VerificationStatus      string
	FetchedAt               time.Time
	ExpiresAt               time.Time
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
	quote = classifyProviderQuote(quote)
	minimum := int64(0)
	return Result{
		Card: RateCard{
			CourierCode:          quote.CourierCode,
			CourierName:          quote.CourierName,
			ServiceCode:          quote.ServiceCode,
			ServiceName:          quote.ServiceName,
			CanonicalServiceCode: quote.CanonicalServiceCode,
			ServiceGroup:         quote.ServiceGroup,
			ServiceType:          quote.ServiceType,
			ServiceVariantCode:   quote.ServiceVariantCode,
			PricingModel:         "provider_quote",
			Currency:             "IDR",
			ETDMinDays:           quote.ETDMinDays,
			ETDMaxDays:           quote.ETDMaxDays,
			VerificationStatus:   quote.VerificationStatus,
			SourceProvider:       quote.ProviderCode,
			EffectiveFrom:        quote.FetchedAt,
			FetchedAt:            quote.FetchedAt,
			ExpiresAt:            &quote.ExpiresAt,
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

func classifyProviderQuote(quote ProviderQuote) ProviderQuote {
	// Adapters may provide a reviewed provider-specific alias. Preserve that
	// canonical classification instead of re-inferring it from the raw code.
	if quote.CanonicalServiceCode != "" &&
		quote.ServiceGroup != "" &&
		quote.ServiceGroup != servicecatalog.GroupUnknown &&
		quote.ServiceType != "" &&
		quote.ServiceType != servicecatalog.TypeUnknown {
		if quote.ServiceVariantCode == "" &&
			!strings.EqualFold(quote.CanonicalServiceCode, quote.ServiceCode) {
			quote.ServiceVariantCode = quote.ServiceCode
		}
		if quote.ClassificationSource == "" {
			quote.ClassificationSource = "provider_alias"
		}
		return quote
	}
	classification := servicecatalog.Classify(
		quote.CourierCode,
		quote.ServiceCode,
		quote.ServiceName+" "+quote.Description,
	)
	if classification.Matched {
		quote.CanonicalServiceCode = classification.CanonicalCode
		quote.ServiceGroup = classification.ServiceGroup
		quote.ServiceType = classification.ServiceType
		quote.ServiceVariantCode = classification.VariantCode
		quote.ClassificationSource = classification.ClassificationSource
		quote.ClassificationReference = classification.SourceReference
		return quote
	}
	if quote.ServiceGroup == "" {
		quote.ServiceGroup = servicecatalog.GroupUnknown
	}
	if quote.ServiceType == "" {
		quote.ServiceType = servicecatalog.TypeUnknown
	}
	if quote.ServiceVariantCode == "" {
		quote.ServiceVariantCode = quote.ServiceCode
	}
	if quote.ClassificationSource == "" {
		quote.ClassificationSource = "provider_observed"
	}
	return quote
}
