package providercredentials

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/emisell/api-kurir/internal/tenancy"
)

const testEncryptionKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

type memoryRepository struct {
	input    CreateInput
	item     Credential
	disabled bool
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
		CredentialAlias: input.CredentialAlias,
		DisplayKey:      input.KeyPrefix + "••••" + input.KeyLastFour,
		DailyLimit:      input.DailyLimit, Active: true,
		ValidationStatus: "valid", LastValidatedAt: time.Now(),
		CreatedAt: time.Now(),
	}
	return r.item, nil
}

func (r *memoryRepository) Disable(context.Context, string, string, string) error {
	r.disabled = true
	r.item.Active = false
	return nil
}

func (r *memoryRepository) DisableForTenant(
	_ context.Context,
	tenantID, _ string,
	_ string,
	_ string,
) error {
	if r.item.TenantID != tenantID {
		return ErrNotFound
	}
	r.disabled = true
	r.item.Active = false
	return nil
}

func (r *memoryRepository) ResolveActive(context.Context, string) (StoredCredential, error) {
	if r.item.ID == "" || r.disabled {
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
	validator := &acceptingValidator{}
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

func TestTenantProviderResolutionRequiresExplicitIntegrationID(t *testing.T) {
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
	if _, _, _, err := service.ResolveProviderCredential(ctx, "rajaongkir"); !errors.Is(err, ErrNoActiveCredential) {
		t.Fatalf("tenant without integration ID must stay on free mode: %v", err)
	}
	ctx = tenancy.WithIdentity(context.Background(), tenancy.Identity{
		TenantID: "merchant_123", IntegrationID: repository.item.ID,
	})
	secret, _, _, err := service.ResolveProviderCredential(ctx, "rajaongkir")
	if err != nil || secret != "tenant-provider-secret" {
		t.Fatalf("explicit tenant integration was not resolved: secret=%q err=%v", secret, err)
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

type resolverStub struct {
	secret string
	alias  string
	limit  int64
	err    error
}

func (r resolverStub) ResolveProviderCredential(
	context.Context,
	string,
) (string, string, int64, error) {
	return r.secret, r.alias, r.limit, r.err
}
