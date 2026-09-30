package cache

import (
	"context"
	"sync"
	"time"
)

// LocalLocker prevents duplicate work inside one API process without holding
// a database connection while an upstream provider request is in flight.
// Multi-instance deployments should enable Redis for distributed locking.
type LocalLocker struct {
	mu    sync.Mutex
	locks map[string]struct{}
}

func NewLocalLocker() *LocalLocker {
	return &LocalLocker{locks: make(map[string]struct{})}
}

func (l *LocalLocker) WithLock(
	ctx context.Context,
	key string,
	_ time.Duration,
	fn func(context.Context) error,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	l.mu.Lock()
	if _, exists := l.locks[key]; exists {
		l.mu.Unlock()
		return ErrLockNotAcquired
	}
	l.locks[key] = struct{}{}
	l.mu.Unlock()

	defer func() {
		l.mu.Lock()
		delete(l.locks, key)
		l.mu.Unlock()
	}()
	return fn(ctx)
}
