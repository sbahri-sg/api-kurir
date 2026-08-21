package merchantproviders

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestProviderActivationAndCredentialFallbackIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	suffix := time.Now().UTC().UnixNano()
	tenantID := fmt.Sprintf("provider_merchant_%d", suffix)
	otherTenantID := fmt.Sprintf("provider_other_%d", suffix)
	credentialID := fmt.Sprintf("00000000-0000-4000-8000-%012x", uint64(suffix)&0xffffffffffff)
	fingerprint := sha256.Sum256([]byte(credentialID))
	alias := fmt.Sprintf("rajaongkir-provider-%d", suffix)
	defer func() {
		_, _ = pool.Exec(context.Background(), `
			DELETE FROM tenant_active_shipping_providers
			WHERE tenant_id = ANY($1::text[])
		`, []string{tenantID, otherTenantID})
		_, _ = pool.Exec(context.Background(), `
			DELETE FROM provider_credentials WHERE id = $1::uuid
		`, credentialID)
	}()

	_, err = pool.Exec(ctx, `
		INSERT INTO provider_credentials (
			id,
			tenant_id,
			provider_code,
			credential_alias,
			secret_ciphertext,
			secret_fingerprint,
			key_prefix,
			key_last_four,
			daily_limit,
			created_by
		)
		VALUES ($1::uuid, $2, 'rajaongkir', $3, $4, $5, 'test', '1234', 50000, 'integration-test')
	`, credentialID, tenantID, alias, []byte("ciphertext"), fingerprint[:])
	if err != nil {
		t.Fatal(err)
	}

	service := NewService(NewPostgresRepository(pool))
	initial, err := service.Catalog(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if initial.ActiveProviderCode != DefaultProviderCode || initial.Version != 0 {
		t.Fatalf("unexpected initial catalog: %#v", initial)
	}

	version := initial.Version
	active, err := service.Activate(ctx, tenantID, "rajaongkir", ChangeInput{
		CredentialID:    credentialID,
		ExpectedVersion: &version,
		UpdatedBy:       "integration-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if active.ActiveProviderCode != "rajaongkir" || active.Version != 1 ||
		active.ActiveCredentialID == nil || *active.ActiveCredentialID != credentialID {
		t.Fatalf("unexpected active catalog: %#v", active)
	}

	_, err = service.Activate(ctx, otherTenantID, "rajaongkir", ChangeInput{
		CredentialID: credentialID,
		UpdatedBy:    "integration-test",
	})
	if !errors.Is(err, ErrCredentialUnavailable) {
		t.Fatalf("cross-tenant credential error=%v want ErrCredentialUnavailable", err)
	}

	_, err = pool.Exec(ctx, `
		UPDATE provider_credentials
		SET active = false, disabled_at = now(), disabled_by = 'integration-test'
		WHERE id = $1::uuid
	`, credentialID)
	if err != nil {
		t.Fatal(err)
	}
	fallback, err := service.Catalog(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if fallback.ActiveProviderCode != DefaultProviderCode || fallback.Version != 2 ||
		fallback.ActiveCredentialID != nil {
		t.Fatalf("credential disable did not fallback safely: %#v", fallback)
	}
}
