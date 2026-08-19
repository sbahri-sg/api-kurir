package main

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/emisell/api-kurir/internal/locations"
	_ "modernc.org/sqlite"
)

type capturedLegacyWriter struct {
	items []locations.ImportedProviderLocation
}

func (w *capturedLegacyWriter) ImportLegacyProviderLocations(
	_ context.Context,
	provider string,
	items []locations.ImportedProviderLocation,
) (int, error) {
	if provider != providerCode {
		panic("unexpected provider: " + provider)
	}
	w.items = append(w.items, append([]locations.ImportedProviderLocation(nil), items...)...)
	return len(items), nil
}

func TestImportLegacyRegionsPreservesProviderIDsAndHierarchy(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "regions.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	statements := []string{
		`CREATE TABLE provinces (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`,
		`CREATE TABLE cities (id INTEGER PRIMARY KEY, province_id INTEGER NOT NULL, name TEXT NOT NULL)`,
		`CREATE TABLE districts (id INTEGER PRIMARY KEY, city_id INTEGER NOT NULL, name TEXT NOT NULL, zip_code TEXT NOT NULL)`,
		`CREATE TABLE sub_districts (id INTEGER PRIMARY KEY, district_id INTEGER NOT NULL, name TEXT NOT NULL, zip_code TEXT NOT NULL)`,
		`INSERT INTO provinces VALUES (32, 'MALUKU UTARA')`,
		`INSERT INTO cities VALUES (688, 32, 'KOTA TERNATE')`,
		`INSERT INTO districts VALUES (7126, 688, 'TERNATE SELATAN', '97719')`,
		`INSERT INTO sub_districts VALUES (82995, 7126, 'SASA', '97719')`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}

	writer := &capturedLegacyWriter{}
	counts, err := importLegacyRegions(context.Background(), db, writer, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, level := range []string{"province", "city", "district", "subdistrict"} {
		if counts[level] != 1 {
			t.Fatalf("%s count: got %d want 1", level, counts[level])
		}
	}
	if len(writer.items) != 4 {
		t.Fatalf("items: got %d want 4", len(writer.items))
	}

	province := writer.items[0]
	if province.ProviderLocationID != "32" ||
		province.Province != "MALUKU UTARA" ||
		province.ParentProviderLocationID != "" {
		t.Fatalf("unexpected province: %#v", province)
	}
	city := writer.items[1]
	if city.ProviderLocationID != "688" ||
		city.ParentProviderLocationID != "32" ||
		city.City != "KOTA TERNATE" {
		t.Fatalf("unexpected city: %#v", city)
	}
	district := writer.items[2]
	if district.ProviderLocationID != "7126" ||
		district.ParentProviderLocationID != "688" ||
		district.PostalCode != "97719" {
		t.Fatalf("unexpected district: %#v", district)
	}
	subdistrict := writer.items[3]
	if subdistrict.ProviderLocationID != "82995" ||
		subdistrict.ParentProviderLocationID != "7126" ||
		subdistrict.Subdistrict != "SASA" {
		t.Fatalf("unexpected subdistrict: %#v", subdistrict)
	}
}

func TestParseOptionsRequiresSourceAndBoundsBatch(t *testing.T) {
	t.Setenv("LEGACY_REGION_DB_PATH", "")
	if _, err := parseOptions(nil); err == nil {
		t.Fatal("expected missing source error")
	}
	if _, err := parseOptions([]string{"-source", "regions.db", "-batch-size", "0"}); err == nil {
		t.Fatal("expected invalid batch size error")
	}
	options, err := parseOptions([]string{"-source", "regions.db", "-batch-size", "100"})
	if err != nil {
		t.Fatal(err)
	}
	if options.source != "regions.db" || options.batchSize != 100 {
		t.Fatalf("unexpected options: %#v", options)
	}
}
