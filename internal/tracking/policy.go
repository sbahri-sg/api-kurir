package tracking

import "time"

const (
	DefaultProviderHitLimit = 10
	MaxNotFoundChecks       = 3
)

// ApplyEconomyCheckpoint centralizes the shared-credential polling policy.
// providerHits includes the provider call that produced result.
func ApplyEconomyCheckpoint(result Result, providerHits, providerHitLimit int) Result {
	if providerHitLimit <= 0 {
		providerHitLimit = DefaultProviderHitLimit
	}
	result.IsFinal = isTerminalStatus(result.NormalizedStatus)
	if result.IsFinal || providerHits >= providerHitLimit {
		result.NextRefreshAt = nil
		return result
	}

	interval := 12 * time.Hour
	switch result.NormalizedStatus {
	case "out_for_delivery":
		interval = 2 * time.Hour
	case "pending_pickup", "unknown":
		if providerHits > 1 {
			interval = 24 * time.Hour
		}
	case "picked_up", "in_transit", "delivery_failed":
		interval = 12 * time.Hour
	}
	next := result.FetchedAt.Add(interval)
	result.NextRefreshAt = &next
	return result
}

func notFoundCheckpoint(fetchedAt time.Time, notFoundCount, providerHits, providerHitLimit int) (*time.Time, bool) {
	if providerHitLimit <= 0 {
		providerHitLimit = DefaultProviderHitLimit
	}
	if notFoundCount >= MaxNotFoundChecks || providerHits >= providerHitLimit {
		return nil, true
	}
	interval := 12 * time.Hour
	if notFoundCount >= 2 {
		interval = 24 * time.Hour
	}
	next := fetchedAt.Add(interval)
	return &next, false
}

func isTerminalStatus(status string) bool {
	switch status {
	case "delivered", "returned", "cancelled":
		return true
	default:
		return false
	}
}
