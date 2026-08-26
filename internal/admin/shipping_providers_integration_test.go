package admin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestShippingProviderManagementIntegration(t *testing.T) {
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

	code := fmt.Sprintf("provider-test-%d", time.Now().UTC().UnixNano())
	if len(code) > 48 {
		code = code[:48]
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `
			DELETE FROM admin_audit_logs
			WHERE resource_type = 'shipping_provider' AND resource_id = $1
		`, code)
		_, _ = pool.Exec(context.Background(), `
			DELETE FROM shipping_integration_providers WHERE code = $1
		`, code)
	}()

	repository := NewPostgresRepository(pool)
	created, err := repository.CreateShippingProvider(ctx, ShippingProviderCreateInput{
		Code: code, Name: "Provider Test",
		Logo:             "https://api-kurir.emisell.com/provider-logos/default.svg",
		Description:      "Provider sementara untuk pengujian integrasi API Kurir.",
		IntegrationType:  "managed_upstream",
		DistributionType: "merchant",
		CredentialType:   "api_key_secret",
		DisplayOrder:     900,
	}, "integration-test", "req-provider-create")
	if err != nil {
		t.Fatal(err)
	}
	if created.Available || created.BuiltIn || !created.RequiresCredential ||
		created.CredentialType != "api_key_secret" || len(created.CredentialFields) != 2 {
		t.Fatalf("unexpected provider defaults: %#v", created)
	}

	updated, err := repository.UpdateShippingProvider(ctx, code, ShippingProviderUpdateInput{
		Name:           "Provider Test Updated",
		Logo:           "https://api-kurir.emisell.com/provider-logos/default.svg",
		Description:    "Provider sementara yang telah diperbarui melalui admin API.",
		CredentialType: "oauth2_client_credentials",
		Available:      true, DisplayOrder: 901,
	}, "integration-test", "req-provider-update")
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Available || updated.Name != "Provider Test Updated" || updated.DisplayOrder != 901 ||
		updated.CredentialType != "oauth2_client_credentials" || len(updated.CredentialFields) != 2 {
		t.Fatalf("unexpected updated provider: %#v", updated)
	}

	items, err := repository.ListShippingProviders(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range items {
		if item.Code == code {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("created provider %q is missing from admin catalog", code)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO partner_integration_submissions (
			provider_code, version, status, file_name, artifact_size,
			artifact_sha256, artifact, submitted_by
		) VALUES (
			$1, '0.0.1', 'technical_review', 'provider-test.zip', 1,
			repeat('a', 64), decode('00', 'hex'), 'integration-test'
		)
	`, code); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO partner_access_keys (
			provider_code, display_key, secret_hash, created_by
		) VALUES (
			$1, 'epk_live_********' || right(md5($1), 4),
			encode(digest($1, 'sha256'), 'hex'), 'integration-test'
		)
	`, code); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO partner_explorer_credentials (
			provider_code, display_key, auth_header, secret_ciphertext,
			created_by, updated_by
		) VALUES (
			$1, 'test-key', 'key', decode(repeat('78', 36), 'hex'),
			'integration-test', 'integration-test'
		)
	`, code); err != nil {
		t.Fatal(err)
	}

	if err := repository.DeleteShippingProvider(
		ctx, "emisell", "integration-test", "req-provider-delete-built-in",
	); !errors.Is(err, ErrConflict) {
		t.Fatalf("delete built-in error=%v want ErrConflict", err)
	}
	if err := repository.DeleteShippingProvider(
		ctx, code, "integration-test", "req-provider-delete",
	); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.getShippingProvider(ctx, code); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted provider error=%v want ErrNotFound", err)
	}
	var remainingArtifacts int64
	if err := pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM partner_integration_submissions WHERE provider_code = $1)
			+ (SELECT count(*) FROM partner_access_keys WHERE provider_code = $1)
			+ (SELECT count(*) FROM partner_explorer_credentials WHERE provider_code = $1)
	`, code).Scan(&remainingArtifacts); err != nil {
		t.Fatal(err)
	}
	if remainingArtifacts != 0 {
		t.Fatalf("onboarding artifacts were not purged: %d", remainingArtifacts)
	}
}
