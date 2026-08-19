package locations

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLegacyBulkImportAndHierarchyLookup(t *testing.T) {
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
	provider := "legacy-integration-" + suffix
	postalCode := "65432"
	repository := NewPostgresRepository(pool)
	t.Cleanup(func() {
		cleanupCtx := context.Background()
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
		_, _ = pool.Exec(
			cleanupCtx,
			"DELETE FROM locations WHERE public_id LIKE $1",
			"loc_legacy_"+provider+"_%",
		)
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
			ProviderLocationID: "32",
			ProviderLabel:      "MALUKU UTARA",
			Province:           "MALUKU UTARA",
			SourceEndpoint:     "legacy/provinces",
		}},
		{{
			ProviderLocationID:       "32",
			ProviderLabel:            "TERNATE",
			Province:                 "MALUKU UTARA",
			City:                     "TERNATE",
			ParentProviderLocationID: "32",
			SourceEndpoint:           "legacy/cities",
		}},
		{{
			ProviderLocationID:       "32",
			ProviderLabel:            "TERNATE SELATAN",
			Province:                 "MALUKU UTARA",
			City:                     "TERNATE",
			District:                 "TERNATE SELATAN",
			ParentProviderLocationID: "32",
			SourceEndpoint:           "legacy/districts",
		}},
		{{
			ProviderLocationID:       "32",
			ProviderLabel:            "SASA",
			Province:                 "MALUKU UTARA",
			City:                     "TERNATE",
			District:                 "TERNATE SELATAN",
			Subdistrict:              "SASA",
			PostalCode:               postalCode,
			ParentProviderLocationID: "32",
			SourceEndpoint:           "legacy/sub-districts",
		}},
	}
	for _, items := range levels {
		count, err := repository.ImportLegacyProviderLocations(ctx, provider, items)
		if err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("imported count: got %d want 1", count)
		}
	}

	provinces, err := repository.ListLegacyHierarchy(ctx, provider, "province", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(provinces) != 1 || provinces[0].ID != "32" || provinces[0].Name != "MALUKU UTARA" {
		t.Fatalf("unexpected provinces: %#v", provinces)
	}
	subdistricts, err := repository.ListLegacyHierarchy(ctx, provider, "subdistrict", "32")
	if err != nil {
		t.Fatal(err)
	}
	if len(subdistricts) != 1 ||
		subdistricts[0].ID != "32" ||
		subdistricts[0].ProvinceID != "32" ||
		subdistricts[0].CityID != "32" ||
		subdistricts[0].DistrictID != "32" ||
		subdistricts[0].PostalCode != postalCode {
		t.Fatalf("unexpected subdistrict hierarchy: %#v", subdistricts)
	}
	searchResult, err := repository.SearchLegacy(ctx, provider, "Sasa", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(searchResult) != 1 ||
		searchResult[0].ID != "32" ||
		searchResult[0].Name != "SASA" ||
		searchResult[0].PostalCode != postalCode {
		t.Fatalf("unexpected legacy search result: %#v", searchResult)
	}
	rawSearchResult, err := repository.SearchLegacy(
		ctx,
		provider,
		"Jl. Mawar No. 12, Kel. Sasa, Kec. Ternate Selatan, Kota Ternate, Maluku Utara 65432",
		20,
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(rawSearchResult) != 1 || rawSearchResult[0].ID != "32" {
		t.Fatalf("unexpected normalized raw-address result: %#v", rawSearchResult)
	}

	publicID, err := repository.ResolveLegacyPublicID(ctx, provider, "district", "32")
	if err != nil {
		t.Fatal(err)
	}
	if publicID == "" {
		t.Fatal("resolved public ID must not be empty")
	}
	found, err := repository.FindLegacyRegion(ctx, provider, "district", "32")
	if err != nil {
		t.Fatal(err)
	}
	if found.CanonicalPublicID != publicID || found.Name != "TERNATE SELATAN" {
		t.Fatalf("unexpected district: %#v", found)
	}
}
