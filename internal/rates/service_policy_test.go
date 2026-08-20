package rates

import (
	"context"
	"errors"
	"testing"
	"time"
)

type eligibilityRepository struct {
	cards    []RateCard
	policies []ServicePolicy
}

type eligibilityQuoteProvider struct{}

func (eligibilityQuoteProvider) Code() string { return "rajaongkir" }

func (eligibilityQuoteProvider) Quote(context.Context, Request) ([]ProviderQuote, error) {
	now := time.Now().UTC()
	return []ProviderQuote{
		{
			ProviderCode:       "rajaongkir",
			CourierCode:        "jne",
			CourierName:        "JNE",
			ServiceCode:        "REG",
			ServiceName:        "Regular",
			Cost:               12_000,
			VerificationStatus: "observed",
			FetchedAt:          now,
			ExpiresAt:          now.Add(time.Hour),
		},
		{
			ProviderCode:       "rajaongkir",
			CourierCode:        "jne",
			CourierName:        "JNE",
			ServiceCode:        "JTR",
			ServiceName:        "JNE Trucking",
			Cost:               40_000,
			VerificationStatus: "observed",
			FetchedAt:          now,
			ExpiresAt:          now.Add(time.Hour),
		},
	}, nil
}

func (repository eligibilityRepository) FindActiveRateCards(
	context.Context,
	Request,
) ([]RateCard, error) {
	return repository.cards, nil
}

func (repository eligibilityRepository) FindActiveServicePolicies(
	context.Context,
	[]string,
) ([]ServicePolicy, error) {
	return repository.policies, nil
}

func TestServiceFiltersCargoBelowMinimumAcceptedWeight(t *testing.T) {
	t.Parallel()

	repository := eligibilityRepository{
		cards: []RateCard{
			policyTestRateCard("REG", "regular", "parcel"),
			policyTestRateCard("JTR", "cargo", "cargo"),
		},
		policies: []ServicePolicy{
			{
				CourierCode:                "jne",
				ServiceCode:                "REG",
				MinimumAcceptedWeightGrams: 1,
				MinimumBillableWeightGrams: int64Pointer(1_000),
			},
			{
				ServiceGroup:               "cargo",
				MinimumAcceptedWeightGrams: 3_000,
			},
		},
	}
	service := NewService(repository, time.Second)

	results, err := service.Calculate(context.Background(), Request{
		Origin:            "loc_origin",
		Destination:       "loc_destination",
		ActualWeightGrams: 800,
		Couriers:          []string{"jne"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Card.ServiceCode != "REG" {
		t.Fatalf("unexpected eligible services: %#v", results)
	}
	if results[0].Weight.BillingGrams != 1_000 {
		t.Fatalf("billing weight: got %d want 1000", results[0].Weight.BillingGrams)
	}
}

func TestServiceFiltersProviderQuoteBeforeCheckoutResult(t *testing.T) {
	t.Parallel()

	repository := eligibilityRepository{policies: []ServicePolicy{{
		ServiceGroup:               "cargo",
		MinimumAcceptedWeightGrams: 3_000,
	}}}
	service := NewService(
		repository,
		time.Second,
		WithProviderFallback(
			eligibilityQuoteProvider{},
			&memorySnapshots{},
			immediateLocker{},
			time.Second,
		),
	)

	results, err := service.Calculate(context.Background(), Request{
		Origin:            "loc_origin",
		Destination:       "loc_destination",
		ActualWeightGrams: 1_000,
		Couriers:          []string{"jne"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Card.ServiceCode != "REG" {
		t.Fatalf("cargo provider quote leaked to checkout: %#v", results)
	}
}

func TestServiceSpecificPolicyOverridesCargoDefaultAndAppliesMinimumBilling(t *testing.T) {
	t.Parallel()

	repository := eligibilityRepository{
		cards: []RateCard{policyTestRateCard("JTR", "cargo", "cargo")},
		policies: []ServicePolicy{
			{
				ServiceGroup:               "cargo",
				MinimumAcceptedWeightGrams: 3_000,
			},
			{
				CourierCode:                "jne",
				ServiceCode:                "JTR",
				MinimumAcceptedWeightGrams: 10_000,
				MinimumBillableWeightGrams: int64Pointer(10_000),
				MaximumAcceptedWeightGrams: int64Pointer(600_000),
			},
		},
	}
	service := NewService(repository, time.Second)

	if _, err := service.Calculate(context.Background(), Request{
		Origin:            "loc_origin",
		Destination:       "loc_destination",
		ActualWeightGrams: 3_000,
		Couriers:          []string{"jne"},
	}); !errors.Is(err, ErrRateNotAvailable) {
		t.Fatalf("3 kg JTR must be filtered, got %v", err)
	}

	results, err := service.Calculate(context.Background(), Request{
		Origin:            "loc_origin",
		Destination:       "loc_destination",
		ActualWeightGrams: 10_000,
		Couriers:          []string{"jne"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Weight.BillingGrams != 10_000 {
		t.Fatalf("unexpected JTR eligibility result: %#v", results)
	}
}

func TestServiceFiltersWeightAboveServiceMaximum(t *testing.T) {
	t.Parallel()

	repository := eligibilityRepository{
		cards: []RateCard{policyTestRateCard("HBO", "cargo", "cargo")},
		policies: []ServicePolicy{{
			CourierCode:                "jnt",
			ServiceCode:                "HBO",
			MinimumAcceptedWeightGrams: 2_310,
			MaximumAcceptedWeightGrams: int64Pointer(10_300),
		}},
	}
	service := NewService(repository, time.Second)

	_, err := service.Calculate(context.Background(), Request{
		Origin:            "loc_origin",
		Destination:       "loc_destination",
		ActualWeightGrams: 10_301,
		Couriers:          []string{"jnt"},
	})
	if !errors.Is(err, ErrRateNotAvailable) {
		t.Fatalf("overweight service must be filtered, got %v", err)
	}
}

func TestServicePolicyUsesProvidedWeightWithoutDimensionCalculation(t *testing.T) {
	t.Parallel()

	evaluation := evaluateServicePolicy(Request{
		ActualWeightGrams: 2_999,
		Dimensions: &Dimensions{
			LengthCM: 200,
			WidthCM:  200,
			HeightCM: 200,
		},
	}, ServicePolicy{
		WeightBasis:                WeightBasisProvided,
		MinimumAcceptedWeightGrams: 3_000,
	})
	if evaluation.Eligible || evaluation.Reason != ReasonWeightBelowMinimum {
		t.Fatalf("dimensions must not alter provided checkout weight: %#v", evaluation)
	}
}

func policyTestRateCard(serviceCode, serviceGroup, serviceType string) RateCard {
	courierCode := "jne"
	if serviceCode == "HBO" {
		courierCode = "jnt"
	}
	return RateCard{
		CourierCode:            courierCode,
		CourierName:            "Courier",
		ServiceCode:            serviceCode,
		CanonicalServiceCode:   serviceCode,
		ServiceName:            serviceCode,
		ServiceGroup:           serviceGroup,
		ServiceType:            serviceType,
		PricingModel:           "per_kg",
		RatePerIncrement:       1_000,
		WeightIncrementGrams:   1_000,
		RoundingMode:           "ceil",
		RoundingIncrementGrams: 1_000,
	}
}

func int64Pointer(value int64) *int64 {
	return &value
}
