package apikeys

import (
	"bytes"
	"context"
	"testing"
	"time"
)

type memoryRepository struct {
	input             CreateInput
	item              APIKey
	authenticateCalls int
	revoked           bool
}

func (r *memoryRepository) List(context.Context, int, int) ([]APIKey, error) {
	return []APIKey{r.item}, nil
}

func (r *memoryRepository) Create(_ context.Context, input CreateInput) (APIKey, error) {
	r.input = input
	r.item = APIKey{
		ID:          "key-id",
		KeyPrefix:   input.KeyPrefix,
		KeyLastFour: input.KeyLast4,
		DisplayKey:  input.KeyPrefix + "••••" + input.KeyLast4,
		Scopes:      input.Scopes,
		Active:      true,
		CreatedBy:   input.CreatedBy,
		CreatedAt:   time.Now(),
	}
	return r.item, nil
}

func (r *memoryRepository) Revoke(context.Context, string, string, string) error {
	r.revoked = true
	return nil
}

func (r *memoryRepository) Authenticate(_ context.Context, keyHash []byte) (bool, error) {
	r.authenticateCalls++
	return !r.revoked && bytes.Equal(keyHash, r.input.KeyHash), nil
}

func (r *memoryRepository) AuthenticateScope(
	_ context.Context,
	keyHash []byte,
	scope string,
) (bool, error) {
	r.authenticateCalls++
	if r.revoked || !bytes.Equal(keyHash, r.input.KeyHash) {
		return false, nil
	}
	for _, candidate := range r.input.Scopes {
		if candidate == scope {
			return true, nil
		}
	}
	return false, nil
}

func TestGenerateStoresOnlyHashAndReturnsSecretOnce(t *testing.T) {
	t.Parallel()

	repository := &memoryRepository{}
	service := NewService(repository)
	result, err := service.Generate(
		context.Background(),
		"operator",
		"req_test",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Secret) < 40 || result.Secret[:len(keyNamespace)] != keyNamespace {
		t.Fatalf("unexpected generated secret format: %q", result.Secret)
	}
	if bytes.Contains(repository.input.KeyHash, []byte(result.Secret)) {
		t.Fatal("stored hash must not contain the plaintext secret")
	}
	if len(repository.input.KeyHash) != 32 {
		t.Fatalf("expected SHA-256 hash, got %d bytes", len(repository.input.KeyHash))
	}
	if result.APIKey.DisplayKey == result.Secret {
		t.Fatal("list model must not expose the plaintext secret")
	}
}

func TestAuthenticateCachesValidKeyAndRevokeClearsCache(t *testing.T) {
	t.Parallel()

	repository := &memoryRepository{}
	service := NewService(repository)
	result, err := service.Generate(
		context.Background(),
		"operator",
		"req_test",
	)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		valid, err := service.Authenticate(context.Background(), result.Secret)
		if err != nil || !valid {
			t.Fatalf("expected valid generated key: valid=%v err=%v", valid, err)
		}
	}
	if repository.authenticateCalls != 1 {
		t.Fatalf("expected one repository authentication, got %d", repository.authenticateCalls)
	}
	if err := service.Revoke(context.Background(), result.APIKey.ID, "operator", "req_revoke"); err != nil {
		t.Fatal(err)
	}
	valid, err := service.Authenticate(context.Background(), result.Secret)
	if err != nil {
		t.Fatal(err)
	}
	if valid {
		t.Fatal("revoked key must no longer authenticate")
	}
}

func TestMainServiceKeyRequiresGatewayScope(t *testing.T) {
	t.Parallel()

	repository := &memoryRepository{}
	service := NewService(repository)
	publicKey, err := service.Generate(context.Background(), "operator", "req_public")
	if err != nil {
		t.Fatal(err)
	}
	allowed, err := service.AuthenticateScope(
		context.Background(), publicKey.Secret, ScopeGatewayAccess,
	)
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Fatal("public key must not receive gateway access")
	}

	mainKey, err := service.GenerateMainService(
		context.Background(), "operator", "req_main_service",
	)
	if err != nil {
		t.Fatal(err)
	}
	allowed, err = service.AuthenticateScope(
		context.Background(), mainKey.Secret, ScopeGatewayAccess,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !allowed {
		t.Fatal("main service key must receive gateway access")
	}
	if mainKey.APIKey.Kind != KeyKindMainService {
		t.Fatalf("kind=%q want %q", mainKey.APIKey.Kind, KeyKindMainService)
	}
}
