package apikeys

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"sync"
	"time"
)

const (
	keyNamespace    = "ek_live_"
	randomKeyBytes  = 32
	cacheTTL        = time.Minute
	cacheMaxEntries = 10_000
)

type cacheEntry struct {
	validUntil time.Time
}

type Service struct {
	repository Repository
	mu         sync.RWMutex
	cache      map[string]cacheEntry
}

func NewService(repository Repository) *Service {
	return &Service{
		repository: repository,
		cache:      make(map[string]cacheEntry),
	}
}

func (s *Service) List(ctx context.Context, limit, offset int) ([]APIKey, error) {
	return s.repository.List(ctx, limit, offset)
}

func (s *Service) Generate(
	ctx context.Context,
	actor, requestID string,
) (GeneratedAPIKey, error) {
	return s.generate(ctx, actor, requestID, []string{ScopeShippingRead, ScopeTrackingRead})
}

func (s *Service) GenerateMainService(
	ctx context.Context,
	actor, requestID string,
) (GeneratedAPIKey, error) {
	return s.generate(ctx, actor, requestID, []string{
		ScopeShippingRead,
		ScopeTrackingRead,
		ScopeGatewayAccess,
	})
}

func (s *Service) generate(
	ctx context.Context,
	actor, requestID string,
	scopes []string,
) (GeneratedAPIKey, error) {
	randomValue := make([]byte, randomKeyBytes)
	if _, err := rand.Read(randomValue); err != nil {
		return GeneratedAPIKey{}, err
	}
	secret := keyNamespace + base64.RawURLEncoding.EncodeToString(randomValue)
	hash := sha256.Sum256([]byte(secret))
	prefixLength := len(keyNamespace) + 8

	item, err := s.repository.Create(ctx, CreateInput{
		KeyPrefix: secret[:prefixLength],
		KeyLast4:  secret[len(secret)-4:],
		KeyHash:   hash[:],
		Scopes:    scopes,
		CreatedBy: actor,
		RequestID: requestID,
	})
	if err != nil {
		return GeneratedAPIKey{}, err
	}
	item.Kind = keyKindFromScopes(item.Scopes)
	return GeneratedAPIKey{APIKey: item, Secret: secret}, nil
}

func (s *Service) Revoke(
	ctx context.Context,
	id, actor, requestID string,
) error {
	if err := s.repository.Revoke(ctx, id, actor, requestID); err != nil {
		return err
	}
	s.mu.Lock()
	clear(s.cache)
	s.mu.Unlock()
	return nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (bool, error) {
	return s.authenticate(ctx, token, "")
}

func (s *Service) AuthenticateScope(
	ctx context.Context,
	token, scope string,
) (bool, error) {
	return s.authenticate(ctx, token, strings.TrimSpace(scope))
}

func (s *Service) authenticate(ctx context.Context, token, scope string) (bool, error) {
	token = strings.TrimSpace(token)
	if !strings.HasPrefix(token, keyNamespace) {
		return false, nil
	}
	hash := sha256.Sum256([]byte(token))
	cacheKey := hex.EncodeToString(hash[:]) + ":" + scope
	now := time.Now()

	s.mu.RLock()
	entry, cached := s.cache[cacheKey]
	s.mu.RUnlock()
	if cached && entry.validUntil.After(now) {
		return true, nil
	}

	var valid bool
	var err error
	if scope == "" {
		valid, err = s.repository.Authenticate(ctx, hash[:])
	} else {
		valid, err = s.repository.AuthenticateScope(ctx, hash[:], scope)
	}
	if err != nil || !valid {
		return valid, err
	}
	s.mu.Lock()
	s.cache[cacheKey] = cacheEntry{validUntil: now.Add(cacheTTL)}
	if len(s.cache) > cacheMaxEntries {
		for key, candidate := range s.cache {
			if !candidate.validUntil.After(now) || len(s.cache) > cacheMaxEntries {
				delete(s.cache, key)
			}
			if len(s.cache) <= cacheMaxEntries {
				break
			}
		}
	}
	s.mu.Unlock()
	return true, nil
}
