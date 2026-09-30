package rates

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPrimaryCircuitBreakerOpensAndRecoversWithSingleProbe(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	breaker := newPrimaryCircuitBreaker(2, 30*time.Second)
	breaker.now = func() time.Time { return now }

	if !breaker.allow() {
		t.Fatal("closed circuit must allow the primary provider")
	}
	breaker.record(ErrProviderUnavailable)
	if !breaker.allow() {
		t.Fatal("circuit must remain closed below the failure threshold")
	}
	breaker.record(ErrProviderRateLimited)
	if breaker.allow() {
		t.Fatal("open circuit must skip the primary provider")
	}

	now = now.Add(31 * time.Second)
	const callers = 32
	var allowed atomic.Int32
	var group sync.WaitGroup
	group.Add(callers)
	for range callers {
		go func() {
			defer group.Done()
			if breaker.allow() {
				allowed.Add(1)
			}
		}()
	}
	group.Wait()
	if got := allowed.Load(); got != 1 {
		t.Fatalf("half-open probes=%d want 1", got)
	}

	breaker.record(nil)
	if !breaker.allow() {
		t.Fatal("successful recovery probe must close the circuit")
	}
}

func TestPrimaryCircuitBreakerIgnoresRouteSpecificFailures(t *testing.T) {
	t.Parallel()
	breaker := newPrimaryCircuitBreaker(1, time.Minute)
	breaker.record(ErrRateNotAvailable)
	if !breaker.allow() {
		t.Fatal("an unavailable route must not disable the provider globally")
	}
	breaker.record(ErrProviderLocationMapping)
	if !breaker.allow() {
		t.Fatal("a missing location mapping must not disable the provider globally")
	}
}

func TestServiceSkipsPrimaryWhileCircuitIsOpen(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	primary := &chainQuoteProvider{code: "rajaongkir", err: ErrProviderUnavailable}
	secondary := &chainQuoteProvider{code: "biteship", quotes: []ProviderQuote{{
		ProviderCode: "biteship", CourierCode: "jne", CourierName: "JNE",
		ServiceCode: "reg", CanonicalServiceCode: "REG",
		ServiceGroup: "regular", ServiceType: "parcel",
		ServiceName: "JNE Regular", Cost: 13_000,
		VerificationStatus: "observed", FetchedAt: now, ExpiresAt: now.Add(time.Hour),
	}}}
	service := NewService(
		emptyRepository{}, time.Second,
		WithProviderFallbackChain(
			primary,
			&providerAwareSnapshots{quotes: make(map[string][]ProviderQuote)},
			immediateLocker{}, time.Second,
			staticProviderFallbackPolicy{allowed: true},
			secondary,
		),
		WithPrimaryCircuitBreaker(2, time.Minute),
	)
	request := Request{
		Origin: "origin", Destination: "destination", ActualWeightGrams: 1_000,
		Couriers: []string{"jne"},
	}
	for attempt := 0; attempt < 3; attempt++ {
		results, err := service.Calculate(context.Background(), request)
		if err != nil || len(results) != 1 || results[0].Card.SourceProvider != "biteship" {
			t.Fatalf("attempt=%d results=%#v err=%v", attempt+1, results, err)
		}
	}
	if primary.calls != 2 {
		t.Fatalf("primary calls=%d want 2; third request must use the backup directly", primary.calls)
	}
	if secondary.calls != 1 {
		t.Fatalf("secondary calls=%d want 1; fresh backup snapshot must be reused", secondary.calls)
	}
}

func TestServiceCircuitBreakerDoesNotCrossBYOKCredentialBoundary(t *testing.T) {
	t.Parallel()
	primary := &chainQuoteProvider{code: "rajaongkir", err: ErrProviderUnavailable}
	secondary := &chainQuoteProvider{code: "biteship"}
	service := NewService(
		emptyRepository{}, time.Second,
		WithProviderFallbackChain(
			primary,
			&providerAwareSnapshots{quotes: make(map[string][]ProviderQuote)},
			immediateLocker{}, time.Second,
			staticProviderFallbackPolicy{allowed: false},
			secondary,
		),
		WithCredentialSelector(&credentialSelectorStub{id: "credential_merchant_a"}),
		WithPrimaryCircuitBreaker(1, time.Minute),
	)
	request := Request{
		TenantID: "merchant_a", Origin: "origin", Destination: "destination",
		ActualWeightGrams: 1_000, Couriers: []string{"jne"},
	}
	for attempt := 0; attempt < 3; attempt++ {
		_, err := service.Calculate(context.Background(), request)
		if err != ErrProviderUnavailable {
			t.Fatalf("attempt=%d err=%v want %v", attempt+1, err, ErrProviderUnavailable)
		}
	}
	if primary.calls != 3 {
		t.Fatalf("primary calls=%d want 3; BYOK calls must not share the platform circuit", primary.calls)
	}
	if secondary.calls != 0 {
		t.Fatalf("secondary calls=%d want 0", secondary.calls)
	}
}

