package rates

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/emisell/api-kurir/internal/platform/cache"
)

type countingRepository struct {
	calls atomic.Int64
	card  RateCard
}

func (r *countingRepository) FindActiveRateCards(context.Context, Request) ([]RateCard, error) {
	r.calls.Add(1)
	time.Sleep(25 * time.Millisecond)
	return []RateCard{r.card}, nil
}

func TestServiceCoalescesIdenticalRequests(t *testing.T) {
	t.Parallel()

	repository := &countingRepository{
		card: RateCard{
			PricingModel:           "flat",
			BasePrice:              15000,
			WeightIncrementGrams:   1000,
			RoundingMode:           "ceil",
			RoundingIncrementGrams: 1000,
		},
	}
	service := NewService(repository, time.Second)
	request := Request{
		Origin:            "loc_origin",
		Destination:       "loc_destination",
		ActualWeightGrams: 1000,
		Couriers:          []string{"jne"},
	}

	const concurrentRequests = 20
	var wg sync.WaitGroup
	wg.Add(concurrentRequests)
	errs := make(chan error, concurrentRequests)
	for range concurrentRequests {
		go func() {
			defer wg.Done()
			_, err := service.Calculate(context.Background(), request)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if calls := repository.calls.Load(); calls != 1 {
		t.Fatalf("repository calls: got %d want 1", calls)
	}
}

type emptyRepository struct{}

func (emptyRepository) FindActiveRateCards(context.Context, Request) ([]RateCard, error) {
	return nil, nil
}

type staticShippingProviderGate struct {
	active bool
	err    error
}

func (gate staticShippingProviderGate) HasActiveProvider(context.Context, string) (bool, error) {
	return gate.active, gate.err
}

func TestServiceRejectsTenantRateWhenShippingIsInactive(t *testing.T) {
	t.Parallel()
	repository := &countingRepository{card: testFlatRateCard("REG", 15_000)}
	service := NewService(
		repository,
		time.Second,
		WithShippingProviderGate(staticShippingProviderGate{}),
	)

	_, err := service.Calculate(context.Background(), Request{
		TenantID:          "merchant_123",
		Origin:            "loc_origin",
		Destination:       "loc_destination",
		ActualWeightGrams: 1_000,
		Couriers:          []string{"jne"},
	})
	if !errors.Is(err, ErrShippingDisabled) {
		t.Fatalf("error=%v want ErrShippingDisabled", err)
	}
	if calls := repository.calls.Load(); calls != 0 {
		t.Fatalf("repository calls=%d want=0", calls)
	}
}

func TestServiceAllowsTenantRateWhenShippingIsActive(t *testing.T) {
	t.Parallel()
	service := NewService(
		staticRepository{cards: []RateCard{testFlatRateCard("REG", 15_000)}},
		time.Second,
		WithShippingProviderGate(staticShippingProviderGate{active: true}),
	)

	results, err := service.Calculate(context.Background(), Request{
		TenantID:          "merchant_123",
		Origin:            "loc_origin",
		Destination:       "loc_destination",
		ActualWeightGrams: 1_000,
		Couriers:          []string{"jne"},
	})
	if err != nil || len(results) != 1 {
		t.Fatalf("results=%#v err=%v", results, err)
	}
}

type memorySnapshots struct {
	mu     sync.Mutex
	quotes []ProviderQuote
}

func (s *memorySnapshots) FindFreshProviderQuotes(
	context.Context,
	Request,
	string,
) ([]ProviderQuote, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]ProviderQuote(nil), s.quotes...), nil
}

func (s *memorySnapshots) SaveProviderQuotes(
	_ context.Context,
	_ Request,
	quotes []ProviderQuote,
) error {
	s.mu.Lock()
	s.quotes = append([]ProviderQuote(nil), quotes...)
	s.mu.Unlock()
	return nil
}

type quoteProvider struct {
	calls atomic.Int64
}

func (p *quoteProvider) Code() string            { return "rajaongkir" }
func (p *quoteProvider) CredentialAlias() string { return "test" }
func (p *quoteProvider) DailyLimit() int64       { return 50_000 }
func (p *quoteProvider) Quote(context.Context, Request) ([]ProviderQuote, error) {
	p.calls.Add(1)
	now := time.Now().UTC()
	return []ProviderQuote{{
		ProviderCode:       "rajaongkir",
		CourierCode:        "jne",
		CourierName:        "JNE",
		ServiceCode:        "REG",
		ServiceName:        "Reguler",
		Cost:               18000,
		VerificationStatus: "observed",
		FetchedAt:          now,
		ExpiresAt:          now.Add(time.Hour),
	}}, nil
}

type immediateLocker struct{}

func (immediateLocker) WithLock(
	ctx context.Context,
	_ string,
	_ time.Duration,
	fn func(context.Context) error,
) error {
	return fn(ctx)
}

var _ cache.Locker = immediateLocker{}

