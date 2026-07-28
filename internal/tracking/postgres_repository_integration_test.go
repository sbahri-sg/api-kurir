package tracking

import (
	"context"
	"os"
	"testing"
	"time"

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
