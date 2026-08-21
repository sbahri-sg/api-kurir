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

var tenantProviderCodes = map[string]struct{}{
	"rajaongkir": {},
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
	providerCode = strings.ToLower(strings.TrimSpace(providerCode))
	if !validTenantID(tenantID) {
		return Credential{}, ErrInvalidTenant
	}
	if _, supported := tenantProviderCodes[providerCode]; !supported {
		return Credential{}, ErrUnsupportedProvider
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

func (s *Service) DisableForTenantProvider(
	ctx context.Context,
	tenantID, providerCode, actor, requestID string,
) error {
	tenantID = strings.TrimSpace(tenantID)
	providerCode = strings.ToLower(strings.TrimSpace(providerCode))
	if !validTenantID(tenantID) {
		return ErrInvalidTenant
	}
	if _, supported := tenantProviderCodes[providerCode]; !supported {
		return ErrUnsupportedProvider
	}
	return s.repository.DisableForTenantProvider(
		ctx,
		tenantID,
		providerCode,
		actor,
		requestID,
	)
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

// ActiveCredentialID resolves the credential selected internally for a
// merchant. It is intentionally not part of the external Emisell contract.
func (s *Service) ActiveCredentialID(
	ctx context.Context,
	tenantID, providerCode string,
) (string, error) {
	tenantID = strings.TrimSpace(tenantID)
	providerCode = strings.ToLower(strings.TrimSpace(providerCode))
	if !validTenantID(tenantID) {
		return "", ErrInvalidTenant
	}
	if _, supported := supportedProviderCodes[providerCode]; !supported {
		return "", ErrUnsupportedProvider
	}
	return s.repository.ActiveCredentialID(ctx, tenantID, providerCode)
}

// SelectedCredentialID is used by rate snapshots. An empty value means the
// merchant is on the built-in/free provider and therefore has no paid
// credential to pin.
func (s *Service) SelectedCredentialID(
	ctx context.Context,
	tenantID, providerCode string,
) (string, error) {
	id, err := s.ActiveCredentialID(ctx, tenantID, providerCode)
	if errors.Is(err, ErrNoActiveCredential) {
		return "", nil
	}
	return id, err
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
	primary            Resolver
	fallbacks          map[string]StaticCredential
	platformAuthorizer PlatformCredentialAuthorizer
}

type PlatformCredentialAuthorizer interface {
	AllowsPlatformCredential(ctx context.Context, tenantID string) (bool, error)
}

type StaticCredential struct {
	Secret          string
	CredentialAlias string
	DailyLimit      int64
}

func NewStaticFallbackResolver(
	primary Resolver,
	fallbacks map[string]StaticCredential,
	platformAuthorizers ...PlatformCredentialAuthorizer,
) *StaticFallbackResolver {
	resolver := &StaticFallbackResolver{primary: primary, fallbacks: fallbacks}
	if len(platformAuthorizers) > 0 {
		resolver.platformAuthorizer = platformAuthorizers[0]
	}
	return resolver
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
	// A tenant can use the shared platform pool only when it explicitly selected
	// the built-in Emisell provider. BYOK providers remain strictly isolated.
	if identity, tenantRequest := tenancy.FromContext(ctx); tenantRequest {
		if identity.ProviderCredentialID != "" || r.platformAuthorizer == nil {
			return "", "", 0, ErrNoActiveCredential
		}
		allowed, authorizeErr := r.platformAuthorizer.AllowsPlatformCredential(
			ctx,
			identity.TenantID,
		)
		if authorizeErr != nil {
			return "", "", 0, authorizeErr
		}
		if !allowed {
			return "", "", 0, ErrNoActiveCredential
		}
		secret, alias, limit, err = r.primary.ResolveProviderCredential(
			tenancy.WithoutIdentity(ctx),
			providerCode,
		)
		if err == nil {
			return secret, alias, limit, nil
		}
		if !errors.Is(err, ErrNoActiveCredential) {
			return "", "", 0, err
		}
	}
	fallback, ok := r.fallbacks[providerCode]
	if !ok || strings.TrimSpace(fallback.Secret) == "" {
		return "", "", 0, ErrNoActiveCredential
	}
	return fallback.Secret, fallback.CredentialAlias, fallback.DailyLimit, nil
}
