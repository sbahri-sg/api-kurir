package cache

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type Redis struct {
	client *redis.Client
	prefix string
}

func NewRedis(client *redis.Client, prefix string) *Redis {
	return &Redis{
		client: client,
		prefix: strings.TrimSuffix(prefix, ":"),
	}
}

func (r *Redis) Get(ctx context.Context, key string) ([]byte, bool, error) {
	value, err := r.client.Get(ctx, r.key("cache", key)).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("redis get: %w", err)
	}
	return value, true, nil
}

func (r *Redis) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := r.client.Set(ctx, r.key("cache", key), value, ttl).Err(); err != nil {
		return fmt.Errorf("redis set: %w", err)
	}
	return nil
}

func (r *Redis) Delete(ctx context.Context, key string) error {
	if err := r.client.Del(ctx, r.key("cache", key)).Err(); err != nil {
		return fmt.Errorf("redis delete: %w", err)
	}
	return nil
}

func (r *Redis) WithLock(
	ctx context.Context,
	key string,
	ttl time.Duration,
	fn func(context.Context) error,
) error {
	if ttl <= 0 {
		ttl = 10 * time.Second
	}
	token, err := randomToken()
	if err != nil {
		return fmt.Errorf("create lock token: %w", err)
	}
	lockKey := r.key("lock", key)
	acquired, err := r.client.SetNX(ctx, lockKey, token, ttl).Result()
	if err != nil {
		return fmt.Errorf("redis acquire lock: %w", err)
	}
	if !acquired {
		return ErrLockNotAcquired
	}

	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = r.client.Eval(unlockCtx, `
			if redis.call("get", KEYS[1]) == ARGV[1] then
				return redis.call("del", KEYS[1])
			end
			return 0
		`, []string{lockKey}, token).Err()
	}()
	return fn(ctx)
}

func (r *Redis) Allow(
	ctx context.Context,
	key string,
	limit int64,
	window time.Duration,
) (RateLimitDecision, error) {
	if limit <= 0 || window <= 0 {
		return RateLimitDecision{}, fmt.Errorf("limit and window must be positive")
	}

	rateKey := r.key("rate", key)
	result, err := r.client.Eval(ctx, `
		local current = redis.call("INCR", KEYS[1])
		if current == 1 then
			redis.call("PEXPIRE", KEYS[1], ARGV[1])
		end
		local ttl = redis.call("PTTL", KEYS[1])
		return {current, ttl}
	`, []string{rateKey}, window.Milliseconds()).Int64Slice()
	if err != nil {
		return RateLimitDecision{}, fmt.Errorf("redis rate limit: %w", err)
	}

	now := time.Now()
	remaining := limit - result[0]
	if remaining < 0 {
		remaining = 0
	}
	return RateLimitDecision{
		Allowed:   result[0] <= limit,
		Remaining: remaining,
		ResetAt:   now.Add(time.Duration(result[1]) * time.Millisecond),
	}, nil
}

func (r *Redis) key(namespace, key string) string {
	if r.prefix == "" {
		return namespace + ":" + key
	}
	return r.prefix + ":" + namespace + ":" + key
}

func randomToken() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
