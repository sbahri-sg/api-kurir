package locations

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestProviderHierarchyAndPostalCodeSearch(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	suffix := time.Now().UTC().Format("150405000000000")
	provider := "integration-postal-" + suffix
	province := "PROVINSI TEST " + suffix
	city := "KOTA TEST " + suffix
	district := "KECAMATAN TEST " + suffix
	subdistrict := "KELURAHAN TEST " + suffix
	postalCode := "54321"
	sharedProviderLocationID := "shared-" + suffix
	repository := NewPostgresRepository(pool)

	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = pool.Exec(
			cleanupCtx,
			"DELETE FROM provider_location_sync_checkpoints WHERE provider_code = $1",
			provider,
		)
		_, _ = pool.Exec(
			cleanupCtx,
			"DELETE FROM location_postal_codes WHERE provider_code = $1",
			provider,
		)
		_, _ = pool.Exec(
			cleanupCtx,
			"DELETE FROM provider_location_mappings WHERE provider_code = $1",
			provider,
		)
		for _, level := range []string{"subdistrict", "district", "city", "province"} {
			_, _ = pool.Exec(
				cleanupCtx,
				"DELETE FROM locations WHERE province = $1 AND level = $2",
				province,
				level,
			)
		}
		_, _ = pool.Exec(cleanupCtx, `
			DELETE FROM postal_codes
			WHERE code = $1
			  AND NOT EXISTS (
				SELECT 1
				FROM location_postal_codes
				WHERE postal_code_id = postal_codes.id
			  )
		`, postalCode)
	})

	levels := [][]ImportedProviderLocation{
		{{
			ProviderLocationID: sharedProviderLocationID,
			ProviderLabel:      province,
			Province:           province,
			SourceEndpoint:     "destination/province",
		}},
		{{
			ProviderLocationID:       sharedProviderLocationID,
			ProviderLabel:            city,
			Province:                 province,
			City:                     city,
			ParentProviderLocationID: sharedProviderLocationID,
			SourceEndpoint:           "destination/city",
		}},
		{{
			ProviderLocationID:       sharedProviderLocationID,
			ProviderLabel:            district,
			Province:                 province,
			City:                     city,
			District:                 district,
			PostalCode:               "0",
			ParentProviderLocationID: sharedProviderLocationID,
			SourceEndpoint:           "destination/district",
		}},
		{{
			ProviderLocationID:       sharedProviderLocationID,
			ProviderLabel:            subdistrict,
			Province:                 province,
			City:                     city,
			District:                 district,
			Subdistrict:              subdistrict,
			PostalCode:               postalCode,
			ParentProviderLocationID: sharedProviderLocationID,
			SourceEndpoint:           "destination/sub-district",
		}},
	}
	for _, items := range levels {
		count, err := repository.UpsertProviderLocations(ctx, provider, items)
		if err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("unexpected imported count: %d", count)
		}
	}

	var mappingCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM provider_location_mappings
		WHERE provider_code = $1
		  AND provider_location_id = $2
	`, provider, sharedProviderLocationID).Scan(&mappingCount); err != nil {
		t.Fatal(err)
	}
	if mappingCount != 4 {
		t.Fatalf(
			"expected provider ID to coexist across four levels, got %d",
			mappingCount,
		)
	}

	found, err := repository.Search(ctx, postalCode, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 {
		t.Fatalf("expected one postal result, got %#v", found)
	}
	if found[0].Subdistrict != subdistrict ||
		found[0].PostalCode != postalCode ||
		len(found[0].PostalCodes) != 1 ||
		found[0].PostalCodes[0] != postalCode {
		t.Fatalf("unexpected postal result: %#v", found[0])
	}

	if err := repository.MarkSyncCheckpointCompleted(
		ctx,
		provider,
		"integration",
		"subdistrict",
		sharedProviderLocationID,
		1,
		"response-hash",
	); err != nil {
		t.Fatal(err)
	}
	completed, err := repository.SyncCheckpointCompleted(
		ctx,
		provider,
		"integration",
		"subdistrict",
		sharedProviderLocationID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !completed {
		t.Fatal("expected checkpoint to be completed")
	}
	itemCount, foundCheckpoint, err := repository.SyncCheckpointItemCount(
		ctx,
		provider,
		"integration",
		"subdistrict",
		sharedProviderLocationID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !foundCheckpoint || itemCount != 1 {
		t.Fatalf(
			"unexpected checkpoint state: found=%t item_count=%d",
			foundCheckpoint,
			itemCount,
		)
	}
}
