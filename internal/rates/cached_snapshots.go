package rates

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	platformcache "github.com/emisell/api-kurir/internal/platform/cache"
)

// CachedSnapshotRepository keeps hot provider quotes outside PostgreSQL while
// preserving PostgreSQL as the durable source of truth. The same decorator
// works with the in-process cache and Redis.
type CachedSnapshotRepository struct {
	repository SnapshotRepository
	cache      platformcache.Cache
	ttl        time.Duration
	now        func() time.Time
}

func NewCachedSnapshotRepository(
	repository SnapshotRepository,
	quoteCache platformcache.Cache,
	ttl time.Duration,
) *CachedSnapshotRepository {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &CachedSnapshotRepository{
		repository: repository,
		cache:      quoteCache,
		ttl:        ttl,
		now:        time.Now,
	}
}

func (r *CachedSnapshotRepository) FindFreshProviderQuotes(
	ctx context.Context,
	request Request,
	providerCode string,
) ([]ProviderQuote, error) {
	key := providerQuoteCacheKey(request, providerCode)
	if r.cache != nil {
		payload, found, err := r.cache.Get(ctx, key)
		if err == nil && found {
			var quotes []ProviderQuote
			if json.Unmarshal(payload, &quotes) == nil && freshProviderQuotes(quotes, r.now()) {
				return quotes, nil
			}
			_ = r.cache.Delete(context.WithoutCancel(ctx), key)
		}
	}

	quotes, err := r.repository.FindFreshProviderQuotes(ctx, request, providerCode)
	if err != nil {
		return nil, err
	}
	r.cacheQuotes(ctx, key, quotes)
	return quotes, nil
}

func (r *CachedSnapshotRepository) SaveProviderQuotes(
	ctx context.Context,
	request Request,
	quotes []ProviderQuote,
) error {
	if err := r.repository.SaveProviderQuotes(ctx, request, quotes); err != nil {
		return err
	}
	providerCode := ""
	if len(quotes) > 0 {
		providerCode = quotes[0].ProviderCode
	}
	r.cacheQuotes(ctx, providerQuoteCacheKey(request, providerCode), quotes)
	return nil
}

func (r *CachedSnapshotRepository) cacheQuotes(
	ctx context.Context,
	key string,
	quotes []ProviderQuote,
) {
	if r.cache == nil || len(quotes) == 0 || strings.TrimSpace(key) == "" {
		return
	}
	payload, err := json.Marshal(quotes)
	if err != nil {
		return
	}
	ttl := r.ttl
	now := r.now()
	for _, quote := range quotes {
		remaining := quote.ExpiresAt.Sub(now)
		if remaining <= 0 {
			return
		}
		if remaining < ttl {
			ttl = remaining
		}
	}
	_ = r.cache.Set(ctx, key, payload, ttl)
}

func freshProviderQuotes(quotes []ProviderQuote, now time.Time) bool {
	if len(quotes) == 0 {
		return false
	}
	for _, quote := range quotes {
		if quote.ExpiresAt.IsZero() || !quote.ExpiresAt.After(now) {
			return false
		}
	}
	return true
}

func providerQuoteCacheKey(request Request, providerCode string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(providerCode)) + ":" + requestKey(request)))
	return "provider-quotes:" + hex.EncodeToString(sum[:])
}
