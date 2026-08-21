package admin

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDeleteTrackingOperationCascadesAndKeepsAudit(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	unique := time.Now().UTC().Format("20060102150405.000000000")
	var shipmentID string
	err = pool.QueryRow(ctx, `
		INSERT INTO tracking_shipments (
			courier_code, waybill_hash, waybill_masked, waybill_ciphertext
		) VALUES ('integration-delete', $1, '********1234', decode('00', 'hex'))
		RETURNING id::text
	`, "admin-delete-"+unique).Scan(&shipmentID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM tracking_shipments WHERE id = $1::uuid", shipmentID)
		_, _ = pool.Exec(context.Background(), `
			DELETE FROM admin_audit_logs
			WHERE resource_type = 'tracking_shipment' AND resource_id = $1
		`, shipmentID)
	})

	var subscriptionID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO tracking_subscriptions (
			tenant_id, order_reference, fulfillment_reference, shipment_id
		) VALUES ($1, 'order-delete', $2, $3::uuid)
		RETURNING id::text
	`, "tenant-delete-"+unique, "fulfillment-delete-"+unique, shipmentID).Scan(&subscriptionID); err != nil {
		t.Fatal(err)
	}
	statements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tracking_refresh_jobs (shipment_id) VALUES ($1::uuid)`, []any{shipmentID}},
		{`INSERT INTO tracking_status_history (
			shipment_id, normalized_status, provider_fetched_at
		) VALUES ($1::uuid, 'unknown', now())`, []any{shipmentID}},
		{`INSERT INTO tracking_webhook_outbox (
			subscription_id, shipment_id, tenant_id, event_type, deduplication_key
		) VALUES ($2::uuid, $1::uuid, 'tenant-delete', 'tracking.validated', $3)`, []any{shipmentID, subscriptionID, "delete-test-" + unique}},
		{`INSERT INTO tracking_subscription_revisions (
			subscription_id, tenant_id, order_reference, fulfillment_reference,
			shipment_id, revision
		) VALUES ($2::uuid, 'tenant-delete', 'order-delete', 'fulfillment-delete',
			$1::uuid, 1)`, []any{shipmentID, subscriptionID}},
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatal(err)
		}
	}

	repository := NewPostgresRepository(pool)
	page, err := repository.ListTrackingOperations(ctx, TrackingOperationFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	foundCiphertext := false
	for _, item := range page.Items {
		if item.ID == shipmentID {
			foundCiphertext = len(item.WaybillCiphertext) > 0
			break
		}
	}
	if !foundCiphertext {
		t.Fatal("expected encrypted waybill in internal admin repository result")
	}
	if err := repository.DeleteTrackingOperation(ctx, shipmentID, "integration-admin", "req-delete"); err != nil {
		t.Fatal(err)
	}

	var shipmentCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM tracking_shipments WHERE id = $1::uuid
	`, shipmentID).Scan(&shipmentCount); err != nil {
		t.Fatal(err)
	}
	if shipmentCount != 0 {
		t.Fatalf("expected tracking shipment to be deleted, got %d", shipmentCount)
	}

	tables := []string{
		"tracking_refresh_jobs", "tracking_status_history",
		"tracking_subscriptions", "tracking_subscription_revisions", "tracking_webhook_outbox",
	}
	for _, table := range tables {
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE shipment_id = $1::uuid", shipmentID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("expected %s rows to be deleted, got %d", table, count)
		}
	}

	var auditCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM admin_audit_logs
		WHERE action = 'delete_permanently'
		  AND resource_type = 'tracking_shipment'
		  AND resource_id = $1
	`, shipmentID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("expected one retained audit row, got %d", auditCount)
	}

	if err := repository.DeleteTrackingOperation(ctx, shipmentID, "integration-admin", "req-delete-again"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound on second deletion, got %v", err)
	}
}
