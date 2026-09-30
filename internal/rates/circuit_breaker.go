package rates

import (
	"errors"
	"sync"
	"time"
)

// primaryCircuitBreaker protects the shared primary provider from repeated
// failures. Once open, only one recovery probe is allowed after the cooldown;
// concurrent requests continue through the configured fallback chain.
type primaryCircuitBreaker struct {
	mu               sync.Mutex
	failureThreshold int
	openDuration     time.Duration
	now              func() time.Time

	consecutiveFailures int
	openedAt            time.Time
	probeInFlight       bool
}

func newPrimaryCircuitBreaker(
	failureThreshold int,
	openDuration time.Duration,
) *primaryCircuitBreaker {
	if failureThreshold < 1 || openDuration <= 0 {
		return nil
	}
	return &primaryCircuitBreaker{
		failureThreshold: failureThreshold,
		openDuration:     openDuration,
		now:              time.Now,
	}
}

func (b *primaryCircuitBreaker) allow() bool {
	if b == nil {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.openedAt.IsZero() {
		return true
	}
	if b.now().Sub(b.openedAt) < b.openDuration {
		return false
	}
	if b.probeInFlight {
		return false
	}
	b.probeInFlight = true
	return true
}

func (b *primaryCircuitBreaker) record(err error) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	if !isPrimaryCircuitFailure(err) {
		b.consecutiveFailures = 0
		b.openedAt = time.Time{}
		b.probeInFlight = false
		return
	}

	b.probeInFlight = false
	if !b.openedAt.IsZero() {
		b.openedAt = b.now()
		return
	}
	b.consecutiveFailures++
	if b.consecutiveFailures >= b.failureThreshold {
		b.openedAt = b.now()
	}
}

func isPrimaryCircuitFailure(err error) bool {
	return errors.Is(err, ErrProviderUnavailable) ||
		errors.Is(err, ErrProviderRateLimited) ||
		errors.Is(err, ErrProviderQuotaExhausted) ||
		errors.Is(err, ErrProviderUnauthorized)
}
