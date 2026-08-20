package tracking

import (
	"testing"
	"time"
)

func TestEconomyCheckpointIntervalsAndBudget(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	tests := []struct {
		status  string
		hits    int
		want    time.Duration
		stopped bool
	}{
		{status: "pending_pickup", hits: 1, want: 12 * time.Hour},
		{status: "pending_pickup", hits: 2, want: 24 * time.Hour},
		{status: "in_transit", hits: 4, want: 12 * time.Hour},
		{status: "out_for_delivery", hits: 5, want: 2 * time.Hour},
		{status: "in_transit", hits: 10, stopped: true},
		{status: "delivered", hits: 2, stopped: true},
	}
	for _, test := range tests {
		result := ApplyEconomyCheckpoint(Result{
			NormalizedStatus: test.status,
			FetchedAt:        now,
		}, test.hits, 10)
		if test.stopped {
			if result.NextRefreshAt != nil {
				t.Fatalf("%s/%d must stop polling", test.status, test.hits)
			}
			continue
		}
		if result.NextRefreshAt == nil || result.NextRefreshAt.Sub(now) != test.want {
			t.Fatalf("%s/%d: got %#v want %s", test.status, test.hits, result.NextRefreshAt, test.want)
		}
	}
}

func TestNotFoundStopsAfterThreeChecks(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	first, invalid := notFoundCheckpoint(now, 1, 1, 10)
	if invalid || first == nil || first.Sub(now) != 12*time.Hour {
		t.Fatalf("unexpected first checkpoint: %#v invalid=%v", first, invalid)
	}
	second, invalid := notFoundCheckpoint(now, 2, 2, 10)
	if invalid || second == nil || second.Sub(now) != 24*time.Hour {
		t.Fatalf("unexpected second checkpoint: %#v invalid=%v", second, invalid)
	}
	third, invalid := notFoundCheckpoint(now, 3, 3, 10)
	if !invalid || third != nil {
		t.Fatalf("third miss must become invalid: %#v invalid=%v", third, invalid)
	}
}
