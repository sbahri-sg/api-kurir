package rates

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformcache "github.com/emisell/api-kurir/internal/platform/cache"
)

type keyedMemorySnapshots struct {
	mu        sync.Mutex
	quotes    map[string][]ProviderQuote
	findCalls atomic.Int64
}

type sharedPlatformCredentialSelector struct{}

func (sharedPlatformCredentialSelector) SelectedCredentialID(
	context.Context,
	string,
	string,
) (string, error) {
	return "", nil
}

type delayedQuoteProvider struct {
	calls atomic.Int64
	delay time.Duration
}

type countingReferenceRepository struct {
	rateCardCalls     atomic.Int64
	policyCalls       atomic.Int64
	presentationCalls atomic.Int64
}

func (r *countingReferenceRepository) FindActiveRateCards(
	context.Context,
	Request,
) ([]RateCard, error) {
	r.rateCardCalls.Add(1)
	return nil, nil
}

func (r *countingReferenceRepository) FindActiveServicePolicies(
	context.Context,
	[]string,
) ([]ServicePolicy, error) {
	r.policyCalls.Add(1)
	return nil, nil
}

func (r *countingReferenceRepository) FindActiveCourierPresentations(
	context.Context,
	[]string,
) (map[string]CourierPresentation, error) {
	r.presentationCalls.Add(1)
	return nil, nil
}

func (p *delayedQuoteProvider) Code() string { return "rajaongkir" }

func (p *delayedQuoteProvider) Quote(
	ctx context.Context,
	_ Request,
) ([]ProviderQuote, error) {
	p.calls.Add(1)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(p.delay):
	}
	now := time.Now().UTC()
	return []ProviderQuote{{
		ProviderCode: "rajaongkir", CourierCode: "jne", CourierName: "JNE",
		ServiceCode: "REG", ServiceName: "Regular", Cost: 15_000,
		VerificationStatus: "observed", FetchedAt: now, ExpiresAt: now.Add(time.Hour),
	}}, nil
}

func (s *keyedMemorySnapshots) FindFreshProviderQuotes(
	_ context.Context,
	request Request,
	providerCode string,
) ([]ProviderQuote, error) {
	s.findCalls.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]ProviderQuote(nil), s.quotes[providerCode+":"+requestKey(request)]...), nil
}

func (s *keyedMemorySnapshots) SaveProviderQuotes(
	_ context.Context,
	request Request,
	quotes []ProviderQuote,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	providerCode := ""
	if len(quotes) > 0 {
		providerCode = quotes[0].ProviderCode
	}
	s.quotes[providerCode+":"+requestKey(request)] = append([]ProviderQuote(nil), quotes...)
	return nil
}

func TestPlatformProviderSnapshotIsSharedAcrossMerchants(t *testing.T) {
	t.Parallel()
	provider := &quoteProvider{}
	snapshots := &keyedMemorySnapshots{quotes: make(map[string][]ProviderQuote)}
	service := NewService(
		emptyRepository{}, time.Second,
		WithProviderFallback(provider, snapshots, immediateLocker{}, time.Second),
		WithCredentialSelector(&credentialSelectorStub{}),
	)

	for _, tenantID := range []string{"merchant_a", "merchant_b"} {
		results, err := service.Calculate(context.Background(), Request{
			TenantID: tenantID, Origin: "origin", Destination: "destination",
			ActualWeightGrams: 1_000, Couriers: []string{"jne"},
		})
		if err != nil || len(results) != 1 {
			t.Fatalf("tenant=%s results=%#v err=%v", tenantID, results, err)
		}
	}
	if calls := provider.calls.Load(); calls != 1 {
		t.Fatalf("provider calls=%d want 1", calls)
	}
}

func TestBYOKProviderSnapshotRemainsTenantScoped(t *testing.T) {
	t.Parallel()
	provider := &quoteProvider{}
	snapshots := &keyedMemorySnapshots{quotes: make(map[string][]ProviderQuote)}
	service := NewService(
		emptyRepository{}, time.Second,
		WithProviderFallback(provider, snapshots, immediateLocker{}, time.Second),
		WithCredentialSelector(&credentialSelectorStub{id: "credential_byok"}),
	)

	for _, tenantID := range []string{"merchant_a", "merchant_b"} {
		results, err := service.Calculate(context.Background(), Request{
			TenantID: tenantID, Origin: "origin", Destination: "destination",
			ActualWeightGrams: 1_000, Couriers: []string{"jne"},
		})
		if err != nil || len(results) != 1 {
			t.Fatalf("tenant=%s results=%#v err=%v", tenantID, results, err)
		}
	}
	if calls := provider.calls.Load(); calls != 2 {
		t.Fatalf("provider calls=%d want 2", calls)
	}
}

func TestPlatformSnapshotCollapsesThousandConcurrentMerchants(t *testing.T) {
	provider := &delayedQuoteProvider{delay: 50 * time.Millisecond}
	snapshots := &keyedMemorySnapshots{quotes: make(map[string][]ProviderQuote)}
	references := &countingReferenceRepository{}
	service := NewService(
		references, 3*time.Second,
		WithProviderFallback(
			provider,
			snapshots,
			platformcache.NewLocalLocker(),
			time.Second,
		),
		WithCredentialSelector(sharedPlatformCredentialSelector{}),
		WithServicePolicyCache(platformcache.NewMemory(100), 5*time.Minute),
	)

	const customers = 1_000
	start := make(chan struct{})
	errorsFound := make(chan error, customers)
	var group sync.WaitGroup
	group.Add(customers)
	for index := 0; index < customers; index++ {
		go func(customer int) {
			defer group.Done()
			<-start
			results, err := service.Calculate(context.Background(), Request{
				TenantID: fmt.Sprintf("merchant_%04d", customer),
				Origin:   "origin", Destination: "destination",
				ActualWeightGrams: 1_000, Couriers: []string{"jne"},
			})
			if err != nil || len(results) != 1 || results[0].Cost.Total != 15_000 {
				errorsFound <- fmt.Errorf("customer=%d results=%#v err=%v", customer, results, err)
			}
		}(index)
	}
	close(start)
	group.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Error(err)
	}
	if calls := provider.calls.Load(); calls != 1 {
		t.Fatalf("provider calls=%d want 1", calls)
	}
	if calls := snapshots.findCalls.Load(); calls > 2 {
		t.Fatalf("snapshot reads=%d want at most 2", calls)
	}
	if calls := references.policyCalls.Load(); calls != 1 {
		t.Fatalf("policy reads=%d want 1", calls)
	}
	if calls := references.presentationCalls.Load(); calls != 1 {
		t.Fatalf("presentation reads=%d want 1", calls)
	}
	if calls := references.rateCardCalls.Load(); calls != 1 {
		t.Fatalf("rate card reads=%d want 1", calls)
	}
}
