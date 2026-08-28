package providercredentials

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/emisell/api-kurir/internal/tenancy"
)

var (
	ErrUnsupportedProvider             = errors.New("provider is not supported")
	ErrInvalidSecret                   = errors.New("provider API key is invalid")
	ErrInvalidTenant                   = errors.New("tenant ID is invalid")
	ErrInvalidDailyLimit               = errors.New("provider daily limit is invalid")
	ErrCredentialCapabilityUnavailable = errors.New("provider credential capability is unavailable")
)

var supportedProviderCodes = map[string]struct{}{
	"rajaongkir": {},
	"biteship":   {},
}

type Validator interface {
	Validate(ctx context.Context, providerCode, secret string) error
}

type BundleValidator interface {
	ValidateCredentials(
		ctx context.Context,
		providerCode string,
		credentialType string,
		values map[string]string,
	) error
}

type encryptedCredentialBundle struct {
	Version int               `json:"version"`
	Type    string            `json:"type"`
	Values  map[string]string `json:"values"`
}

type Resolver interface {
	ResolveProviderCredential(
		ctx context.Context,
		providerCode string,
	) (secret, credentialAlias string, dailyLimit int64, err error)
}

type CapabilityResolver interface {
	ResolveProviderCredentialForCapability(
		ctx context.Context,
		providerCode string,
		capability string,
	) (secret, credentialAlias string, dailyLimit int64, err error)
}

// CapabilityEnvironmentResolver resolves both the credential and the provider
// execution environment. It lets trusted backend callers omit an environment
// header while keeping every subsequent shipment operation in the environment
// selected when the credential was installed.
type CapabilityEnvironmentResolver interface {
	ResolveProviderCredentialForCapabilityEnvironment(
		ctx context.Context,
		providerCode string,
		capability string,
	) (secret, credentialAlias, environment string, dailyLimit int64, err error)
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
	return s.AddForTenantCredentials(
		ctx, tenantID, providerCode,
		map[string]string{"api_key": secret},
		dailyLimit, actor, requestID,
	)
}

func (s *Service) AddForTenantCredentials(
	ctx context.Context,
	tenantID, providerCode string,
	values map[string]string,
	dailyLimit int64,
	actor, requestID string,
) (Credential, error) {
	return s.AddForTenantEnvironmentCredentials(
		ctx, tenantID, providerCode, EnvironmentLive, values,
		dailyLimit, actor, requestID,
	)
}

func (s *Service) AddForTenantEnvironmentCredentials(
	ctx context.Context,
	tenantID, providerCode, environment string,
	values map[string]string,
	dailyLimit int64,
	actor, requestID string,
) (Credential, error) {
	tenantID = strings.TrimSpace(tenantID)
	providerCode = strings.ToLower(strings.TrimSpace(providerCode))
	var validEnvironment bool
	environment, validEnvironment = NormalizeEnvironmentStrict(environment)
	if !validEnvironment {
		return Credential{}, ErrEnvironmentUnavailable
	}
	if !validTenantID(tenantID) {
		return Credential{}, ErrInvalidTenant
	}
	if dailyLimit == 0 {
		dailyLimit = DefaultDailyLimit
	}
	if dailyLimit < 1 || dailyLimit > 100_000_000 {
		return Credential{}, ErrInvalidDailyLimit
	}
	credentialType, fields, err := s.credentialDefinition(ctx, providerCode)
	if err != nil {
		return Credential{}, err
	}
	if credentialType == CredentialTypeCapabilityAPIKeys ||
		(credentialType == CredentialTypeProviderDeclared && providerCode == "rajaongkir") {
		values = normalizeLegacyShippingAPIKey(values)
		if values == nil {
			return Credential{}, ErrInvalidSecret
		}
	}
	normalized, err := NormalizeCredentialValuesForFields(fields, environment, values)
	if err != nil {
		return Credential{}, ErrInvalidSecret
	}
	validationContext := WithExecutionEnvironment(ctx, environment)
	if err := s.validateCredentialValues(validationContext, providerCode, credentialType, normalized); err != nil {
		return Credential{}, fmt.Errorf("%w: %v", ErrInvalidSecret, err)
	}
	payload, err := json.Marshal(encryptedCredentialBundle{
		Version: 1, Type: credentialType, Values: normalized,
	})
	if err != nil {
		return Credential{}, err
	}
	displaySource := credentialDisplaySource(credentialType, fields, normalized)
	return s.store(
		ctx, tenantID, providerCode, environment, payload, displaySource,
		dailyLimit, actor, requestID,
	)
}

