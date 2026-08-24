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
	if initial.ActiveProviderCode != nil || initial.Version != 0 {
		t.Fatalf("unexpected initial catalog: %#v", initial)
	}
	for _, provider := range initial.Providers {
		if provider.Active {
			t.Fatalf("new merchant has an active provider: %#v", provider)
		}
		if provider.Logo == "" || provider.Description == "" {
			t.Fatalf("provider presentation metadata is incomplete: %#v", provider)
		}
	}

	version := initial.Version
	emisell, err := service.Activate(ctx, tenantID, EmisellProviderCode, ChangeInput{
		ExpectedVersion: &version,
		UpdatedBy:       "integration-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if emisell.ActiveProviderCode == nil || *emisell.ActiveProviderCode != EmisellProviderCode || emisell.Version != 1 {
		t.Fatalf("unexpected Emisell catalog: %#v", emisell)
	}
	version = emisell.Version
	inactive, err := service.Deactivate(ctx, tenantID, EmisellProviderCode, ChangeInput{
		ExpectedVersion: &version,
		UpdatedBy:       "integration-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if inactive.ActiveProviderCode != nil || inactive.Version != 2 {
		t.Fatalf("Emisell provider was not deactivated: %#v", inactive)
	}

	version = inactive.Version
	active, err := service.Activate(ctx, tenantID, "rajaongkir", ChangeInput{
		ExpectedVersion: &version,
		UpdatedBy:       "integration-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if active.ActiveProviderCode == nil || *active.ActiveProviderCode != "rajaongkir" || active.Version != 3 {
		t.Fatalf("unexpected active catalog: %#v", active)
	}

	_, err = service.Activate(ctx, otherTenantID, "rajaongkir", ChangeInput{
		UpdatedBy: "integration-test",
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
	if fallback.ActiveProviderCode != nil || fallback.Version != 4 {
		t.Fatalf("credential disable did not deactivate shipping safely: %#v", fallback)
	}
}

func TestCatalogExcludesUnavailableProvidersIntegration(t *testing.T) {
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
	providerCode := fmt.Sprintf("hidden_%d", suffix)
	tenantID := fmt.Sprintf("catalog_merchant_%d", suffix)
	defer func() {
		_, _ = pool.Exec(
			context.Background(),
			"DELETE FROM shipping_integration_providers WHERE code = $1",
			providerCode,
		)
	}()

	_, err = pool.Exec(ctx, `
		INSERT INTO shipping_integration_providers (
			code, name, logo_url, description, built_in,
			requires_credential, available, display_order
		)
		VALUES ($1, 'Hidden Provider', 'https://example.com/provider.svg',
			'Provider integration test yang belum tersedia.', false, true, false, 9999)
	`, providerCode)
	if err != nil {
		t.Fatal(err)
	}

	repository := NewPostgresRepository(pool)
	hiddenCatalog, err := repository.Catalog(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	for _, provider := range hiddenCatalog.Providers {
		if provider.Code == providerCode {
			t.Fatalf("unavailable provider leaked into merchant catalog: %#v", provider)
		}
	}

	if _, err := pool.Exec(ctx, `
		UPDATE shipping_integration_providers
		SET available = true
		WHERE code = $1
	`, providerCode); err != nil {
		t.Fatal(err)
	}
	visibleCatalog, err := repository.Catalog(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	for _, provider := range visibleCatalog.Providers {
		if provider.Code == providerCode {
			if !provider.Available {
				t.Fatalf("visible provider is not marked available: %#v", provider)
			}
			return
		}
	}
	t.Fatalf("available provider was not returned in merchant catalog")
}
