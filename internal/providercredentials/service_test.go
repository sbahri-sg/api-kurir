package providercredentials

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/emisell/api-kurir/internal/tenancy"
)

const testEncryptionKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

type memoryRepository struct {
	input           CreateInput
	item            Credential
	disabled        bool
	credentialTypes map[string]string
}

func (r *memoryRepository) CredentialType(_ context.Context, providerCode string) (string, error) {
	if value, exists := r.credentialTypes[providerCode]; exists {
		return value, nil
	}
	if providerCode == "biteship" {
		return "", ErrUnsupportedProvider
	}
	return CredentialTypeAPIKey, nil
}

func (r *memoryRepository) List(context.Context) ([]Credential, error) {
	if r.item.ID == "" {
		return []Credential{}, nil
	}
	return []Credential{r.item}, nil
}

func (r *memoryRepository) ListForTenant(_ context.Context, tenantID string) ([]Credential, error) {
	if r.item.ID == "" || r.item.TenantID != tenantID {
		return []Credential{}, nil
	}
	return []Credential{r.item}, nil
}

func (r *memoryRepository) Create(_ context.Context, input CreateInput) (Credential, error) {
	r.input = input
	r.item = Credential{
		ID: input.ID, TenantID: input.TenantID, ProviderCode: input.ProviderCode,
		Environment:     input.Environment,
		CredentialAlias: input.CredentialAlias,
		DisplayKey:      input.KeyPrefix + "••••" + input.KeyLastFour,
		DailyLimit:      input.DailyLimit, Active: true,
		ValidationStatus: "valid", LastValidatedAt: time.Now(),
		CreatedAt: time.Now(),
	}
	return r.item, nil
}

type declarativeMemoryRepository struct {
	memoryRepository
	fields []FieldDefinition
}

type livePolicyDeclarativeMemoryRepository struct {
	declarativeMemoryRepository
}

func (r *livePolicyDeclarativeMemoryRepository) CredentialEnvironment(
	_ context.Context,
	_ string,
	_ string,
	_ string,
) (string, error) {
	return EnvironmentLive, nil
}

func (r *declarativeMemoryRepository) CredentialDefinition(
	context.Context,
	string,
) (string, []FieldDefinition, error) {
	return CredentialTypeProviderDeclared, r.fields, nil
}

func (r *declarativeMemoryRepository) CredentialEnvironment(
	_ context.Context,
	_ string,
	_ string,
	executionEnvironment string,
) (string, error) {
	return NormalizeEnvironment(executionEnvironment), nil
}

func (r *memoryRepository) Disable(context.Context, string, string, string) error {
	r.disabled = true
	r.item.Active = false
	return nil
}

func (r *memoryRepository) DisableForTenantProvider(
	_ context.Context,
	tenantID, providerCode string,
	_ string,
	_ string,
) error {
	if r.item.TenantID != tenantID || r.item.ProviderCode != providerCode {
		return ErrNotFound
	}
	r.disabled = true
	r.item.Active = false
	return nil
}

func (r *memoryRepository) ActiveStoredForTenantProvider(
	_ context.Context,
	tenantID, providerCode, environment string,
) (StoredCredential, error) {
	if r.item.ID == "" || r.disabled || r.item.TenantID != tenantID ||
		r.item.ProviderCode != providerCode || r.item.Environment != NormalizeEnvironment(environment) {
		return StoredCredential{}, ErrNotFound
	}
	return StoredCredential{
		Credential:       r.item,
		SecretCiphertext: r.input.SecretCiphertext,
		Fingerprint:      r.input.SecretFingerprint,
	}, nil
}