func (s *Service) PatchForTenantEnvironmentCredentials(
	ctx context.Context,
	tenantID, providerCode, environment string,
	values map[string]string,
	dailyLimit *int64,
	actor, requestID string,
) (Credential, error) {
	tenantID = strings.TrimSpace(tenantID)
	providerCode = strings.ToLower(strings.TrimSpace(providerCode))
	var validEnvironment bool
	environment, validEnvironment = NormalizeEnvironmentStrict(environment)
	if !validEnvironment {
		return Credential{}, ErrEnvironmentUnavailable
	}
	if !validTenantID(tenantID) {
		return Credential{}, ErrInvalidTenant
	}
	if len(values) == 0 {
		return Credential{}, ErrInvalidSecret
	}
	stored, err := s.repository.ActiveStoredForTenantProvider(
		ctx, tenantID, providerCode, environment,
	)
	if err != nil {
		return Credential{}, err
	}
	credentialType, fields, err := s.credentialDefinition(ctx, providerCode)
	if err != nil {
		return Credential{}, err
	}
	existing, _, err := s.decryptStoredCredential(stored)
	if err != nil {
		return Credential{}, err
	}
	merged := make(map[string]string, len(existing)+len(values))
	for code, value := range existing {
		merged[code] = value
	}
	for code, value := range values {
		merged[code] = value
	}
	normalized, err := NormalizeCredentialValuesForFields(fields, environment, merged)
	if err != nil {
		return Credential{}, ErrInvalidSecret
	}
	validationContext := WithExecutionEnvironment(ctx, environment)
	if err := s.validatePatchedCredentialValues(
		validationContext, providerCode, credentialType, normalized, values,
	); err != nil {
		return Credential{}, fmt.Errorf("%w: %v", ErrInvalidSecret, err)
	}
	payload, err := json.Marshal(encryptedCredentialBundle{
		Version: 1, Type: credentialType, Values: normalized,
	})
	if err != nil {
		return Credential{}, err
	}
	limit := stored.DailyLimit
	if dailyLimit != nil {
		limit = *dailyLimit
	}
	if limit < 1 || limit > 100_000_000 {
		return Credential{}, ErrInvalidDailyLimit
	}
	displaySource := credentialDisplaySource(credentialType, fields, normalized)
	return s.store(
		ctx, tenantID, providerCode, environment, payload, displaySource,
		limit, actor, requestID,
	)
}

func (s *Service) credentialDefinition(
	ctx context.Context,
	providerCode string,
) (string, []FieldDefinition, error) {
	if repository, ok := s.repository.(SchemaRepository); ok {
		return repository.CredentialDefinition(ctx, providerCode)
	}
	credentialType, err := s.repository.CredentialType(ctx, providerCode)
	if err != nil {
		return "", nil, err
	}
	return credentialType, FieldsForCredentialType(credentialType), nil
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

	return s.store(
		ctx, tenantID, providerCode, EnvironmentLive, []byte(secret), secret,
		dailyLimit, actor, requestID,
	)
}

func (s *Service) store(
	ctx context.Context,
	tenantID, providerCode, environment string,
	payload []byte,
	displaySource string,
	dailyLimit int64,
	actor, requestID string,
) (Credential, error) {
	if len(payload) == 0 || len(displaySource) < 8 || len(displaySource) > 1024 {
		return Credential{}, ErrInvalidSecret
	}
	fingerprint := sha256.Sum256(payload)
	fingerprintHex := hex.EncodeToString(fingerprint[:])
	alias := providerCode + "-" + environment + "-" + fingerprintHex[:8]
	prefixLength := 4
	if len(displaySource)-4 < prefixLength {
		prefixLength = len(displaySource) - 4
	}
	if prefixLength < 1 {
		return Credential{}, ErrInvalidSecret
	}
	associatedData := []byte(providerCode + ":" + fingerprintHex)
	ciphertext, err := s.cipher.Encrypt(payload, associatedData)
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
		Environment:         environment,
		CredentialAlias:     alias,
		KeyPrefix:           displaySource[:prefixLength],
		KeyLastFour:         displaySource[len(displaySource)-4:],
		SecretCiphertext:    ciphertext,
		SecretFingerprint:   fingerprint[:],
		DailyLimit:          dailyLimit,
		ValidationQuotaCost: providerValidationQuotaCost(providerCode),
		CreatedBy:           actor,
		RequestID:           requestID,
	})
}

