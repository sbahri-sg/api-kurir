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

func TestServiceNormalizesProviderCourierAliasBeforePolicyEvaluation(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	provider := &chainQuoteProvider{code: "partner", quotes: []ProviderQuote{
		{
			ProviderCode: "partner", CourierCode: "J&T Express",
			ServiceCode: "EZ", ServiceName: "J&T EZ", Cost: 12_000,
			VerificationStatus: "observed", FetchedAt: now,
			ExpiresAt: now.Add(time.Hour),
		},
		{
			ProviderCode: "partner", CourierCode: "J&T Express",
			ServiceCode: "HBO", ServiceName: "J&T HEBOH", Cost: 35_000,
			VerificationStatus: "observed", FetchedAt: now,
			ExpiresAt: now.Add(time.Hour),
		},
	}}
	repository := eligibilityRepository{policies: []ServicePolicy{
		{CourierCode: "jnt", ServiceCode: "EZ", MinimumAcceptedWeightGrams: 1},
		{CourierCode: "jnt", ServiceCode: "HBO", MinimumAcceptedWeightGrams: 3_000},
	}}
	service := NewService(
		repository,
		time.Second,
		WithProviderFallback(
			provider,
			&providerAwareSnapshots{quotes: make(map[string][]ProviderQuote)},
			immediateLocker{},
			time.Second,
		),
	)

	results, err := service.Calculate(context.Background(), Request{
		Origin: "loc_origin", Destination: "loc_destination",
		ActualWeightGrams: 1_000, Couriers: []string{"J&T"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Card.CourierCode != "jnt" ||
		results[0].Card.ServiceCode != "EZ" {
		t.Fatalf("unexpected normalized checkout services: %#v", results)
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

func TestServiceShowsOnlyCargoForEightyKilogramCheckout(t *testing.T) {
	t.Parallel()

	repository := eligibilityRepository{
		cards: []RateCard{
			policyTestRateCard("REG", "regular", "parcel"),
			policyTestRateCard("OKE", "economy", "parcel"),
			policyTestRateCard("YES", "next_day", "parcel"),
			policyTestRateCard("JTR", "cargo", "cargo"),
		},
		policies: []ServicePolicy{
			{
				ServiceGroup:               "regular",
				MinimumAcceptedWeightGrams: 1,
				MaximumAcceptedWeightGrams: int64Pointer(50_000),
			},
			{
				ServiceGroup:               "economy",
				MinimumAcceptedWeightGrams: 1,
				MaximumAcceptedWeightGrams: int64Pointer(50_000),
			},
			{
				ServiceGroup:               "next_day",
				MinimumAcceptedWeightGrams: 1,
				MaximumAcceptedWeightGrams: int64Pointer(50_000),
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
		ActualWeightGrams: 80_000,
		Couriers:          []string{"jne"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Card.ServiceGroup != "cargo" {
		t.Fatalf("80 kg checkout must only expose eligible cargo: %#v", results)
	}
}

func TestAnterajaCargoUsesSeparateAcceptedAndBillableMinimums(t *testing.T) {
	t.Parallel()

	repository := eligibilityRepository{
		cards: []RateCard{policyTestRateCardForCourier(
			"anteraja", "BIG", "cargo", "cargo",
		)},
		policies: []ServicePolicy{
			{
				CourierCode:                "anteraja",
				ServiceCode:                "BIG",
				MinimumAcceptedWeightGrams: 3_000,
				MinimumBillableWeightGrams: int64Pointer(5_000),
				MaximumAcceptedWeightGrams: int64Pointer(100_000),
			},
		},
	}
	service := NewService(repository, time.Second)

	results, err := service.Calculate(context.Background(), Request{
		Origin:            "loc_origin",
		Destination:       "loc_destination",
		ActualWeightGrams: 3_000,
		Couriers:          []string{"anteraja"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Weight.BillingGrams != 5_000 {
		t.Fatalf("3 kg Anteraja Cargo must be billed from 5 kg: %#v", results)
	}

	_, err = service.Calculate(context.Background(), Request{
		Origin:            "loc_origin",
		Destination:       "loc_destination",
		ActualWeightGrams: 100_001,
		Couriers:          []string{"anteraja"},
	})
	if !errors.Is(err, ErrRateNotAvailable) {
		t.Fatalf("Anteraja Cargo over 100 kg must be filtered, got %v", err)
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
	return policyTestRateCardForCourier(
		courierCode,
		serviceCode,
		serviceGroup,
		serviceType,
	)
}

func policyTestRateCardForCourier(
	courierCode,
	serviceCode,
	serviceGroup,
	serviceType string,
) RateCard {
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
