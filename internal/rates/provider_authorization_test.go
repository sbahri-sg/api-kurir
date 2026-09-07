package rates

import (
	"context"
	"errors"
	"testing"
	"time"
)

type selectedProvider struct{}

func (selectedProvider) ActiveProviderCode(context.Context, string) (string, error) {
	return "rajaongkir", nil
}

type providerAuthFunc func(context.Context, string, string, string) error

func (f providerAuthFunc) Authorize(c context.Context, m, p, o string) error { return f(c, m, p, o) }
func TestInstalledProviderCheckedBeforeCachedRates(t *testing.T) {
	denied := errors.New("revoked")
	var grantErr error
	calls := 0
	gate := providerAuthFunc(func(_ context.Context, m, p, o string) error {
		calls++
		if m != "merchant-a" || p != "rajaongkir" || o != "rates.read" {
			t.Fatal("wrong binding identity")
		}
		return grantErr
	})
	repo := &countingRepository{card: RateCard{PricingModel: "flat", BasePrice: 15000, WeightIncrementGrams: 1000, RoundingMode: "ceil", RoundingIncrementGrams: 1000}}
	s := NewService(repo, time.Second, WithInstalledProviderAuthorization(selectedProvider{}, gate), WithRequestAuthorization(func(context.Context, Request) error { return nil }))
	r := Request{TenantID: "merchant-a", Origin: "o", Destination: "d", ActualWeightGrams: 1000, Couriers: []string{"jne"}}
	if _, err := s.Calculate(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	grantErr = denied
	if _, err := s.Calculate(context.Background(), r); !errors.Is(err, denied) {
		t.Fatal("cached rate bypassed revocation", err)
	}
	if calls != 2 || repo.calls.Load() != 1 {
		t.Fatal("authorization not called for every request")
	}
}
