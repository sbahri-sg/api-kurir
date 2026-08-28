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
	providerCode := fmt.Sprintf("provider_test_%d", suffix)
	if len(providerCode) > 48 {
		providerCode = providerCode[:48]
	}
	credentialID := fmt.Sprintf("00000000-0000-4000-8000-%012x", uint64(suffix)&0xffffffffffff)
	fingerprint := sha256.Sum256([]byte(credentialID))
	alias := fmt.Sprintf("provider-test-%d", suffix)
	defer func() {
		_, _ = pool.Exec(context.Background(), `
			DELETE FROM tenant_active_shipping_providers
			WHERE tenant_id = ANY($1::text[])
		`, []string{tenantID, otherTenantID})
		_, _ = pool.Exec(context.Background(), `
			DELETE FROM provider_credentials WHERE id = $1::uuid
		`, credentialID)
		_, _ = pool.Exec(context.Background(), `
			DELETE FROM shipping_integration_providers WHERE code = $1
		`, providerCode)
	}()

	_, err = pool.Exec(ctx, `
		INSERT INTO shipping_integration_providers (
			code, name, logo_url, description, built_in,
			integration_type, distribution_type,
			requires_credential, credential_type, available, display_order
		)
		VALUES ($1, 'Provider Test', 'https://example.com/provider.svg',
			'Provider managed untuk pengujian aktivasi merchant.', false,
			'managed_upstream', 'merchant', true, 'api_key', true, 9999)
	`, providerCode)
	if err != nil {
		t.Fatal(err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO provider_credentials (
			id,
			tenant_id,
			provider_code,
			environment_code,
			credential_alias,
			secret_ciphertext,
			secret_fingerprint,
			key_prefix,
			key_last_four,
			daily_limit,
			created_by
		)
		VALUES ($1::uuid, $2, $3, 'sandbox', $4, $5, $6, 'test', '1234', 50000, 'integration-test')
	`, credentialID, tenantID, providerCode, alias, []byte("ciphertext"), fingerprint[:])
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
		if provider.Code == providerCode && !provider.Installed {
			t.Fatalf("valid sandbox credential was not reported as installed: %#v", provider)
		}
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
	active, err := service.Activate(ctx, tenantID, providerCode, ChangeInput{
		ExpectedVersion: &version,
		UpdatedBy:       "integration-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if active.ActiveProviderCode == nil || *active.ActiveProviderCode != providerCode || active.Version != 3 {
		t.Fatalf("unexpected active catalog: %#v", active)
	}

	_, err = service.Activate(ctx, otherTenantID, providerCode, ChangeInput{
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
			integration_type, distribution_type,
			requires_credential, credential_type, available, display_order
		)
		VALUES ($1, 'Hidden Provider', 'https://example.com/provider.svg',
			'Provider integration test yang belum tersedia.', false,
			'managed_upstream', 'merchant', true, 'api_key', false, 9999)
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

func TestPartnerHostedActivationPinsReleaseAndScopesIntegration(t *testing.T) {
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
	providerCode := fmt.Sprintf("partner_%d", suffix)
	if len(providerCode) > 48 {
		providerCode = providerCode[:48]
	}
	tenantID := fmt.Sprintf("partner_merchant_%d", suffix)
	defer func() {
		_, _ = pool.Exec(context.Background(), `
			DELETE FROM tenant_active_shipping_providers WHERE tenant_id = $1
		`, tenantID)
		_, _ = pool.Exec(context.Background(), `
			UPDATE shipping_integration_providers SET active_release_id = NULL WHERE code = $1
		`, providerCode)
		_, _ = pool.Exec(context.Background(), `
			DELETE FROM partner_integration_submissions WHERE provider_code = $1
		`, providerCode)
		_, _ = pool.Exec(context.Background(), `
			DELETE FROM shipping_integration_providers WHERE code = $1
		`, providerCode)
	}()

	_, err = pool.Exec(ctx, `
		INSERT INTO shipping_integration_providers (
			code, name, logo_url, description, built_in,
			integration_type, distribution_type, requires_credential,
			credential_type, available, display_order
		)
		VALUES ($1, 'Hosted Partner', 'https://example.com/provider.svg',
			'Provider partner-hosted untuk pengujian instalasi release.', false,
			'partner_hosted', 'merchant', false, 'none', true, 9997)
	`, providerCode)
	if err != nil {
		t.Fatal(err)
	}
	var releaseID string
	err = pool.QueryRow(ctx, `
		INSERT INTO partner_integration_submissions (
			provider_code, version, status, file_name, content_type,
			artifact_size, artifact_sha256, artifact, scan_report,
			required_scopes, submitted_by, reviewed_by
		)
		VALUES ($1, '2.1.0', 'published', 'release.zip', 'application/zip',
			1, repeat('b', 64), decode('00', 'hex'),
			'{"passed":true}'::jsonb,
			ARRAY['rates:read', 'shipments:write', 'tracking:read']::text[],
			'integration-test', 'integration-test')
		RETURNING id::text
	`, providerCode).Scan(&releaseID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `
		UPDATE shipping_integration_providers
		SET active_release_id = $2::uuid
		WHERE code = $1
	`, providerCode, releaseID)
	if err != nil {
		t.Fatal(err)
	}

	service := NewService(NewPostgresRepository(pool))
	catalog, err := service.Catalog(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	provider := findProvider(t, catalog, providerCode)
	if provider.ActiveReleaseVersion != "2.1.0" || len(provider.RequiredScopes) != 3 {
		t.Fatalf("partner release metadata missing from catalog: %#v", provider)
	}

	active, err := service.Activate(ctx, tenantID, providerCode, ChangeInput{
		ExpectedVersion: &catalog.Version,
		UpdatedBy:       "integration-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	provider = findProvider(t, active, providerCode)
	if !provider.Active || len(provider.GrantedScopes) != 3 {
		t.Fatalf("partner release was not pinned on activation: %#v", provider)
	}
	var pinnedReleaseID string
	var pinnedScopes []string
	err = pool.QueryRow(ctx, `
		SELECT release_id::text, granted_scopes
		FROM tenant_active_shipping_providers
		WHERE tenant_id = $1
	`, tenantID).Scan(&pinnedReleaseID, &pinnedScopes)
	if err != nil {
		t.Fatal(err)
	}
	if pinnedReleaseID != releaseID || len(pinnedScopes) != 3 {
		t.Fatalf("pinned release/scopes=%q %#v want=%q", pinnedReleaseID, pinnedScopes, releaseID)
	}
}

func findProvider(t *testing.T, catalog Catalog, code string) Provider {
	t.Helper()
	for _, provider := range catalog.Providers {
		if provider.Code == code {
			return provider
		}
	}
	t.Fatalf("provider %q is missing from catalog", code)
	return Provider{}
}
