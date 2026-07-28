package main

import (
	"testing"
	"time"
)

func TestDurationUntilJakartaMidnight(t *testing.T) {
	t.Parallel()

	jakarta := time.FixedZone("Asia/Jakarta", 7*60*60)
	now := time.Date(2026, time.July, 28, 23, 59, 30, 0, jakarta)
	if actual := durationUntilJakartaMidnight(now); actual != 30*time.Second {
		t.Fatalf("unexpected duration until reset: %s", actual)
	}
}
