package cache

import (
	"context"
	"errors"
	"time"
)

var ErrLockNotAcquired = errors.New("distributed lock not acquired")

type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}

type Locker interface {
	WithLock(ctx context.Context, key string, ttl time.Duration, fn func(context.Context) error) error
}

type RateLimitDecision struct {
	Allowed   bool
	Remaining int64
	ResetAt   time.Time
}

type RateLimiter interface {
	Allow(ctx context.Context, key string, limit int64, window time.Duration) (RateLimitDecision, error)
}