func TestTenantCannotInstallInternalFallbackProvider(t *testing.T) {
	t.Parallel()

	cipher, err := NewCipher(testEncryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(&memoryRepository{}, cipher, &acceptingValidator{})
	_, err = service.AddForTenant(
		context.Background(),
		"merchant_123",
		"biteship",
		"biteship_test.provider-secret",
		50_000,
		"tenant:merchant_123",
		"req_test",
	)
	if !errors.Is(err, ErrUnsupportedProvider) {
		t.Fatalf("expected internal fallback provider rejection, got %v", err)
	}
}

func (r *memoryRepository) ActiveCredentialID(
	_ context.Context,
	tenantID, providerCode string,
) (string, error) {
	if r.item.ID == "" || r.disabled || r.item.TenantID != tenantID ||
		r.item.ProviderCode != providerCode {
		return "", ErrNoActiveCredential
	}
	return r.item.ID, nil
}

func (r *memoryRepository) ResolveActive(ctx context.Context, _ string) (StoredCredential, error) {
	if r.item.ID == "" || r.disabled ||
		r.item.Environment != ExecutionEnvironment(ctx) {
		return StoredCredential{}, ErrNoActiveCredential
	}
	return StoredCredential{
		Credential:       r.item,
		SecretCiphertext: r.input.SecretCiphertext,
		Fingerprint:      r.input.SecretFingerprint,
	}, nil
}

type acceptingValidator struct {
	calls int
	err   error
}

type acceptingBundleValidator struct {
	acceptingValidator
	credentialType string
	values         map[string]string
}

func (v *acceptingBundleValidator) ValidateCredentials(
	_ context.Context,
	_ string,
	credentialType string,
	values map[string]string,
) error {
	v.calls++
	v.credentialType = credentialType
	v.values = make(map[string]string, len(values))
	for code, value := range values {
		v.values[code] = value
	}
	return v.err
}

func (v *acceptingValidator) Validate(context.Context, string, string) error {
	v.calls++
	return v.err
}

func TestAddEncryptsProviderSecretAndResolverDecryptsIt(t *testing.T) {
	t.Parallel()

	cipher, err := NewCipher(testEncryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	repository := &memoryRepository{}
	validator := &acceptingBundleValidator{}
	service := NewService(repository, cipher, validator)
	secret := "rajaongkir-provider-secret"

	item, err := service.Add(
		context.Background(),
		"rajaongkir",
		secret,
		"operator",
		"req_test",
	)
	if err != nil {
		t.Fatal(err)
	}
	if validator.calls != 1 {
		t.Fatalf("validator calls: got %d want 1", validator.calls)
	}
	if bytes.Contains(repository.input.SecretCiphertext, []byte(secret)) {
		t.Fatal("ciphertext must not contain plaintext provider secret")
	}
	if item.DisplayKey == secret || len(repository.input.SecretFingerprint) != 32 {
		t.Fatal("stored metadata must not expose plaintext and must contain SHA-256 fingerprint")
	}
	resolved, alias, limit, err := service.ResolveProviderCredential(
		context.Background(),
		"rajaongkir",
	)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != secret || alias != item.CredentialAlias || limit != DefaultDailyLimit {
		t.Fatalf("unexpected resolved credential metadata")
	}
}

func TestOAuthCredentialBundleIsEncryptedAndResolvedAsFields(t *testing.T) {
	t.Parallel()

	cipher, err := NewCipher(testEncryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	repository := &memoryRepository{credentialTypes: map[string]string{
		"oauth-provider": CredentialTypeOAuth2ClientCredentials,
	}}
	validator := &acceptingBundleValidator{}
	service := NewService(repository, cipher, validator)
	created, err := service.AddForTenantCredentials(
		context.Background(), "merchant_123", "oauth-provider",
		map[string]string{
			"client_id": "merchant-client-1234", "client_secret": "oauth-secret-value",
		},
		50_000, "tenant:merchant_123", "req_oauth",
	)
	if err != nil {
		t.Fatal(err)
	}
	if validator.credentialType != CredentialTypeOAuth2ClientCredentials ||
		validator.values["client_id"] != "merchant-client-1234" {
		t.Fatalf("validator did not receive credential bundle: %#v", validator)
	}
	if bytes.Contains(repository.input.SecretCiphertext, []byte("oauth-secret-value")) {
		t.Fatal("OAuth client secret was stored as plaintext")
	}
	if created.DisplayKey != "merc••••1234" {
		t.Fatalf("display key=%q", created.DisplayKey)
	}
	values, credentialType, _, _, err := service.ResolveProviderCredentialValues(
		context.Background(), "oauth-provider",
	)
	if err != nil {
		t.Fatal(err)
	}
	if credentialType != CredentialTypeOAuth2ClientCredentials ||
		values["client_secret"] != "oauth-secret-value" {
		t.Fatalf("resolved type=%q values=%#v", credentialType, values)
	}
}

func TestCapabilityAPIKeysResolveByOperation(t *testing.T) {
	t.Parallel()

	cipher, err := NewCipher(testEncryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	repository := &memoryRepository{credentialTypes: map[string]string{
		"rajaongkir": CredentialTypeCapabilityAPIKeys,
	}}
	validator := &acceptingBundleValidator{}
	service := NewService(repository, cipher, validator)
	_, err = service.AddForTenantCredentials(
		context.Background(), "merchant_123", "rajaongkir",
		map[string]string{
			"shipping_api_key": "shipping-production-key",
			"delivery_api_key": "delivery-production-key",
		},
		50_000, "tenant:merchant_123", "req_capabilities",
	)
	if err != nil {
		t.Fatal(err)
	}
	if validator.calls != 1 {
		t.Fatalf("shipping key validation calls=%d want 1", validator.calls)
	}
	shipping, _, _, err := service.ResolveProviderCredentialForCapability(
		context.Background(), "rajaongkir", "rates:read",
	)
	if err != nil || shipping != "shipping-production-key" {
		t.Fatalf("resolve shipping key=%q err=%v", shipping, err)
	}
	delivery, _, _, err := service.ResolveProviderCredentialForCapability(
		context.Background(), "rajaongkir", "pickup:write",
	)
	if err != nil || delivery != "delivery-production-key" {
		t.Fatalf("resolve delivery key=%q err=%v", delivery, err)
	}
	legacyResolverValue, _, _, err := service.ResolveProviderCredential(
		context.Background(), "rajaongkir",
	)
	if err != nil || legacyResolverValue != "shipping-production-key" {
		t.Fatalf("legacy resolver key=%q err=%v", legacyResolverValue, err)
	}
}

func TestCapabilityAPIKeysDoNotReuseShippingKeyForDelivery(t *testing.T) {
	t.Parallel()

	cipher, err := NewCipher(testEncryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	repository := &memoryRepository{credentialTypes: map[string]string{
		"rajaongkir": CredentialTypeCapabilityAPIKeys,
	}}
	service := NewService(repository, cipher, &acceptingBundleValidator{})
	_, err = service.AddForTenant(
		context.Background(), "merchant_123", "rajaongkir",
		"legacy-shipping-key", 50_000, "tenant:merchant_123", "req_legacy_shipping",
	)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, err = service.ResolveProviderCredentialForCapability(
		context.Background(), "rajaongkir", "shipments:write",
	)
	if !errors.Is(err, ErrCredentialCapabilityUnavailable) {
		t.Fatalf("expected unavailable delivery capability, got %v", err)
	}
}

func TestCapabilityShippingAndDeliveryCredentialCanBeStoredForSandbox(t *testing.T) {
	t.Parallel()

	cipher, err := NewCipher(testEncryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	repository := &memoryRepository{credentialTypes: map[string]string{
		"rajaongkir": CredentialTypeCapabilityAPIKeys,
	}}
	validator := &acceptingBundleValidator{}
	service := NewService(repository, cipher, validator)
	credential, err := service.AddForTenantEnvironmentCredentials(
		context.Background(), "merchant_123", "rajaongkir", EnvironmentSandbox,
		map[string]string{
			"shipping_api_key": "shipping-cost-key",
			"delivery_api_key": "sandbox-delivery-key",
		},
		50_000, "tenant:merchant_123", "req_sandbox_delivery",
	)
	if err != nil {
		t.Fatal(err)
	}
	if credential.Environment != EnvironmentSandbox || credential.DisplayKey == "" {
		t.Fatalf("unexpected sandbox credential: %#v", credential)
	}
	if validator.calls != 1 || validator.values["shipping_api_key"] != "shipping-cost-key" ||
		validator.values["delivery_api_key"] != "sandbox-delivery-key" {
		t.Fatalf("sandbox credential bundle was not validated: %#v", validator)
	}
	value, credentialType, _, _, err := service.ResolveProviderCredentialValues(
		WithExecutionEnvironment(context.Background(), EnvironmentSandbox), "rajaongkir",
	)
	if err != nil {
		t.Fatal(err)
	}
	if credentialType != CredentialTypeCapabilityAPIKeys ||
		value["shipping_api_key"] != "shipping-cost-key" ||
		value["delivery_api_key"] != "sandbox-delivery-key" {
		t.Fatalf("unexpected resolved credential: type=%q values=%#v", credentialType, value)
	}
}

func TestRajaOngkirSandboxShippingCostUsesSandboxBundleBeforeLivePolicy(t *testing.T) {
	t.Parallel()

	cipher, err := NewCipher(testEncryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	repository := &livePolicyDeclarativeMemoryRepository{
		declarativeMemoryRepository: declarativeMemoryRepository{fields: []FieldDefinition{
			{
				Code: "shipping_api_key", Label: "Shipping key", InputType: "password",
				Secret: true, Required: true, Capabilities: []string{"rates:read", "tracking:read"},
				Environments: []string{EnvironmentLive, EnvironmentSandbox},
			},
			{
				Code: "delivery_api_key", Label: "Delivery key", InputType: "password",
				Secret: true, Required: false, Capabilities: []string{"shipments:write"},
				Environments: []string{EnvironmentLive, EnvironmentSandbox},
			},
		}},
	}
	service := NewService(repository, cipher, &acceptingBundleValidator{})
	_, err = service.AddForTenantEnvironmentCredentials(
		context.Background(), "merchant_123", "rajaongkir", EnvironmentSandbox,
		map[string]string{
			"shipping_api_key": "shipping-cost-key",
			"delivery_api_key": "sandbox-delivery-key",
		},
		50_000, "tenant:merchant_123", "req_sandbox_bundle",
	)
	if err != nil {
		t.Fatal(err)
	}
	secret, _, _, err := service.ResolveProviderCredentialForCapability(
		WithExecutionEnvironment(context.Background(), EnvironmentSandbox),
		"rajaongkir", "rates:read",
	)
	if err != nil || secret != "shipping-cost-key" {
		t.Fatalf("sandbox shipping key=%q err=%v", secret, err)
	}
}

func TestCapabilityEnvironmentResolverSelectsInstalledSandboxCredential(t *testing.T) {
	t.Parallel()

	cipher, err := NewCipher(testEncryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	repository := &memoryRepository{credentialTypes: map[string]string{
		"rajaongkir": CredentialTypeCapabilityAPIKeys,
	}}
	service := NewService(repository, cipher, &acceptingBundleValidator{})
	_, err = service.AddForTenantEnvironmentCredentials(
		context.Background(), "merchant_123", "rajaongkir", EnvironmentSandbox,
		map[string]string{
			"shipping_api_key": "shipping-cost-key",
			"delivery_api_key": "sandbox-delivery-key",
		},
		50_000, "tenant:merchant_123", "req_sandbox_auto",
	)
	if err != nil {
		t.Fatal(err)
	}
	secret, _, environment, _, err := service.ResolveProviderCredentialForCapabilityEnvironment(
		context.Background(), "rajaongkir", "shipments:write",
	)
	if err != nil || secret != "sandbox-delivery-key" || environment != EnvironmentSandbox {
		t.Fatalf("secret=%q environment=%q err=%v", secret, environment, err)
	}
}

func TestProviderDeclaredSandboxCredentialUsesSandboxFieldOnly(t *testing.T) {
	t.Parallel()

	cipher, err := NewCipher(testEncryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	repository := &declarativeMemoryRepository{fields: []FieldDefinition{
		{
			Code: "shipping_api_key", Label: "Shipping key", InputType: "password",
			Secret: true, Required: true, Capabilities: []string{"rates:read"},
			Environments: []string{EnvironmentLive},
		},
		{
			Code: "delivery_api_key", Label: "Delivery key", InputType: "password",
			Secret: true, Required: true, Capabilities: []string{"shipments:write"},
			Environments: []string{EnvironmentSandbox},
		},
	}}
	service := NewService(repository, cipher, &acceptingBundleValidator{})
	_, err = service.AddForTenantEnvironmentCredentials(
		context.Background(), "merchant_123", "provider-hosted", EnvironmentSandbox,
		map[string]string{"delivery_api_key": "sandbox-delivery-key"},
		50_000, "tenant:merchant_123", "req_sandbox",
	)
	if err != nil {
		t.Fatal(err)
	}
	if repository.input.Environment != EnvironmentSandbox {
		t.Fatalf("environment=%q", repository.input.Environment)
	}
	if _, exists := repository.inputSecretValue(t, cipher, "shipping_api_key"); exists {
		t.Fatal("live field must not be stored in sandbox credential")
	}
	if value, exists := repository.inputSecretValue(t, cipher, "delivery_api_key"); !exists || value != "sandbox-delivery-key" {
		t.Fatalf("sandbox delivery credential missing: value=%q exists=%t", value, exists)
	}
}

func TestProviderDeclaredRajaOngkirAcceptsLegacyAPIKey(t *testing.T) {
	t.Parallel()

	cipher, err := NewCipher(testEncryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	repository := &declarativeMemoryRepository{fields: []FieldDefinition{{
		Code: "shipping_api_key", Label: "Shipping key", InputType: "password",
		Secret: true, Required: true, Capabilities: []string{"rates:read"},
		Environments: []string{EnvironmentLive},
	}}}
	service := NewService(repository, cipher, &acceptingBundleValidator{})
	_, err = service.AddForTenantEnvironmentCredentials(
		context.Background(), "merchant_123", "rajaongkir", EnvironmentLive,
		map[string]string{"api_key": "legacy-shipping-key"},
		50_000, "tenant:merchant_123", "req_legacy",
	)
	if err != nil {
		t.Fatal(err)
	}
	value, exists := repository.inputSecretValue(t, cipher, "shipping_api_key")
	if !exists || value != "legacy-shipping-key" {
		t.Fatalf("shipping credential value=%q exists=%t", value, exists)
	}
}

func TestPatchProviderDeclaredCredentialMergesOptionalDeliveryKey(t *testing.T) {
	t.Parallel()

	cipher, err := NewCipher(testEncryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	repository := &declarativeMemoryRepository{fields: []FieldDefinition{
		{
			Code: "shipping_api_key", Label: "Shipping key", InputType: "password",
			Secret: true, Required: true, Capabilities: []string{"rates:read"},
			Environments: []string{EnvironmentLive},
		},
		{
			Code: "delivery_api_key", Label: "Delivery key", InputType: "password",
			Secret: true, Required: false, Capabilities: []string{"pickup:write"},
			Environments: []string{EnvironmentLive, EnvironmentSandbox},
		},
	}}
	validator := &acceptingBundleValidator{}
	service := NewService(repository, cipher, validator)
	_, err = service.AddForTenantEnvironmentCredentials(
		context.Background(), "merchant_123", "rajaongkir", EnvironmentLive,
		map[string]string{"shipping_api_key": "shipping-live-key"},
		50_000, "tenant:merchant_123", "req_create",
	)
	if err != nil {
		t.Fatal(err)
	}
	if validator.calls != 1 {
		t.Fatalf("initial validation calls=%d want=1", validator.calls)
	}
	updated, err := service.PatchForTenantEnvironmentCredentials(
		context.Background(), "merchant_123", "rajaongkir", EnvironmentLive,
		map[string]string{"delivery_api_key": "delivery-live-key"},
		nil, "tenant:merchant_123", "req_patch",
	)
	if err != nil {
		t.Fatal(err)
	}
	if updated.DailyLimit != 50_000 || validator.calls != 2 {
		t.Fatalf("updated=%#v validation calls=%d", updated, validator.calls)
	}
	if len(validator.values) != 1 || validator.values["delivery_api_key"] != "delivery-live-key" {
		t.Fatalf("patch should validate only changed key: %#v", validator.values)
	}
	shipping, shippingExists := repository.inputSecretValue(t, cipher, "shipping_api_key")
	delivery, deliveryExists := repository.inputSecretValue(t, cipher, "delivery_api_key")
	if !shippingExists || shipping != "shipping-live-key" ||
		!deliveryExists || delivery != "delivery-live-key" {
		t.Fatalf("merged fields shipping=%q/%t delivery=%q/%t", shipping, shippingExists, delivery, deliveryExists)
	}
	availability, err := service.AvailableCredentialFieldsForTenant(
		context.Background(), "merchant_123", "rajaongkir",
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := availability.FieldsByEnvironment[EnvironmentLive]; len(got) != 2 || got[0] != "delivery_api_key" || got[1] != "shipping_api_key" {
		t.Fatalf("available live fields=%#v", got)
	}
}

func TestProviderDeclaredDeliveryKeyValidationRejectsPostAndPatchWithoutReplacingStoredBundle(t *testing.T) {
	t.Parallel()

	cipher, err := NewCipher(testEncryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	fields := []FieldDefinition{
		{
			Code: "shipping_api_key", Label: "Shipping key", InputType: "password",
			Secret: true, Required: true, Capabilities: []string{"rates:read"},
			Environments: []string{EnvironmentLive},
		},
		{
			Code: "delivery_api_key", Label: "Delivery key", InputType: "password",
			Secret: true, Required: false, Capabilities: []string{"shipments:write"},
			Environments: []string{EnvironmentLive, EnvironmentSandbox},
		},
	}

	invalidPostRepository := &declarativeMemoryRepository{fields: fields}
	invalidPostValidator := &acceptingBundleValidator{acceptingValidator: acceptingValidator{
		err: errors.New("unauthorized"),
	}}
	invalidPostService := NewService(invalidPostRepository, cipher, invalidPostValidator)
	_, err = invalidPostService.AddForTenantEnvironmentCredentials(
		context.Background(), "merchant_123", "rajaongkir", EnvironmentSandbox,
		map[string]string{"delivery_api_key": "invalid-delivery-key"},
		50_000, "tenant:merchant_123", "req_invalid_post",
	)
	if !errors.Is(err, ErrInvalidSecret) || invalidPostRepository.item.ID != "" {
		t.Fatalf("invalid POST must be rejected before storage: item=%+v err=%v", invalidPostRepository.item, err)
	}

	patchRepository := &declarativeMemoryRepository{fields: fields}
	patchValidator := &acceptingBundleValidator{}
	patchService := NewService(patchRepository, cipher, patchValidator)
	_, err = patchService.AddForTenantEnvironmentCredentials(
		context.Background(), "merchant_123", "rajaongkir", EnvironmentLive,
		map[string]string{"shipping_api_key": "valid-shipping-key"},
		50_000, "tenant:merchant_123", "req_valid_create",
	)
	if err != nil {
		t.Fatal(err)
	}
	storedCiphertext := append([]byte(nil), patchRepository.input.SecretCiphertext...)
	patchValidator.err = errors.New("unauthorized")
	_, err = patchService.PatchForTenantEnvironmentCredentials(
		context.Background(), "merchant_123", "rajaongkir", EnvironmentLive,
		map[string]string{"delivery_api_key": "invalid-delivery-key"},
		nil, "tenant:merchant_123", "req_invalid_patch",
	)
	if !errors.Is(err, ErrInvalidSecret) ||
		!bytes.Equal(storedCiphertext, patchRepository.input.SecretCiphertext) {
		t.Fatalf("invalid PATCH replaced stored bundle: err=%v", err)
	}
}

func (r *declarativeMemoryRepository) inputSecretValue(
	t *testing.T,
	cipher *Cipher,
	code string,
) (string, bool) {
	t.Helper()
	fingerprintHex := hex.EncodeToString(r.input.SecretFingerprint)
	plaintext, err := cipher.Decrypt(
		r.input.SecretCiphertext,
		[]byte(r.input.ProviderCode+":"+fingerprintHex),
	)
	if err != nil {
		t.Fatal(err)
	}
	var bundle encryptedCredentialBundle
	if err := json.Unmarshal(plaintext, &bundle); err != nil {
		t.Fatal(err)
	}
	value, exists := bundle.Values[code]
	return value, exists
}

func TestBiteshipCredentialValidationDoesNotConsumeTrackingQuota(t *testing.T) {
	t.Parallel()

	cipher, err := NewCipher(testEncryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	repository := &memoryRepository{}
	service := NewService(repository, cipher, &acceptingValidator{})
	_, err = service.Add(
		context.Background(),
		"biteship",
		"biteship_test.provider-secret",
		"operator",
		"req_test",
	)
	if err != nil {
		t.Fatal(err)
	}
	if repository.input.ValidationQuotaCost != 0 {
		t.Fatalf(
			"Biteship catalog validation must not consume paid tracking quota: %d",
			repository.input.ValidationQuotaCost,
		)
	}
}

func TestAddRejectsCredentialWhenProviderValidationFails(t *testing.T) {
	t.Parallel()

	cipher, err := NewCipher(testEncryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(
		&memoryRepository{},
		cipher,
		&acceptingValidator{err: errors.New("unauthorized")},
	)
	_, err = service.Add(
		context.Background(),
		"rajaongkir",
		"invalid-provider-secret",
		"operator",
		"req_test",
	)
	if !errors.Is(err, ErrInvalidSecret) {
		t.Fatalf("expected invalid secret, got %v", err)
	}
}

func TestTenantCredentialIsOwnedAndUsesConfiguredLimit(t *testing.T) {
	t.Parallel()

	cipher, err := NewCipher(testEncryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	repository := &memoryRepository{}
	service := NewService(repository, cipher, &acceptingValidator{})
	item, err := service.AddForTenant(
		context.Background(),
		"merchant_123",
		"rajaongkir",
		"tenant-provider-secret",
		100_000,
		"tenant:merchant_123",
		"req_test",
	)
	if err != nil {
		t.Fatal(err)
	}
	if item.TenantID != "merchant_123" || item.DailyLimit != 100_000 {
		t.Fatalf("unexpected tenant credential: %#v", item)
	}
	listed, err := service.ListForTenant(context.Background(), "merchant_123")
	if err != nil || len(listed) != 1 {
		t.Fatalf("tenant credential list: items=%#v err=%v", listed, err)
	}
}

func TestTenantProviderResolutionUsesMerchantActiveCredential(t *testing.T) {
	t.Parallel()

	cipher, err := NewCipher(testEncryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	repository := &memoryRepository{}
	service := NewService(repository, cipher, &acceptingValidator{})
	_, err = service.AddForTenant(
		context.Background(),
		"merchant_123",
		"rajaongkir",
		"tenant-provider-secret",
		50_000,
		"tenant:merchant_123",
		"req_test",
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx := tenancy.WithIdentity(context.Background(), tenancy.Identity{TenantID: "merchant_123"})
	secret, _, _, err := service.ResolveProviderCredential(ctx, "rajaongkir")
	if err != nil || secret != "tenant-provider-secret" {
		t.Fatalf("merchant active credential was not resolved: secret=%q err=%v", secret, err)
	}
}

func TestStaticFallbackResolverPrefersDatabaseCredential(t *testing.T) {
	t.Parallel()

	primary := resolverStub{secret: "database", alias: "db-key", limit: 50_000}
	resolver := NewStaticFallbackResolver(primary, map[string]StaticCredential{
		"rajaongkir": {
			Secret: "environment", CredentialAlias: "env-key", DailyLimit: 50_000,
		},
	})
	secret, alias, _, err := resolver.ResolveProviderCredential(
		context.Background(),
		"rajaongkir",
	)
	if err != nil {
		t.Fatal(err)
	}
	if secret != "database" || alias != "db-key" {
		t.Fatalf("database credential must take priority")
	}
}

func TestStaticFallbackResolverDoesNotBypassExhaustedDatabasePool(t *testing.T) {
	t.Parallel()

	resolver := NewStaticFallbackResolver(
		resolverStub{err: ErrAllCredentialsExhausted},
		map[string]StaticCredential{
			"rajaongkir": {
				Secret: "environment", CredentialAlias: "env-key", DailyLimit: 50_000,
			},
		},
	)
	_, _, _, err := resolver.ResolveProviderCredential(
		context.Background(),
		"rajaongkir",
	)
	if !errors.Is(err, ErrAllCredentialsExhausted) {
		t.Fatalf("exhausted database pool must not fall back to environment: %v", err)
	}
}

func TestStaticFallbackResolverDoesNotLendPlatformKeyToTenant(t *testing.T) {
	t.Parallel()

	resolver := NewStaticFallbackResolver(
		resolverStub{err: ErrNoActiveCredential},
		map[string]StaticCredential{
			"rajaongkir": {
				Secret: "platform-key", CredentialAlias: "platform", DailyLimit: 50_000,
			},
		},
	)
	ctx := tenancy.WithIdentity(context.Background(), tenancy.Identity{TenantID: "merchant_123"})
	_, _, _, err := resolver.ResolveProviderCredential(ctx, "rajaongkir")
	if !errors.Is(err, ErrNoActiveCredential) {
		t.Fatalf("tenant request must not borrow platform key: %v", err)
	}
}

func TestStaticFallbackResolverAllowsBuiltInTenantToUsePlatformPool(t *testing.T) {
	t.Parallel()

	resolver := NewStaticFallbackResolver(
		tenantAwareResolver{},
		map[string]StaticCredential{
			"rajaongkir": {
				Secret: "environment", CredentialAlias: "env-key", DailyLimit: 50_000,
			},
		},
		platformAuthorizerStub{allowed: true},
	)
	ctx := tenancy.WithIdentity(context.Background(), tenancy.Identity{TenantID: "merchant_123"})
	secret, alias, limit, err := resolver.ResolveProviderCredential(ctx, "rajaongkir")
	if err != nil {
		t.Fatal(err)
	}
	if secret != "platform-database" || alias != "platform-db" || limit != 50_000 {
		t.Fatalf("unexpected platform credential: secret=%q alias=%q limit=%d", secret, alias, limit)
	}
}

func TestStaticFallbackResolverKeepsBYOKTenantIsolated(t *testing.T) {
	t.Parallel()

	resolver := NewStaticFallbackResolver(
		tenantAwareResolver{},
		map[string]StaticCredential{},
		platformAuthorizerStub{allowed: false},
	)
	ctx := tenancy.WithIdentity(context.Background(), tenancy.Identity{TenantID: "merchant_123"})
	_, _, _, err := resolver.ResolveProviderCredential(ctx, "rajaongkir")
	if !errors.Is(err, ErrNoActiveCredential) {
		t.Fatalf("BYOK tenant must remain isolated: %v", err)
	}
}

func TestStaticFallbackResolverUsesSeparateDeliveryCredentialForBuiltInTenant(t *testing.T) {
	t.Parallel()

	resolver := NewStaticFallbackResolver(
		capabilityResolverStub{err: ErrNoActiveCredential},
		map[string]StaticCredential{
			"rajaongkir": {
				Secret: "shipping-key",
				CapabilitySecrets: map[string]string{
					"shipments:write": "delivery-key",
				},
				CredentialAlias: "platform", DailyLimit: 50_000,
			},
		},
		platformAuthorizerStub{allowed: true},
	)
	ctx := tenancy.WithIdentity(context.Background(), tenancy.Identity{TenantID: "merchant_123"})
	secret, _, _, err := resolver.ResolveProviderCredentialForCapability(
		ctx, "rajaongkir", "shipments:write",
	)
	if err != nil {
		t.Fatal(err)
	}
	if secret != "delivery-key" {
		t.Fatalf("expected delivery credential, got %q", secret)
	}
}

type platformAuthorizerStub struct {
	allowed bool
	err     error
}

func (s platformAuthorizerStub) AllowsPlatformCredential(
	context.Context,
	string,
) (bool, error) {
	return s.allowed, s.err
}

type tenantAwareResolver struct{}

func (tenantAwareResolver) ResolveProviderCredential(
	ctx context.Context,
	_ string,
) (string, string, int64, error) {
	if _, tenantRequest := tenancy.FromContext(ctx); tenantRequest {
		return "", "", 0, ErrNoActiveCredential
	}
	return "platform-database", "platform-db", 50_000, nil
}

type resolverStub struct {
	secret string
	alias  string
	limit  int64
	err    error
}

type capabilityResolverStub struct{ err error }

func (c capabilityResolverStub) ResolveProviderCredential(context.Context, string) (string, string, int64, error) {
	return "", "", 0, c.err
}

func (c capabilityResolverStub) ResolveProviderCredentialForCapability(context.Context, string, string) (string, string, int64, error) {
	return "", "", 0, c.err
}

func (r resolverStub) ResolveProviderCredential(
	context.Context,
	string,
) (string, string, int64, error) {
	return r.secret, r.alias, r.limit, r.err
}
