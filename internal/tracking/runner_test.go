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

func TestProviderFailureRetryUsesEconomicalBackoff(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 29, 9, 0, 0, 0, time.FixedZone("Asia/Jakarta", 7*60*60))
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 0, want: time.Hour},
		{attempt: 1, want: 6 * time.Hour},
		{attempt: 2, want: 24 * time.Hour},
		{attempt: 10, want: 24 * time.Hour},
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