func (s *Service) validateCredentialValues(
	ctx context.Context,
	providerCode string,
	credentialType string,
	values map[string]string,
) error {
	if credentialType == CredentialTypeProviderDeclared {
		// Provider packages own the safe field declaration. RajaOngkir has an
		// authoritative validator for both Shipping Cost and Shipping Delivery,
		// so neither key may be accepted without a provider-side check.
		if providerCode == "rajaongkir" {
			if validator, ok := s.validator.(BundleValidator); ok {
				return validator.ValidateCredentials(ctx, providerCode, credentialType, values)
			}
			return ErrUnsupportedProvider
		}
		return nil
	}
	if validator, ok := s.validator.(BundleValidator); ok {
		return validator.ValidateCredentials(ctx, providerCode, credentialType, values)
	}
	var secret string
	switch credentialType {
	case CredentialTypeAPIKey:
		secret = values["api_key"]
	case CredentialTypeCapabilityAPIKeys:
		secret = values["shipping_api_key"]
	case CredentialTypeBearerToken:
		secret = values["token"]
	default:
		return ErrUnsupportedProvider
	}
	return s.validator.Validate(ctx, providerCode, secret)
}

func (s *Service) validatePatchedCredentialValues(
	ctx context.Context,
	providerCode string,
	credentialType string,
	values map[string]string,
	changed map[string]string,
) error {
	if credentialType == CredentialTypeProviderDeclared && providerCode == "rajaongkir" {
		validator, ok := s.validator.(BundleValidator)
		if !ok {
			return ErrUnsupportedProvider
		}
		changedValues := make(map[string]string, len(changed))
		for code := range changed {
			if value, exists := values[code]; exists {
				changedValues[code] = value
			}
		}
		if len(changedValues) == 0 {
			return ErrInvalidSecret
		}
		return validator.ValidateCredentials(
			ctx, providerCode, credentialType, changedValues,
		)
	}
	return s.validateCredentialValues(ctx, providerCode, credentialType, values)
}

func (s *Service) AvailableCredentialFieldsForTenant(
	ctx context.Context,
	tenantID, providerCode string,
) (Availability, error) {
	tenantID = strings.TrimSpace(tenantID)
	providerCode = strings.ToLower(strings.TrimSpace(providerCode))
	if !validTenantID(tenantID) {
		return Availability{}, ErrInvalidTenant
	}
	if _, _, err := s.credentialDefinition(ctx, providerCode); err != nil {
		return Availability{}, err
	}
	result := Availability{
		FieldsByEnvironment: make(map[string][]string),
		StatusByEnvironment: make(map[string]string),
	}
	for _, environment := range []string{EnvironmentLive, EnvironmentSandbox} {
		stored, err := s.repository.ActiveStoredForTenantProvider(
			ctx, tenantID, providerCode, environment,
		)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return Availability{}, err
		}
		values, _, err := s.decryptStoredCredential(stored)
		if err != nil {
			return Availability{}, err
		}
		codes := make([]string, 0, len(values))
		for code, value := range values {
			if strings.TrimSpace(value) != "" {
				codes = append(codes, code)
			}
		}
		sort.Strings(codes)
		result.FieldsByEnvironment[environment] = codes
		result.StatusByEnvironment[environment] = stored.ValidationStatus
	}
	return result, nil
}

