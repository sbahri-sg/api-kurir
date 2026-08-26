package merchantshipping

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSelectedCourierCodesIntegration(t *testing.T) {
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

	tenantID := fmt.Sprintf("rate_selection_%d", time.Now().UTC().UnixNano())
	defer func() {
		_, _ = pool.Exec(
			context.Background(),
			"DELETE FROM tenant_shipping_service_selections WHERE tenant_id = $1",
			tenantID,
		)
		_, _ = pool.Exec(
			context.Background(),
			"DELETE FROM tenant_shipping_preferences WHERE tenant_id = $1",
			tenantID,
		)
		_, _ = pool.Exec(
			context.Background(),
			"DELETE FROM tenant_active_shipping_providers WHERE tenant_id = $1",
			tenantID,
		)
	}()

	if _, err := pool.Exec(ctx, `
		INSERT INTO tenant_active_shipping_providers (
			tenant_id, provider_code, credential_id, updated_by
		)
		VALUES ($1, 'emisell', NULL, 'integration-test')
	`, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO tenant_shipping_preferences (
			tenant_id, provider_code, selection_mode, enabled_groups, updated_by
		)
		VALUES ($1, 'emisell', 'custom', '{}', 'integration-test')
	`, tenantID); err != nil {
		t.Fatal(err)
	}
	tag, err := pool.Exec(ctx, `
		INSERT INTO tenant_shipping_service_selections (
			tenant_id, provider_code, courier_service_id
		)
		SELECT $1, 'emisell', service.id
		FROM courier_services service
		JOIN couriers courier ON courier.id = service.courier_id
		JOIN provider_courier_services provider_service
		  ON provider_service.provider_code = 'emisell'
		 AND provider_service.courier_service_id = service.id
		 AND provider_service.active
		WHERE (courier.code = 'jne' AND service.code IN ('REG', 'JTR'))
		   OR (courier.code = 'jnt' AND service.code = 'EZ')
	`, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if tag.RowsAffected() < 3 {
		t.Fatalf("shipping service seed is incomplete: inserted=%d want at least 3", tag.RowsAffected())
	}

	codes, err := NewPostgresRepository(pool).SelectedCourierCodes(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != 2 || codes[0] != "jne" || codes[1] != "jnt" {
		t.Fatalf("unexpected selected courier codes: %#v", codes)
	}
}
