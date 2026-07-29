package tracking

import (
	"testing"
	"time"
)

func TestTrackingFailureCodeDistinguishesRateLimitFromDailyQuota(t *testing.T) {
	t.Parallel()

	if got := trackingFailureCode(ErrProviderRateLimited); got != "PROVIDER_RATE_LIMITED" {
		t.Fatalf("rate-limit code: got %q", got)
	}
	if got := trackingFailureCode(ErrProviderQuota); got != "PROVIDER_QUOTA_EXHAUSTED" {
		t.Fatalf("quota code: got %q", got)
	}
}

func TestRateLimitRetryUsesShortCappedBackoff(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 29, 9, 0, 0, 0, time.FixedZone("Asia/Jakarta", 7*60*60))
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 0, want: 30 * time.Second},
		{attempt: 1, want: time.Minute},
		{attempt: 2, want: 2 * time.Minute},
		{attempt: 3, want: 4 * time.Minute},
		{attempt: 10, want: 5 * time.Minute},
	}
	for _, test := range tests {
		if got := retryDelay("PROVIDER_RATE_LIMITED", test.attempt, now); got != test.want {
			t.Fatalf(
				"attempt %d: got %s want %s",
				test.attempt,
				got,
				test.want,
			)
		}
	}
}