func credentialDisplaySource(
	credentialType string,
	fields []FieldDefinition,
	values map[string]string,
) string {
	switch credentialType {
	case CredentialTypeOAuth2ClientCredentials:
		return values["client_id"]
	case CredentialTypeAPIKeySecret:
		return values["api_key"]
	case CredentialTypeCapabilityAPIKeys:
		if value := values["shipping_api_key"]; value != "" {
			return value
		}
		return values["delivery_api_key"]
	case CredentialTypeBearerToken:
		return values["token"]
	default:
		if value := values["api_key"]; value != "" {
			return value
		}
		for _, field := range fields {
			if !field.Secret && values[field.Code] != "" {
				return values[field.Code]
			}
		}
		codes := make([]string, 0, len(values))
		for code := range values {
			codes = append(codes, code)
		}
		sort.Strings(codes)
		for _, code := range codes {
			if values[code] != "" {
				return values[code]
			}
		}
		return ""
	}
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
	if _, err := s.repository.CredentialType(ctx, providerCode); err != nil {
		return err
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
	values, credentialType, alias, dailyLimit, err := s.ResolveProviderCredentialValues(
		ctx, providerCode,
	)
	if err != nil {
		return "", "", 0, err
	}
	var secret string
	switch credentialType {
	case CredentialTypeAPIKey:
		secret = values["api_key"]
	case CredentialTypeCapabilityAPIKeys:
		secret = values["shipping_api_key"]
	case CredentialTypeBearerToken:
		secret = values["token"]
	default:
		return "", "", 0, ErrUnsupportedProvider
	}
	if secret == "" {
		return "", "", 0, ErrInvalidSecret
	}
	return secret, alias, dailyLimit, nil
}

func (s *Service) ResolveProviderCredentialForCapability(
	ctx context.Context,
	providerCode string,
	capability string,
) (string, string, int64, error) {
	providerCode = strings.ToLower(strings.TrimSpace(providerCode))
	capability = strings.ToLower(strings.TrimSpace(capability))
	if repository, ok := s.repository.(SchemaRepository); ok {
		credentialEnvironment, resolveErr := repository.CredentialEnvironment(
			ctx, providerCode, capability, ExecutionEnvironment(ctx),
		)
		if resolveErr != nil {
			return "", "", 0, resolveErr
		}
		ctx = WithExecutionEnvironment(ctx, credentialEnvironment)
	}
	values, credentialType, alias, dailyLimit, err := s.ResolveProviderCredentialValues(
		ctx, providerCode,
	)
	if err != nil {
		return "", "", 0, err
	}
	var secret string
	switch credentialType {
	case CredentialTypeCapabilityAPIKeys:
		switch capability {
		case "rates:read", "tracking:read":
			secret = values["shipping_api_key"]
		case "shipments:write", "shipments:read", "pickup:write", "labels:read", "shipments:cancel":
			secret = values["delivery_api_key"]
		}
	case CredentialTypeAPIKey:
		if capability == "rates:read" || capability == "tracking:read" {
			secret = values["api_key"]
		}
	case CredentialTypeBearerToken:
		secret = values["token"]
	case CredentialTypeProviderDeclared:
		_, fields, definitionErr := s.credentialDefinition(ctx, providerCode)
		if definitionErr != nil {
			return "", "", 0, definitionErr
		}
		for _, field := range FieldsForEnvironment(fields, ExecutionEnvironment(ctx)) {
			if len(field.Capabilities) == 0 || containsString(field.Capabilities, capability) {
				if value := values[field.Code]; value != "" {
					secret = value
					break
				}
			}
		}
	}
	if secret == "" {
		return "", "", 0, ErrCredentialCapabilityUnavailable
	}
	return secret, alias, dailyLimit, nil
}

func (s *Service) ResolveProviderCredentialForCapabilityEnvironment(
	ctx context.Context,
	providerCode string,
	capability string,
) (string, string, string, int64, error) {
	environments := []string{EnvironmentLive, EnvironmentSandbox}
	if requested, explicit := RequestedExecutionEnvironment(ctx); explicit {
		environments = []string{requested}
	}
	var lastErr error
	for _, environment := range environments {
		secret, alias, limit, err := s.ResolveProviderCredentialForCapability(
			WithExecutionEnvironment(ctx, environment), providerCode, capability,
		)
		if err == nil {
			return secret, alias, environment, limit, nil
		}
		if !credentialSelectionError(err) {
			return "", "", "", 0, err
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = ErrNoActiveCredential
	}
	return "", "", "", 0, lastErr
}

func credentialSelectionError(err error) bool {
	return errors.Is(err, ErrNoActiveCredential) ||
		errors.Is(err, ErrCredentialCapabilityUnavailable) ||
		errors.Is(err, ErrEnvironmentUnavailable) ||
		errors.Is(err, ErrAllCredentialsExhausted)
}

func (s *Service) ResolveProviderCredentialValues(
	ctx context.Context,
	providerCode string,
) (map[string]string, string, string, int64, error) {
	providerCode = strings.ToLower(strings.TrimSpace(providerCode))
	stored, err := s.repository.ResolveActive(ctx, providerCode)
	if err != nil {
		return nil, "", "", 0, err
	}
	values, credentialType, err := s.decryptStoredCredential(stored)
	if err != nil {
		return nil, "", "", 0, err
	}
	return values, credentialType, stored.CredentialAlias, stored.DailyLimit, nil
}

func (s *Service) decryptStoredCredential(
	stored StoredCredential,
) (map[string]string, string, error) {
	fingerprintHex := hex.EncodeToString(stored.Fingerprint)
	plaintext, err := s.cipher.Decrypt(
		stored.SecretCiphertext,
		[]byte(stored.ProviderCode+":"+fingerprintHex),
	)
	if err != nil {
		return nil, "", err
	}
	var bundle encryptedCredentialBundle
	if json.Unmarshal(plaintext, &bundle) == nil && bundle.Version == 1 && bundle.Type != "" {
		return bundle.Values, bundle.Type, nil
	}
	return map[string]string{"api_key": string(plaintext)}, CredentialTypeAPIKey, nil
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
	Secret            string
	CapabilitySecrets map[string]string
	CredentialAlias   string
	DailyLimit        int64
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

func (r *StaticFallbackResolver) ResolveProviderCredentialForCapability(
	ctx context.Context,
	providerCode string,
	capability string,
) (string, string, int64, error) {
	if primary, ok := r.primary.(CapabilityResolver); ok {
		secret, alias, limit, err := primary.ResolveProviderCredentialForCapability(
			ctx, providerCode, capability,
		)
		if err == nil {
			return secret, alias, limit, nil
		}
		if !errors.Is(err, ErrNoActiveCredential) &&
			!errors.Is(err, ErrCredentialCapabilityUnavailable) {
			return "", "", 0, err
		}
		if identity, tenantRequest := tenancy.FromContext(ctx); tenantRequest {
			if identity.ProviderCredentialID != "" || r.platformAuthorizer == nil {
				return "", "", 0, err
			}
			allowed, authorizeErr := r.platformAuthorizer.AllowsPlatformCredential(ctx, identity.TenantID)
			if authorizeErr != nil {
				return "", "", 0, authorizeErr
			}
			if !allowed {
				return "", "", 0, err
			}
			secret, alias, limit, platformErr := primary.ResolveProviderCredentialForCapability(
				tenancy.WithoutIdentity(ctx), providerCode, capability,
			)
			if platformErr == nil {
				return secret, alias, limit, nil
			}
			if !errors.Is(platformErr, ErrNoActiveCredential) &&
				!errors.Is(platformErr, ErrCredentialCapabilityUnavailable) {
				return "", "", 0, platformErr
			}
		}
	}
	if identity, tenantRequest := tenancy.FromContext(ctx); tenantRequest {
		if identity.ProviderCredentialID != "" || r.platformAuthorizer == nil {
			return "", "", 0, ErrNoActiveCredential
		}
		allowed, err := r.platformAuthorizer.AllowsPlatformCredential(ctx, identity.TenantID)
		if err != nil {
			return "", "", 0, err
		}
		if !allowed {
			return "", "", 0, ErrNoActiveCredential
		}
	}
	fallback, ok := r.fallbacks[providerCode]
	if !ok {
		return "", "", 0, ErrNoActiveCredential
	}
	secret := strings.TrimSpace(fallback.CapabilitySecrets[capability])
	if secret == "" && (capability == "rates:read" || capability == "tracking:read") {
		secret = strings.TrimSpace(fallback.Secret)
	}
	if secret == "" {
		return "", "", 0, ErrCredentialCapabilityUnavailable
	}
	return secret, fallback.CredentialAlias, fallback.DailyLimit, nil
}

func (r *StaticFallbackResolver) ResolveProviderCredentialForCapabilityEnvironment(
	ctx context.Context,
	providerCode string,
	capability string,
) (string, string, string, int64, error) {
	environments := []string{EnvironmentLive, EnvironmentSandbox}
	if requested, explicit := RequestedExecutionEnvironment(ctx); explicit {
		environments = []string{requested}
	}
	var lastErr error
	for _, environment := range environments {
		secret, alias, limit, err := r.ResolveProviderCredentialForCapability(
			WithExecutionEnvironment(ctx, environment), providerCode, capability,
		)
		if err == nil {
			return secret, alias, environment, limit, nil
		}
		if !credentialSelectionError(err) {
			return "", "", "", 0, err
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = ErrNoActiveCredential
	}
	return "", "", "", 0, lastErr
}
