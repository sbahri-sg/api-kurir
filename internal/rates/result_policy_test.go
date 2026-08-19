package rates

import (
	"context"
	"testing"
	"time"
)

type resultPolicyFunc func(context.Context, string, []Result) ([]Result, error)

func (function resultPolicyFunc) Filter(
	ctx context.Context,
	tenantID string,
	results []Result,
) ([]Result, error) {
	return function(ctx, tenantID, results)
}

func TestServiceAppliesResultPolicyOnlyForTenantRequests(t *testing.T) {
	t.Parallel()
	policyCalls := 0
	service := NewService(
		staticRepository{cards: []RateCard{{
			CourierCode:            "jne",
			ServiceCode:            "REG",
			PricingModel:           "flat",
			BasePrice:              10_000,
			WeightIncrementGrams:   1_000,
			RoundingMode:           "ceil",
			RoundingIncrementGrams: 1_000,
		}}},
		time.Second,
		WithResultPolicy(resultPolicyFunc(func(
			_ context.Context,
			tenantID string,
			results []Result,
		) ([]Result, error) {
			policyCalls++
			if tenantID != "merchant_123" {
				t.Fatalf("unexpected tenant: %q", tenantID)
			}
			return results[:0], nil
		})),
	)

	_, err := service.Calculate(context.Background(), Request{
		TenantID:          "merchant_123",
		Origin:            "a",
		Destination:       "b",
		ActualWeightGrams: 1_000,
		Couriers:          []string{"jne"},
	})
	if err != ErrRateNotAvailable {
		t.Fatalf("error=%v want ErrRateNotAvailable", err)
	}
	if policyCalls != 1 {
		t.Fatalf("policy calls=%d want 1", policyCalls)
	}
}
