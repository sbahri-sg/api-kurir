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
			DELETE FROM tenant_active_shipping_providers
			WHERE tenant_id = ANY($1::text[])
		`, []string{tenantA, tenantB})
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
	if _, err := pool.Exec(ctx, `
		INSERT INTO tenant_active_shipping_providers (
			tenant_id,
			provider_code,
			credential_id,
			updated_by
		)
		VALUES ($1, 'rajaongkir', $2::uuid, 'integration-test')
	`, tenantA, credentialA.ID); err != nil {
		t.Fatal(err)
	}

	items, err := service.ListForTenant(ctx, tenantA)
	if err != nil || len(items) != 1 || items[0].ID != credentialA.ID {
		t.Fatalf("tenant A list leaked or missed credential: items=%#v err=%v", items, err)
	}

	wrongCredentialCtx := tenancy.WithIdentity(ctx, tenancy.Identity{
		TenantID: tenantA, ProviderCredentialID: credentialB.ID,
	})
	if _, _, _, err := service.ResolveProviderCredential(wrongCredentialCtx, "rajaongkir"); !errors.Is(err, ErrNoActiveCredential) {
		t.Fatalf("tenant A resolved tenant B credential: %v", err)
	}
	inactiveCredentialCtx := tenancy.WithIdentity(ctx, tenancy.Identity{
		TenantID: tenantB, ProviderCredentialID: credentialB.ID,
	})
	if _, _, _, err := service.ResolveProviderCredential(inactiveCredentialCtx, "rajaongkir"); !errors.Is(err, ErrNoActiveCredential) {
		t.Fatalf("tenant B resolved an installed but inactive credential: %v", err)
	}

	ownedCredentialCtx := tenancy.WithIdentity(ctx, tenancy.Identity{
		TenantID: tenantA,
	})
	secret, _, limit, err := service.ResolveProviderCredential(
		ownedCredentialCtx,
		"rajaongkir",
	)
	if err != nil || secret != fmt.Sprintf("tenant-a-secret-%d", suffix) || limit != 50_000 {
		t.Fatalf("tenant A credential resolution failed: secret=%q limit=%d err=%v", secret, limit, err)
	}

	replacementSecret := fmt.Sprintf("tenant-a-replacement-secret-%d", suffix)
	replacement, err := service.AddForTenant(
		ctx,
		tenantA,
		"rajaongkir",
		replacementSecret,
		75_000,
		"integration-test",
		"req_replace",
	)
	if err != nil {
		t.Fatal(err)
	}
	var activeCount int
	var selectedProvider, selectedCredentialID string
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM provider_credentials
		WHERE tenant_id = $1 AND provider_code = 'rajaongkir' AND active
	`, tenantA).Scan(&activeCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT provider_code, credential_id::text
		FROM tenant_active_shipping_providers
		WHERE tenant_id = $1
	`, tenantA).Scan(&selectedProvider, &selectedCredentialID); err != nil {
		t.Fatal(err)
	}
	if activeCount != 1 || selectedProvider != "rajaongkir" ||
		selectedCredentialID != replacement.ID {
		t.Fatalf(
			"replacement was not atomic: active=%d provider=%q credential=%q",
			activeCount,
			selectedProvider,
			selectedCredentialID,
		)
	}
	secret, _, limit, err = service.ResolveProviderCredential(
		ownedCredentialCtx,
		"rajaongkir",
	)
	if err != nil || secret != replacementSecret || limit != 75_000 {
		t.Fatalf(
			"replacement credential resolution failed: secret=%q limit=%d err=%v",
			secret,
			limit,
			err,
		)
	}
	var selectionVersion int64
	if err := pool.QueryRow(ctx, `
		SELECT version
		FROM tenant_active_shipping_providers
		WHERE tenant_id = $1
	`, tenantA).Scan(&selectionVersion); err != nil {
		t.Fatal(err)
	}
	refreshed, err := service.AddForTenant(
		ctx,
		tenantA,
		"rajaongkir",
		replacementSecret,
		75_000,
		"integration-test",
		"req_refresh",
	)
	if err != nil || refreshed.ID != replacement.ID || !refreshed.Active {
		t.Fatalf("active credential refresh failed: item=%#v err=%v", refreshed, err)
	}
	var refreshedSelectionVersion int64
	if err := pool.QueryRow(ctx, `
		SELECT credential_id::text, version
		FROM tenant_active_shipping_providers
		WHERE tenant_id = $1
	`, tenantA).Scan(&selectedCredentialID, &refreshedSelectionVersion); err != nil {
		t.Fatal(err)
	}
	if selectedCredentialID != replacement.ID ||
		refreshedSelectionVersion != selectionVersion {
		t.Fatalf(
			"refresh changed active provider selection: credential=%q version=%d want=%d",
			selectedCredentialID,
			refreshedSelectionVersion,
			selectionVersion,
		)
	}
	if err := service.DisableForTenantProvider(
		ctx,
		tenantA,
		"rajaongkir",
		"integration-test",
		"req_disable",
	); err != nil {
		t.Fatal(err)
	}
	var fallbackProvider *string
	var fallbackCredentialID *string
	if err := pool.QueryRow(ctx, `
		SELECT provider_code, credential_id::text
		FROM tenant_active_shipping_providers
		WHERE tenant_id = $1
	`, tenantA).Scan(&fallbackProvider, &fallbackCredentialID); err != nil {
		t.Fatal(err)
	}
	if fallbackProvider != nil || fallbackCredentialID != nil {
		t.Fatalf(
			"disabled active credential did not deactivate shipping: provider=%v credential=%v",
			fallbackProvider,
			fallbackCredentialID,
		)
	}

	reactivated, err := service.AddForTenant(
		ctx,
		tenantA,
		"rajaongkir",
		replacementSecret,
		80_000,
		"integration-test",
		"req_reactivate",
	)
	if err != nil {
		t.Fatalf("re-add disabled merchant credential: %v", err)
	}
	if reactivated.ID != replacement.ID || !reactivated.Active ||
		reactivated.DisabledAt != nil || reactivated.DailyLimit != 80_000 {
		t.Fatalf(
			"disabled credential was not reactivated in place: %#v",
			reactivated,
		)
	}
	if err := pool.QueryRow(ctx, `
		SELECT provider_code, credential_id::text
		FROM tenant_active_shipping_providers
		WHERE tenant_id = $1
	`, tenantA).Scan(&fallbackProvider, &fallbackCredentialID); err != nil {
		t.Fatal(err)
	}
	if fallbackProvider != nil || fallbackCredentialID != nil {
		t.Fatalf(
			"credential reactivation must not activate shipping automatically: provider=%v credential=%v",
			fallbackProvider,
			fallbackCredentialID,
		)
	}
	shared, err := service.AddForTenant(
		ctx,
		tenantB,
		"rajaongkir",
		replacementSecret,
		50_000,
		"integration-test",
		"req_duplicate_owner",
	)
	if err != nil {
		t.Fatalf("same provider credential must be reusable by another merchant: %v", err)
	}
	if shared.ID == reactivated.ID || shared.TenantID != tenantB || !shared.Active {
		t.Fatalf("shared upstream key was not isolated per merchant: %#v", shared)
	}
	items, err = service.ListForTenant(ctx, tenantA)
	if err != nil || len(items) == 0 {
		t.Fatalf("tenant A credential disappeared after tenant B install: items=%#v err=%v", items, err)
	}
	items, err = service.ListForTenant(ctx, tenantB)
	if err != nil || len(items) == 0 || items[0].ID != shared.ID {
		t.Fatalf("tenant B did not receive its isolated credential: items=%#v err=%v", items, err)
	}

	if err := service.DisableForTenantProvider(
		ctx,
		tenantA,
		"biteship",
		"integration-test",
		"req_wrong_owner",
	); !errors.Is(err, ErrUnsupportedProvider) {
		t.Fatalf("merchant disabled unsupported provider credential: %v", err)
	}
}