func TestServiceKeepsPrimaryAvailableWhenFallbackIsUnavailable(t *testing.T) {
	t.Parallel()
	primary := &chainQuoteProvider{code: "rajaongkir", err: ErrProviderUnavailable}
	secondary := &chainQuoteProvider{code: "biteship", err: ErrProviderUnavailable}
	service := NewService(
		emptyRepository{}, time.Second,
		WithProviderFallbackChain(
			primary,
			&providerAwareSnapshots{quotes: make(map[string][]ProviderQuote)},
			immediateLocker{}, time.Second,
			staticProviderFallbackPolicy{allowed: true},
			secondary,
		),
		WithPrimaryCircuitBreaker(1, time.Minute),
	)
	request := Request{
		Origin: "origin", Destination: "destination", ActualWeightGrams: 1_000,
		Couriers: []string{"jne"},
	}
	for attempt := 0; attempt < 3; attempt++ {
		_, err := service.Calculate(context.Background(), request)
		if err != ErrProviderUnavailable {
			t.Fatalf("attempt=%d err=%v want %v", attempt+1, err, ErrProviderUnavailable)
		}
	}
	if primary.calls != 3 {
		t.Fatalf("primary calls=%d want 3; an unusable backup must not open the circuit", primary.calls)
	}
}

func TestServiceRetriesPrimaryIfFallbackStopsWorkingWhileCircuitIsOpen(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	primary := &chainQuoteProvider{code: "rajaongkir", err: ErrProviderUnavailable}
	secondary := &chainQuoteProvider{code: "biteship", quotes: []ProviderQuote{{
		ProviderCode: "biteship", CourierCode: "jne", CourierName: "JNE",
		ServiceCode: "reg", CanonicalServiceCode: "REG", ServiceGroup: "regular",
		ServiceType: "parcel", ServiceName: "JNE Regular", Cost: 13_000,
		VerificationStatus: "observed", FetchedAt: now, ExpiresAt: now.Add(time.Hour),
	}}}
	snapshots := &providerAwareSnapshots{quotes: make(map[string][]ProviderQuote)}
	service := NewService(
		emptyRepository{}, time.Second,
		WithProviderFallbackChain(
			primary, snapshots, immediateLocker{}, time.Second,
			staticProviderFallbackPolicy{allowed: true}, secondary,
		),
		WithPrimaryCircuitBreaker(1, time.Minute),
	)
	request := Request{
		Origin: "origin", Destination: "destination", ActualWeightGrams: 1_000,
		Couriers: []string{"jne"},
	}
	if _, err := service.Calculate(context.Background(), request); err != nil {
		t.Fatal(err)
	}

	// The successful fallback opened the circuit. Simulate Biteship becoming
	// unavailable while RajaOngkir has recovered.
	snapshots.quotes = make(map[string][]ProviderQuote)
	secondary.quotes = nil
	secondary.err = ErrProviderUnavailable
	primary.err = nil
	primary.quotes = []ProviderQuote{{
		ProviderCode: "rajaongkir", CourierCode: "jne", CourierName: "JNE",
		ServiceCode: "REG", CanonicalServiceCode: "REG", ServiceGroup: "regular",
		ServiceType: "parcel", ServiceName: "JNE Regular", Cost: 14_000,
		VerificationStatus: "observed", FetchedAt: now, ExpiresAt: now.Add(time.Hour),
	}}
	results, err := service.Calculate(context.Background(), request)
	if err != nil || len(results) != 1 || results[0].Card.SourceProvider != "rajaongkir" {
		t.Fatalf("results=%#v err=%v", results, err)
	}
	if primary.calls != 2 {
		t.Fatalf("primary calls=%d want 2; request must retry recovered primary", primary.calls)
	}
}
