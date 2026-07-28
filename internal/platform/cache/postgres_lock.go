package cache

import (
	"context"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresLocker struct {
	pool *pgxpool.Pool
}

func NewPostgresLocker(pool *pgxpool.Pool) *PostgresLocker {
	return &PostgresLocker{pool: pool}
}

func (l *PostgresLocker) WithLock(
	ctx context.Context,
	key string,
	_ time.Duration,
	fn func(context.Context) error,
) error {
	conn, err := l.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire lock connection: %w", err)
	}
	defer conn.Release()

	lockID := advisoryLockID(key)
	var acquired bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", lockID).Scan(&acquired); err != nil {
		return fmt.Errorf("acquire advisory lock: %w", err)
	}
	if !acquired {
		return ErrLockNotAcquired
	}

	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = conn.Exec(unlockCtx, "SELECT pg_advisory_unlock($1)", lockID)
	}()
	return fn(ctx)
}

func advisoryLockID(key string) int64 {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(key))
	return int64(hash.Sum64())
}
