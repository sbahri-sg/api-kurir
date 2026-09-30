package cache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLocalLockerRejectsDuplicateKeyAndReleases(t *testing.T) {
	t.Parallel()
	locker := NewLocalLocker()
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- locker.WithLock(context.Background(), "same", time.Minute, func(context.Context) error {
			close(entered)
			<-release
			return errors.New("provider failed")
		})
	}()
	<-entered

	err := locker.WithLock(context.Background(), "same", time.Minute, func(context.Context) error {
		return nil
	})
	if !errors.Is(err, ErrLockNotAcquired) {
		t.Fatalf("duplicate lock error=%v want %v", err, ErrLockNotAcquired)
	}
	close(release)
	if err := <-done; err == nil || err.Error() != "provider failed" {
		t.Fatalf("first lock error=%v", err)
	}

	if err := locker.WithLock(context.Background(), "same", time.Minute, func(context.Context) error {
		return nil
	}); err != nil {
		t.Fatalf("released lock cannot be reacquired: %v", err)
	}
}

func TestLocalLockerAllowsManyIndependentKeysConcurrently(t *testing.T) {
	t.Parallel()
	locker := NewLocalLocker()
	const callers = 100
	var active atomic.Int32
	var peak atomic.Int32
	start := make(chan struct{})
	var group sync.WaitGroup
	group.Add(callers)
	for index := 0; index < callers; index++ {
		go func(key string) {
			defer group.Done()
			<-start
			err := locker.WithLock(context.Background(), key, time.Minute, func(context.Context) error {
				current := active.Add(1)
				for {
					previous := peak.Load()
					if current <= previous || peak.CompareAndSwap(previous, current) {
						break
					}
				}
				time.Sleep(5 * time.Millisecond)
				active.Add(-1)
				return nil
			})
			if err != nil {
				t.Errorf("lock %s: %v", key, err)
			}
		}(string(rune(index + 1)))
	}
	close(start)
	group.Wait()
	if got := peak.Load(); got < 2 {
		t.Fatalf("peak concurrency=%d want at least 2", got)
	}
}

func TestLocalLockerHonorsCanceledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	err := NewLocalLocker().WithLock(ctx, "key", time.Minute, func(context.Context) error {
		called = true
		return nil
	})
	if !errors.Is(err, context.Canceled) || called {
		t.Fatalf("err=%v called=%t", err, called)
	}
}
