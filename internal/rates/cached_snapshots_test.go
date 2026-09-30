package rates

import (
	"context"
	"errors"
	"testing"
	"time"

	platformcache "github.com/emisell/api-kurir/internal/platform/cache"
)

type countingSnapshotRepository struct {
	quotes    []ProviderQuote
	findCalls int
	saveCalls int
	err       error
}

func (r *countingSnapshotRepository) FindFreshProviderQuotes(
	context.Context,
	Request,
	string,
) ([]ProviderQuote, error) {
	r.findCalls++
	return append([]ProviderQuote(nil), r.quotes...), r.err
}

func (r *countingSnapshotRepository) SaveProviderQuotes(
	_ context.Context,
	_ Request,
	quotes []ProviderQuote,
) error {
	r.saveCalls++
	r.quotes = append([]ProviderQuote(nil), quotes...)
	return r.err
}

func TestCachedSnapshotRepositoryReadsHotQuoteWithoutPostgres(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	base := &countingSnapshotRepository{quotes: []ProviderQuote{{
		ProviderCode: "rajaongkir", CourierCode: "jne", ServiceCode: "REG",
		Cost: 15_000, FetchedAt: now, ExpiresAt: now.Add(time.Hour),
	}}}
	repository := NewCachedSnapshotRepository(base, platformcache.NewMemory(100), 5*time.Minute)
	repository.now = func() time.Time { return now }
	request := Request{Origin: "a", Destination: "b", ActualWeightGrams: 1_000}

	for attempt := 0; attempt < 2; attempt++ {
		quotes, err := repository.FindFreshProviderQuotes(context.Background(), request, "rajaongkir")
		if err != nil || len(quotes) != 1 {
			t.Fatalf("attempt=%d quotes=%#v err=%v", attempt+1, quotes, err)
		}
	}
	if base.findCalls != 1 {
		t.Fatalf("postgres reads=%d want 1", base.findCalls)
	}
}

func TestCachedSnapshotRepositoryDoesNotServeExpiredQuote(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	base := &countingSnapshotRepository{quotes: []ProviderQuote{{
		ProviderCode: "rajaongkir", ExpiresAt: now.Add(time.Minute),
	}}}
	repository := NewCachedSnapshotRepository(base, platformcache.NewMemory(100), time.Hour)
	repository.now = func() time.Time { return now }
	request := Request{Origin: "a", Destination: "b", ActualWeightGrams: 1_000}
	if _, err := repository.FindFreshProviderQuotes(context.Background(), request, "rajaongkir"); err != nil {
		t.Fatal(err)
	}

	now = now.Add(2 * time.Minute)
	base.quotes = nil
	quotes, err := repository.FindFreshProviderQuotes(context.Background(), request, "rajaongkir")
	if err != nil || len(quotes) != 0 {
		t.Fatalf("quotes=%#v err=%v", quotes, err)
	}
	if base.findCalls != 2 {
		t.Fatalf("postgres reads=%d want 2", base.findCalls)
	}
}

func TestCachedSnapshotRepositoryRequiresDurableSave(t *testing.T) {
	t.Parallel()
	base := &countingSnapshotRepository{err: errors.New("database unavailable")}
	repository := NewCachedSnapshotRepository(base, platformcache.NewMemory(100), time.Minute)
	err := repository.SaveProviderQuotes(context.Background(), Request{}, []ProviderQuote{{
		ProviderCode: "rajaongkir", ExpiresAt: time.Now().Add(time.Hour),
	}})
	if err == nil || base.saveCalls != 1 {
		t.Fatalf("err=%v save calls=%d", err, base.saveCalls)
	}
}
