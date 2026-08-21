package tracking

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/emisell/api-kurir/internal/tenancy"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresTrackingJobLifecycle(t *testing.T) {
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

	repository := NewPostgresRepository(pool)
	hash := "integration-" + time.Now().UTC().Format("20060102150405.000000000")
	shipment, err := repository.Register(
		ctx,
		"integration-test",
		hash,
		"******1234",
		[]byte("encrypted-test-value"),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(
			context.Background(),
			"DELETE FROM tracking_shipments WHERE id = $1::uuid",
			shipment.ID,
		)
	})

	job, err := repository.Claim(ctx, "integration-worker", []string{"integration-test"})
	if err != nil {
		t.Fatal(err)
	}
	if job.ShipmentID != shipment.ID || job.CourierCode != "integration-test" {
		t.Fatalf("unexpected claimed job: %#v", job)
	}
	if err := repository.Complete(ctx, job, Result{
		NormalizedStatus: "delivered",
		StatusLabel:      "Terkirim",
		Summary:          map[string]any{"destination": "TEST"},
		Events:           []Event{},
		ProviderCode:     "integration",
		FetchedAt:        time.Now().UTC(),
		IsFinal:          true,
	}); err != nil {
		t.Fatal(err)
	}

	var status string
	var final bool
	if err := pool.QueryRow(ctx, `
		SELECT normalized_status, is_final
		FROM tracking_shipments
		WHERE id = $1::uuid
	`, shipment.ID).Scan(&status, &final); err != nil {
		t.Fatal(err)
	}
	if status != "delivered" || !final {
		t.Fatalf("unexpected completed shipment: status=%s final=%t", status, final)
	}
}

func TestPostgresTrackingSubscriptionCreatesWebhookOutbox(t *testing.T) {
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

	repository := NewPostgresRepository(pool)
	tenantID := "merchant-integration-" + time.Now().UTC().Format("150405000000")
	tenantCtx := tenancy.WithIdentity(ctx, tenancy.Identity{
		TenantID: tenantID,
	})
	hash := "subscription-" + time.Now().UTC().Format("20060102150405.000000000")
	shipment, err := repository.Register(
		tenantCtx,
		"integration-webhook",
		hash,
		"******5678",
		[]byte("encrypted-test-value"),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM tracking_shipments WHERE id = $1::uuid", shipment.ID)
	})

	subscription, err := repository.UpsertSubscription(
		tenantCtx, shipment.ID, "order-integration", "fulfillment-integration", 0,
	)
	if err != nil {
		t.Fatal(err)
	}
	if subscription.Shipment.ID != shipment.ID {
		t.Fatalf("unexpected subscription: %#v", subscription)
	}
	job, err := repository.Claim(ctx, "integration-worker", []string{"integration-webhook"})
	if err != nil {
		t.Fatal(err)
	}
	fetchedAt := time.Now().UTC()
	result := ApplyEconomyCheckpoint(Result{
		NormalizedStatus: "in_transit",
		StatusLabel:      "Dalam perjalanan",
		Summary:          map[string]any{"status": "IN TRANSIT"},
		Events:           []Event{},
		ProviderCode:     "integration",
		FetchedAt:        fetchedAt,
	}, 1, DefaultProviderHitLimit)
	if err := repository.Complete(ctx, job, result); err != nil {
		t.Fatal(err)
	}
	webhook, err := repository.ClaimWebhook(ctx, "webhook-integration")
	if err != nil {
		t.Fatal(err)
	}
	if webhook.EventType != "tracking.status_changed" ||
		webhook.Data["merchant_id"] != tenantID ||
		webhook.Data["fulfillment_id"] != "fulfillment-integration" {
		t.Fatalf("unexpected webhook: %#v", webhook)
	}
	if err := repository.CompleteWebhook(ctx, webhook.ID, 202); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresTrackingSubscriptionReplacementRequiresRevision(t *testing.T) {
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
	defer pool.Close()

	repository := NewPostgresRepository(pool)
	suffix := time.Now().UTC().Format("150405000000")
	tenantCtx := tenancy.WithIdentity(ctx, tenancy.Identity{
		TenantID: "merchant-revision-" + suffix,
	})
	first, err := repository.RegisterImmediate(
		tenantCtx, "jne", "revision-first-"+suffix,
		"********1111", []byte("encrypted-first"), nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := repository.RegisterImmediate(
		tenantCtx, "jnt", "revision-second-"+suffix,
		"********2222", []byte("encrypted-second"), nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM tracking_shipments WHERE id = $1::uuid", second.ID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM tracking_shipments WHERE id = $1::uuid", first.ID)
	}()

	created, err := repository.UpsertSubscription(
		tenantCtx, first.ID, "order-revision", "fulfillment-revision", 0,
	)
	if err != nil || created.Revision != 1 {
		t.Fatalf("create revision: subscription=%#v err=%v", created, err)
	}
	if _, err := repository.UpsertSubscription(
		tenantCtx, second.ID, "order-revision", "fulfillment-revision", 0,
	); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("expected revision conflict, got %v", err)
	}
	replaced, err := repository.UpsertSubscription(
		tenantCtx, second.ID, "order-revision", "fulfillment-revision", 1,
	)
	if err != nil || replaced.Revision != 2 || replaced.Shipment.ID != second.ID {
		t.Fatalf("replace revision: subscription=%#v err=%v", replaced, err)
	}
	var historyCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM tracking_subscription_revisions
		WHERE subscription_id = $1::uuid
	`, replaced.ID).Scan(&historyCount); err != nil {
		t.Fatal(err)
	}
	if historyCount != 1 {
		t.Fatalf("revision history count: got %d want 1", historyCount)
	}
}
