package providercredentials

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
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

func (r *memoryRepository) Create(_ context.Context, input CreateInput) (Credential, error) {
	r.input = input
	r.item = Credential{
		ID: input.ID, ProviderCode: input.ProviderCode,
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
