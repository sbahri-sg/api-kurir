package providercredentials

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrUnsupportedProvider = errors.New("provider is not supported")
	ErrInvalidSecret       = errors.New("provider API key is invalid")
)

type Validator interface {
	Validate(ctx context.Context, providerCode, secret string) error
}

type Resolver interface {
	ResolveProviderCredential(
		ctx context.Context,
		providerCode string,
	) (secret, credentialAlias string, dailyLimit int64, err error)
}

type Service struct {
	repository Repository
	cipher     *Cipher
	validator  Validator
}

func NewService(repository Repository, cipher *Cipher, validator Validator) *Service {
	return &Service{repository: repository, cipher: cipher, validator: validator}
}

func (s *Service) List(ctx context.Context) ([]Credential, error) {
	return s.repository.List(ctx)
}

func (s *Service) Add(
	ctx context.Context,
	providerCode, secret, actor, requestID string,
) (Credential, error) {
	providerCode = strings.ToLower(strings.TrimSpace(providerCode))
	secret = strings.TrimSpace(secret)
	if providerCode != "rajaongkir" {
		return Credential{}, ErrUnsupportedProvider
	}
	if len(secret) < 8 || len(secret) > 512 {
		return Credential{}, ErrInvalidSecret
	}
	if s.validator == nil {
		return Credential{}, errors.New("provider credential validator is unavailable")
	}
	if err := s.validator.Validate(ctx, providerCode, secret); err != nil {
		return Credential{}, fmt.Errorf("%w: %v", ErrInvalidSecret, err)
	}

	fingerprint := sha256.Sum256([]byte(secret))
	fingerprintHex := hex.EncodeToString(fingerprint[:])
	alias := providerCode + "-" + fingerprintHex[:8]
	prefixLength := 4
	if len(secret)-4 < prefixLength {
		prefixLength = len(secret) - 4
	}
	if prefixLength < 1 {
		return Credential{}, ErrInvalidSecret
	}
	associatedData := []byte(providerCode + ":" + fingerprintHex)
	ciphertext, err := s.cipher.Encrypt([]byte(secret), associatedData)
	if err != nil {
		return Credential{}, err
	}
	id, err := newUUID()
	if err != nil {
		return Credential{}, err
	}
	return s.repository.Create(ctx, CreateInput{
		ID:                id,
		ProviderCode:      providerCode,
		CredentialAlias:   alias,
		KeyPrefix:         secret[:prefixLength],
		KeyLastFour:       secret[len(secret)-4:],
		SecretCiphertext:  ciphertext,
		SecretFingerprint: fingerprint[:],
		DailyLimit:        DefaultDailyLimit,
		CreatedBy:         actor,
		RequestID:         requestID,
	})
}

func (s *Service) Disable(ctx context.Context, id, actor, requestID string) error {
	return s.repository.Disable(ctx, id, actor, requestID)
}

func (s *Service) ResolveProviderCredential(
	ctx context.Context,
	providerCode string,
) (string, string, int64, error) {
	providerCode = strings.ToLower(strings.TrimSpace(providerCode))
	stored, err := s.repository.ResolveActive(ctx, providerCode)
	if err != nil {
		return "", "", 0, err
	}
	fingerprintHex := hex.EncodeToString(stored.Fingerprint)
	plaintext, err := s.cipher.Decrypt(
		stored.SecretCiphertext,
		[]byte(providerCode+":"+fingerprintHex),
	)
	if err != nil {
		return "", "", 0, err
	}
	return string(plaintext), stored.CredentialAlias, stored.DailyLimit, nil
}

func newUUID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate provider credential ID: %w", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		value[0:4],
		value[4:6],
		value[6:8],
		value[8:10],
		value[10:16],
	), nil
}

type StaticFallbackResolver struct {
	primary   Resolver
	fallbacks map[string]StaticCredential
}

type StaticCredential struct {
	Secret          string
	CredentialAlias string
	DailyLimit      int64
}

func NewStaticFallbackResolver(
	primary Resolver,
	fallbacks map[string]StaticCredential,
) *StaticFallbackResolver {
	return &StaticFallbackResolver{primary: primary, fallbacks: fallbacks}
}

func (r *StaticFallbackResolver) ResolveProviderCredential(
	ctx context.Context,
	providerCode string,
) (string, string, int64, error) {
	secret, alias, limit, err := r.primary.ResolveProviderCredential(ctx, providerCode)
	if err == nil {
		return secret, alias, limit, nil
	}
	if !errors.Is(err, ErrNoActiveCredential) {
		return "", "", 0, err
	}
	fallback, ok := r.fallbacks[providerCode]
	if !ok || strings.TrimSpace(fallback.Secret) == "" {
		return "", "", 0, ErrNoActiveCredential
	}
	return fallback.Secret, fallback.CredentialAlias, fallback.DailyLimit, nil
}
