package providercredentials

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/emisell/api-kurir/internal/tenancy"
)

var (
	ErrUnsupportedProvider = errors.New("provider is not supported")
	ErrInvalidSecret       = errors.New("provider API key is invalid")
	ErrInvalidTenant       = errors.New("tenant ID is invalid")
	ErrInvalidDailyLimit   = errors.New("provider daily limit is invalid")
)

var supportedProviderCodes = map[string]struct{}{
	"rajaongkir": {},
	"biteship":   {},
}

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

func (s *Service) ListForTenant(ctx context.Context, tenantID string) ([]Credential, error) {
	tenantID = strings.TrimSpace(tenantID)
	if !validTenantID(tenantID) {
		return nil, ErrInvalidTenant
	}
	return s.repository.ListForTenant(ctx, tenantID)
}

func (s *Service) Add(
	ctx context.Context,
	providerCode, secret, actor, requestID string,
) (Credential, error) {
	return s.add(ctx, "", providerCode, secret, DefaultDailyLimit, actor, requestID)
}

func (s *Service) AddForTenant(
	ctx context.Context,
	tenantID, providerCode, secret string,
	dailyLimit int64,
	actor, requestID string,
) (Credential, error) {
	tenantID = strings.TrimSpace(tenantID)
	if !validTenantID(tenantID) {
		return Credential{}, ErrInvalidTenant
	}
	if dailyLimit == 0 {
		dailyLimit = DefaultDailyLimit
	}
	if dailyLimit < 1 || dailyLimit > 100_000_000 {
		return Credential{}, ErrInvalidDailyLimit
	}
	return s.add(ctx, tenantID, providerCode, secret, dailyLimit, actor, requestID)
}

func (s *Service) add(
	ctx context.Context,
	tenantID, providerCode, secret string,
	dailyLimit int64,
	actor, requestID string,
) (Credential, error) {
	providerCode = strings.ToLower(strings.TrimSpace(providerCode))
	secret = strings.TrimSpace(secret)
	if _, supported := supportedProviderCodes[providerCode]; !supported {
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
		ID:                  id,
		TenantID:            tenantID,
		ProviderCode:        providerCode,
		CredentialAlias:     alias,
		KeyPrefix:           secret[:prefixLength],
		KeyLastFour:         secret[len(secret)-4:],
		SecretCiphertext:    ciphertext,
		SecretFingerprint:   fingerprint[:],
		DailyLimit:          dailyLimit,
		ValidationQuotaCost: providerValidationQuotaCost(providerCode),
		CreatedBy:           actor,
		RequestID:           requestID,
	})
}

func providerValidationQuotaCost(providerCode string) int64 {
	if providerCode == "rajaongkir" {
		return 1
	}
	return 0
}

func (s *Service) Disable(ctx context.Context, id, actor, requestID string) error {
	return s.repository.Disable(ctx, id, actor, requestID)
}

func (s *Service) DisableForTenant(
	ctx context.Context,
	tenantID, id, actor, requestID string,
) error {
	tenantID = strings.TrimSpace(tenantID)
	if !validTenantID(tenantID) {
		return ErrInvalidTenant
	}
	return s.repository.DisableForTenant(ctx, tenantID, id, actor, requestID)
}

func (s *Service) ResolveProviderCredential(
	ctx context.Context,
	providerCode string,
) (string, string, int64, error) {
	providerCode = strings.ToLower(strings.TrimSpace(providerCode))
	if _, tenantRequest := tenancy.FromContext(ctx); tenantRequest &&
		tenancy.IntegrationID(ctx) == "" {
		// No integration ID means the merchant selected Emisell Kurir/free mode.
		// Never consume a paid seller credential implicitly.
		return "", "", 0, ErrNoActiveCredential
	}
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

func validTenantID(value string) bool {
	if len(value) < 1 || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			strings.ContainsRune("._:-", character) {
			continue
		}
		return false
	}
	return true
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
	// A tenant request must never borrow a platform or another seller's key.
	if _, tenantRequest := tenancy.FromContext(ctx); tenantRequest {
		return "", "", 0, ErrNoActiveCredential
	}
	fallback, ok := r.fallbacks[providerCode]
	if !ok || strings.TrimSpace(fallback.Secret) == "" {
		return "", "", 0, ErrNoActiveCredential
	}
	return fallback.Secret, fallback.CredentialAlias, fallback.DailyLimit, nil
}
