package admin

import (
	"context"
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
		DistributionType: "public",
		DisplayOrder:     900,
	}, "integration-test", "req-provider-create")
	if err != nil {
		t.Fatal(err)
	}
	if created.Available || created.BuiltIn || !created.RequiresCredential {
		t.Fatalf("unexpected provider defaults: %#v", created)
	}

	updated, err := repository.UpdateShippingProvider(ctx, code, ShippingProviderUpdateInput{
		Name:        "Provider Test Updated",
		Logo:        "https://api-kurir.emisell.com/provider-logos/default.svg",
		Description: "Provider sementara yang telah diperbarui melalui admin API.",
		Available:   true, DisplayOrder: 901,
	}, "integration-test", "req-provider-update")
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Available || updated.Name != "Provider Test Updated" || updated.DisplayOrder != 901 {
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
}