func TestServiceCachesExactProviderQuote(t *testing.T) {
	t.Parallel()

	snapshots := &memorySnapshots{}
	provider := &quoteProvider{}
	service := NewService(
		emptyRepository{},
		time.Second,
		WithProviderFallback(provider, snapshots, immediateLocker{}, time.Second),
	)
	request := Request{
		Origin:            "loc_origin",
		Destination:       "loc_destination",
		ActualWeightGrams: 1000,
		Couriers:          []string{"jne"},
	}

	for range 2 {
		results, err := service.Calculate(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 1 ||
			results[0].SourceType != "provider_quote" ||
			results[0].Cost.Total != 18000 {
			t.Fatalf("unexpected result: %#v", results)
		}
	}
	if calls := provider.calls.Load(); calls != 1 {
		t.Fatalf("provider calls: got %d want 1", calls)
	}
}

func TestProviderQuoteUsesCanonicalServiceWithoutReplacingRawCode(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 29, 10, 0, 0, 0, time.UTC)
	result := resultFromProviderQuote(Request{
		ActualWeightGrams: 10_000,
	}, ProviderQuote{
		ProviderCode:       "rajaongkir",
		CourierCode:        "jne",
		CourierName:        "JNE",
		ServiceCode:        "JTR>130",
		ServiceName:        "JNE Trucking",
		Cost:               75_000,
		VerificationStatus: "observed",
		FetchedAt:          now,
		ExpiresAt:          now.Add(time.Hour),
	})

	if result.Card.ServiceCode != "JTR>130" ||
		result.Card.CanonicalServiceCode != "JTR" ||
		result.Card.ServiceGroup != "cargo" ||
		result.Card.ServiceType != "cargo" ||
		result.Card.ServiceVariantCode != "JTR>130" {
		t.Fatalf("unexpected classified provider quote: %#v", result.Card)
	}
}

type staticRepository struct {
	cards []RateCard
}

func (r staticRepository) FindActiveRateCards(context.Context, Request) ([]RateCard, error) {
	return r.cards, nil
}

type failingProvider struct{}

func (failingProvider) Code() string            { return "rajaongkir" }
func (failingProvider) CredentialAlias() string { return "test" }
func (failingProvider) DailyLimit() int64       { return 50_000 }
func (failingProvider) Quote(context.Context, Request) ([]ProviderQuote, error) {
	return nil, ErrProviderUnavailable
}

func TestServiceDoesNotHideProviderFailureBehindPartialLocalResult(t *testing.T) {
	t.Parallel()

	service := NewService(
		staticRepository{cards: []RateCard{{
			CourierCode:            "jne",
			ServiceCode:            "REG",
			ServiceName:            "Reguler",
			PricingModel:           "flat",
			BasePrice:              15_000,
			WeightIncrementGrams:   1_000,
			RoundingMode:           "ceil",
			RoundingIncrementGrams: 1_000,
			VerificationStatus:     "official_contract",
			EffectiveFrom:          time.Now().UTC(),
			FetchedAt:              time.Now().UTC(),
			SourceProvider:         "master",
			RoundingProfileCode:    "test",
			MinimumWeightGrams:     0,
			MaximumWeightGrams:     nil,
			VolumetricDivisor:      nil,
			RoundingThresholdGrams: nil,
		}}},
		time.Second,
		WithProviderFallback(
			failingProvider{},
			&memorySnapshots{},
			immediateLocker{},
			time.Second,
		),
	)

	_, err := service.Calculate(context.Background(), Request{
		Origin:            "loc_origin",
		Destination:       "loc_destination",
		ActualWeightGrams: 1000,
		Couriers:          []string{"jne", "tiki"},
	})
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("expected provider failure, got %v", err)
	}
}

func TestServicePriceFilterSortsWithoutDroppingServices(t *testing.T) {
	t.Parallel()

	cards := []RateCard{
		testFlatRateCard("YES", 30_000),
		testFlatRateCard("OKE", 10_000),
		testFlatRateCard("REG", 20_000),
	}
	service := NewService(staticRepository{cards: cards}, time.Second)

	for _, test := range []struct {
		name        string
		priceFilter string
		wantCosts   []int64
	}{
		{name: "default", wantCosts: []int64{10_000, 20_000, 30_000}},
		{name: "lowest", priceFilter: "lowest", wantCosts: []int64{10_000, 20_000, 30_000}},
		{name: "highest", priceFilter: "highest", wantCosts: []int64{30_000, 20_000, 10_000}},
	} {
		t.Run(test.name, func(t *testing.T) {
			results, err := service.Calculate(context.Background(), Request{
				Origin:            "loc_origin",
				Destination:       "loc_destination",
				ActualWeightGrams: 1_000,
				Couriers:          []string{"jne"},
				PriceFilter:       test.priceFilter,
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(results) != len(test.wantCosts) {
				t.Fatalf("result count: got %d want %d", len(results), len(test.wantCosts))
			}
			for index, wantCost := range test.wantCosts {
				if results[index].Cost.Total != wantCost {
					t.Fatalf(
						"cost[%d]: got %d want %d",
						index,
						results[index].Cost.Total,
						wantCost,
					)
				}
			}
		})
	}
}

func testFlatRateCard(serviceCode string, price int64) RateCard {
	return RateCard{
		CourierCode:            "jne",
		CourierName:            "JNE",
		ServiceCode:            serviceCode,
		ServiceName:            serviceCode,
		PricingModel:           "flat",
		BasePrice:              price,
		WeightIncrementGrams:   1_000,
		RoundingMode:           "ceil",
		RoundingIncrementGrams: 1_000,
		VerificationStatus:     "official_contract",
		EffectiveFrom:          time.Now().UTC(),
		FetchedAt:              time.Now().UTC(),
		SourceProvider:         "test",
		RoundingProfileCode:    "test",
	}
}
