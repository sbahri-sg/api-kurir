package providercredentials

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/emisell/api-kurir/internal/tenancy"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTenantCredentialOwnershipIntegration(t *testing.T) {
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
	tenantA := fmt.Sprintf("merchant_a_%d", suffix)
	tenantB := fmt.Sprintf("merchant_b_%d", suffix)
	defer func() {
		_, _ = pool.Exec(context.Background(), `
			DELETE FROM provider_quota_ledger WHERE tenant_id = ANY($1::text[])
		`, []string{tenantA, tenantB})
		_, _ = pool.Exec(context.Background(), `
			DELETE FROM admin_audit_logs
			WHERE resource_type = 'provider_credential'
			  AND details_json->>'tenant_id' = ANY($1::text[])
		`, []string{tenantA, tenantB})
		_, _ = pool.Exec(context.Background(), `
			DELETE FROM provider_credentials WHERE tenant_id = ANY($1::text[])
		`, []string{tenantA, tenantB})
	}()

	cipher, err := NewCipher(testEncryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(NewPostgresRepository(pool), cipher, &acceptingValidator{})
	credentialA, err := service.AddForTenant(
		ctx,
		tenantA,
		"rajaongkir",
		fmt.Sprintf("tenant-a-secret-%d", suffix),
		50_000,
		"integration-test",
		"req_a",
	)
	if err != nil {
		t.Fatal(err)
	}
	credentialB, err := service.AddForTenant(
		ctx,
		tenantB,
		"rajaongkir",
		fmt.Sprintf("tenant-b-secret-%d", suffix),
		100_000,
		"integration-test",
		"req_b",
	)
	if err != nil {
		t.Fatal(err)
	}

	items, err := service.ListForTenant(ctx, tenantA)
	if err != nil || len(items) != 1 || items[0].ID != credentialA.ID {
		t.Fatalf("tenant A list leaked or missed credential: items=%#v err=%v", items, err)
	}

	wrongCredentialCtx := tenancy.WithIdentity(ctx, tenancy.Identity{
		TenantID: tenantA, IntegrationID: credentialB.ID,
	})
	if _, _, _, err := service.ResolveProviderCredential(wrongCredentialCtx, "rajaongkir"); !errors.Is(err, ErrNoActiveCredential) {
		t.Fatalf("tenant A resolved tenant B credential: %v", err)
	}

	ownedCredentialCtx := tenancy.WithIdentity(ctx, tenancy.Identity{
		TenantID: tenantA, IntegrationID: credentialA.ID,
	})
	secret, _, limit, err := service.ResolveProviderCredential(
		ownedCredentialCtx,
		"rajaongkir",
	)
	if err != nil || secret != fmt.Sprintf("tenant-a-secret-%d", suffix) || limit != 50_000 {
		t.Fatalf("tenant A credential resolution failed: secret=%q limit=%d err=%v", secret, limit, err)
	}

	if err := service.DisableForTenant(
		ctx,
		tenantA,
		credentialB.ID,
		"integration-test",
		"req_wrong_owner",
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("tenant A disabled tenant B credential: %v", err)
	}
}
