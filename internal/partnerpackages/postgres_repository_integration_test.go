package partnerpackages

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPublishedReleaseAndRollbackIntegration(t *testing.T) {
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
	providerCode := fmt.Sprintf("release_%d", suffix)
	if len(providerCode) > 48 {
		providerCode = providerCode[:48]
	}
	tenantID := fmt.Sprintf("release_merchant_%d", suffix)
	defer func() {
		_, _ = pool.Exec(context.Background(), `
			DELETE FROM tenant_active_shipping_providers WHERE tenant_id = $1
		`, tenantID)
		_, _ = pool.Exec(context.Background(), `
			UPDATE shipping_integration_providers
			SET active_release_id = NULL
			WHERE code = $1
		`, providerCode)
		_, _ = pool.Exec(context.Background(), `
			DELETE FROM admin_audit_logs
			WHERE resource_type IN ('partner_integration_submission', 'partner_access_key')
			  AND resource_id IN (
				SELECT id::text FROM partner_integration_submissions WHERE provider_code = $1
			  )
		`, providerCode)
		_, _ = pool.Exec(context.Background(), `
			DELETE FROM partner_access_keys WHERE provider_code = $1
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
			available, display_order
		)
		VALUES ($1, 'Release Provider', 'https://example.com/provider.svg',
			'Provider untuk menguji release immutable dan rollback.', false,
			'partner_hosted', 'public', false, false, 9998)
	`, providerCode)
	if err != nil {
		t.Fatal(err)
	}

	firstID := insertApprovedSubmission(t, pool, providerCode, "1.0.0")
	secondID := insertApprovedSubmission(t, pool, providerCode, "1.1.0")
	service := NewService(NewPostgresRepository(pool))

	first, err := service.UpdateStatus(ctx, firstID, StatusUpdate{
		Status: "published", ReviewNote: "first production release",
		ReviewedBy: "integration-test", RequestID: "req-release-first",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !first.IsActiveRelease || first.Status != "published" {
		t.Fatalf("first version did not become active: %#v", first)
	}

	second, err := service.UpdateStatus(ctx, secondID, StatusUpdate{
		Status: "published", ReviewNote: "new production release",
		ReviewedBy: "integration-test", RequestID: "req-release-second",
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err = service.Get(ctx, firstID)
	if err != nil {
		t.Fatal(err)
	}
	if !second.IsActiveRelease || first.Status != "superseded" || first.IsActiveRelease {
		t.Fatalf("new version did not supersede first: first=%#v second=%#v", first, second)
	}

	rolledBack, err := service.UpdateStatus(ctx, firstID, StatusUpdate{
		Status: "published", ReviewNote: "rollback after incident",
		ReviewedBy: "integration-test", RequestID: "req-release-rollback",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err = service.Get(ctx, secondID)
	if err != nil {
		t.Fatal(err)
	}
	if !rolledBack.IsActiveRelease || second.Status != "superseded" || second.IsActiveRelease {
		t.Fatalf("rollback did not restore first version: first=%#v second=%#v", rolledBack, second)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO tenant_active_shipping_providers (
			tenant_id, provider_code, release_id, granted_scopes, updated_by
		)
		VALUES ($1, $2, $3::uuid, ARRAY['rates:read', 'tracking:read']::text[], 'integration-test')
	`, tenantID, providerCode, firstID)
	if err != nil {
		t.Fatal(err)
	}
	suspended, err := service.UpdateStatus(ctx, firstID, StatusUpdate{
		Status: "suspended", ReviewNote: "emergency stop",
		ReviewedBy: "integration-test", RequestID: "req-release-suspend",
	})
	if err != nil {
		t.Fatal(err)
	}
	if suspended.Status != "suspended" || suspended.IsActiveRelease {
		t.Fatalf("release was not suspended: %#v", suspended)
	}
	var selectedProvider *string
	var version int64
	err = pool.QueryRow(ctx, `
		SELECT provider_code, version
		FROM tenant_active_shipping_providers
		WHERE tenant_id = $1
	`, tenantID).Scan(&selectedProvider, &version)
	if err != nil {
		t.Fatal(err)
	}
	if selectedProvider != nil || version != 2 {
		t.Fatalf("suspended release did not deactivate merchant: provider=%v version=%d", selectedProvider, version)
	}
}

func TestManagedProviderCannotReceivePartnerPortalKeyIntegration(t *testing.T) {
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

	service := NewService(NewPostgresRepository(pool))
	_, err = service.GenerateAccessKey(ctx, "rajaongkir", "integration-test", "req-key-type")
	if !errors.Is(err, ErrProviderType) {
		t.Fatalf("managed provider access key error=%v want ErrProviderType", err)
	}
}

func TestPartnerUploadGuardrailsIntegration(t *testing.T) {
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
	providerCode := fmt.Sprintf("guard_%d", suffix)
	if len(providerCode) > 48 {
		providerCode = providerCode[:48]
	}
	var accessKeyID string
	defer func() {
		if accessKeyID != "" {
			_, _ = pool.Exec(context.Background(), `
				DELETE FROM admin_audit_logs
				WHERE resource_type = 'partner_access_key' AND resource_id = $1
			`, accessKeyID)
		}
		_, _ = pool.Exec(context.Background(), `
			DELETE FROM partner_access_keys WHERE provider_code = $1
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
			available, display_order
		)
		VALUES ($1, 'Guardrail Provider', 'https://example.com/provider.svg',
			'Provider untuk menguji upload guardrail.', false,
			'partner_hosted', 'public', false, false, 9997)
	`, providerCode)
	if err != nil {
		t.Fatal(err)
	}

	service := NewService(NewPostgresRepository(pool))
	generated, err := service.GenerateAccessKey(ctx, providerCode, "integration-test", "req-guard-key")
	if err != nil {
		t.Fatal(err)
	}
	accessKeyID = generated.AccessKey.ID
	for attempt := 0; attempt < MaxUploadsPerHour; attempt++ {
		release, reserveErr := service.BeginUpload(ctx, accessKeyID)
		if reserveErr != nil {
			t.Fatalf("attempt %d failed: %v", attempt+1, reserveErr)
		}
		release()
	}
	if _, err := service.BeginUpload(ctx, accessKeyID); !errors.Is(err, ErrUploadRateLimit) {
		t.Fatalf("rate limit error=%v want %v", err, ErrUploadRateLimit)
	}

	for index := 0; index < MaxProviderSubmissions; index++ {
		insertApprovedSubmission(t, pool, providerCode, fmt.Sprintf("quota-%d", index))
	}
	_, err = service.Upload(ctx, UploadInput{
		ProviderCode: providerCode,
		Version:      "quota-overflow",
		FileName:     "quota-overflow.zip",
		Payload: testArchive(t, map[string]string{
			"emisell-extension.yaml": testManifest(providerCode),
			"openapi.yaml":           testOpenAPI(),
		}),
		SubmittedBy: "integration-test",
	})
	if !errors.Is(err, ErrStorageQuota) {
		t.Fatalf("quota error=%v want %v", err, ErrStorageQuota)
	}
}

func insertApprovedSubmission(
	t *testing.T,
	pool *pgxpool.Pool,
	providerCode, version string,
) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO partner_integration_submissions (
			provider_code, version, status, file_name, content_type,
			artifact_size, artifact_sha256, artifact, scan_report,
			required_scopes, submitted_by, reviewed_by
		)
		VALUES (
			$1, $2, 'approved', $2 || '.zip', 'application/zip',
			1, repeat('a', 64), decode('00', 'hex'),
			'{"passed":true,"file_count":1,"expanded_size":1,"checks":[],"warnings":[],"manifest":{"schema_version":"1","provider_code":"test","provider_name":"Test","contract_version":"v1","sandbox_url":"https://sandbox.example.com","production_url":"https://api.example.com","declared_capabilities":["rates","tracking"],"declared_services":["regular"]},"required_openapi_paths":[]}'::jsonb,
			ARRAY['rates:read', 'tracking:read']::text[], 'integration-test', 'integration-test'
		)
		RETURNING id::text
	`, providerCode, version).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
