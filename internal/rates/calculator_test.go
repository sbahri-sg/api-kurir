package rates

import (
	"testing"
	"time"
)

func TestCalculateJTRWeightBoundaries(t *testing.T) {
	t.Parallel()

	threshold := int64(300)
	divisor := int64(5000)
	card := RateCard{
		PricingModel:           "per_kg",
		RatePerIncrement:       4000,
		MinimumWeightGrams:     10000,
		WeightIncrementGrams:   1000,
		VolumetricDivisor:      &divisor,
		RoundingProfileCode:    "jne-jtr-public-2026",
		RoundingMode:           "threshold",
		RoundingIncrementGrams: 1000,
		RoundingThresholdGrams: &threshold,
	}

	tests := []struct {
		name         string
		actualGrams  int64
		wantBilling  int64
		wantShipping int64
	}{
		{name: "minimum applies below ten kilograms", actualGrams: 9200, wantBilling: 10000, wantShipping: 40000},
		{name: "two hundred gram fraction rounds down", actualGrams: 10200, wantBilling: 10000, wantShipping: 40000},
		{name: "exact three hundred gram boundary rounds up", actualGrams: 10300, wantBilling: 11000, wantShipping: 44000},
		{name: "eight hundred gram fraction rounds up", actualGrams: 10800, wantBilling: 11000, wantShipping: 44000},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := Calculate(Request{ActualWeightGrams: tt.actualGrams}, card)
			if err != nil {
				t.Fatal(err)
			}
			if result.Weight.BillingGrams != tt.wantBilling {
				t.Fatalf("billing weight: got %d want %d", result.Weight.BillingGrams, tt.wantBilling)
			}
			if result.Cost.Shipping != tt.wantShipping {
				t.Fatalf("shipping: got %d want %d", result.Cost.Shipping, tt.wantShipping)
			}
		})
	}
}

func TestCalculateUsesVolumetricWeight(t *testing.T) {
	t.Parallel()

	threshold := int64(300)
	divisor := int64(5000)
	card := RateCard{
		PricingModel:           "per_kg",
		RatePerIncrement:       4000,
		MinimumWeightGrams:     10000,
		WeightIncrementGrams:   1000,
		VolumetricDivisor:      &divisor,
		RoundingMode:           "threshold",
		RoundingIncrementGrams: 1000,
		RoundingThresholdGrams: &threshold,
	}
	request := Request{
		ActualWeightGrams: 10000,
		Dimensions: &Dimensions{
			LengthCM: 100,
			WidthCM:  50,
			HeightCM: 20,
		},
	}

	result, err := Calculate(request, card)
	if err != nil {
		t.Fatal(err)
	}
	if result.Weight.VolumetricGrams != 20000 {
		t.Fatalf("volumetric weight: got %d want 20000", result.Weight.VolumetricGrams)
	}
	if result.Weight.BillingGrams != 20000 {
		t.Fatalf("billing weight: got %d want 20000", result.Weight.BillingGrams)
	}
}

func TestRateCardExpiryMetadataDoesNotAffectPureCalculation(t *testing.T) {
	t.Parallel()

	card := RateCard{
		PricingModel:           "flat",
		BasePrice:              15000,
		WeightIncrementGrams:   1000,
		RoundingMode:           "ceil",
		RoundingIncrementGrams: 1000,
		EffectiveFrom:          time.Now(),
	}
	result, err := Calculate(Request{ActualWeightGrams: 1000}, card)
	if err != nil {
		t.Fatal(err)
	}
	if result.Cost.Total != 15000 {
		t.Fatalf("total: got %d want 15000", result.Cost.Total)
	}
}
