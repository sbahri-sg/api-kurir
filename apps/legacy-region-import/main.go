package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/emisell/api-kurir/internal/database"
	"github.com/emisell/api-kurir/internal/locations"
	_ "modernc.org/sqlite"
)

const (
	providerCode     = "rajaongkir"
	defaultBatchSize = 500
)

type importOptions struct {
	source    string
	batchSize int
}

type legacyLocationWriter interface {
	ImportLegacyProviderLocations(
		context.Context,
		string,
		[]locations.ImportedProviderLocation,
	) (int, error)
}

type legacySource struct {
	level          string
	sourceEndpoint string
	query          string
}

var legacySources = []legacySource{
	{
		level:          "province",
		sourceEndpoint: "legacy-region-dump/provinces",
		query: `
			SELECT
				CAST(province.id AS TEXT),
				province.name,
				province.name,
				'', '', '', '', ''
			FROM provinces province
			ORDER BY province.id
		`,
	},
	{
		level:          "city",
		sourceEndpoint: "legacy-region-dump/cities",
		query: `
			SELECT
				CAST(city.id AS TEXT),
				city.name,
				province.name,
				city.name,
				'', '', '',
				CAST(province.id AS TEXT)
			FROM cities city
			JOIN provinces province ON province.id = city.province_id
			ORDER BY city.id
		`,
	},
	{
		level:          "district",
		sourceEndpoint: "legacy-region-dump/districts",
		query: `
			SELECT
				CAST(district.id AS TEXT),
				district.name,
				province.name,
				city.name,
				district.name,
				'',
				district.zip_code,
				CAST(city.id AS TEXT)
			FROM districts district
			JOIN cities city ON city.id = district.city_id
			JOIN provinces province ON province.id = city.province_id
			ORDER BY district.id
		`,
	},
	{
		level:          "subdistrict",
		sourceEndpoint: "legacy-region-dump/sub-districts",
		query: `
			SELECT
				CAST(subdistrict.id AS TEXT),
				subdistrict.name,
				province.name,
				city.name,
				district.name,
				subdistrict.name,
				subdistrict.zip_code,
				CAST(district.id AS TEXT)
			FROM sub_districts subdistrict
			JOIN districts district ON district.id = subdistrict.district_id
			JOIN cities city ON city.id = district.city_id
			JOIN provinces province ON province.id = city.province_id
			ORDER BY subdistrict.id
		`,
	},
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("legacy region import failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	options, err := parseOptions(os.Args[1:])
	if err != nil {
		return err
	}
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}

	sourcePath, err := filepath.Abs(options.source)
	if err != nil {
		return fmt.Errorf("resolve legacy region database path: %w", err)
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("open legacy region database: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("legacy region database must be a regular file")
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	sourceDB, err := sql.Open(
		"sqlite",
		"file:"+filepath.ToSlash(sourcePath)+"?mode=ro",
	)
	if err != nil {
		return fmt.Errorf("open legacy SQLite database: %w", err)
	}
	defer sourceDB.Close()
	sourceDB.SetMaxOpenConns(1)
	if err := sourceDB.PingContext(ctx); err != nil {
		return fmt.Errorf("ping legacy SQLite database: %w", err)
	}

	pool, err := database.Open(ctx, databaseURL, 0, 8)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := database.Migrate(ctx, pool); err != nil {
		return err
	}

	startedAt := time.Now()
	counts, err := importLegacyRegions(
		ctx,
		sourceDB,
		locations.NewPostgresRepository(pool),
		options.batchSize,
	)
	if err != nil {
		return err
	}
	total := 0
	for _, count := range counts {
		total += count
	}
	logger.Info(
		"legacy RajaOngkir region dump imported",
		"source", sourcePath,
		"provinces", counts["province"],
		"cities", counts["city"],
		"districts", counts["district"],
		"subdistricts", counts["subdistrict"],
		"total", total,
		"duration", time.Since(startedAt).String(),
	)
	return nil
}

func parseOptions(args []string) (importOptions, error) {
	flags := flag.NewFlagSet("legacy-region-import", flag.ContinueOnError)
	defaultSource := strings.TrimSpace(os.Getenv("LEGACY_REGION_DB_PATH"))
	source := flags.String(
		"source",
		defaultSource,
		"path to region-service SQLite regions.db",
	)
	batchSize := flags.Int(
		"batch-size",
		defaultBatchSize,
		"PostgreSQL upsert batch size",
	)
	if err := flags.Parse(args); err != nil {
		return importOptions{}, err
	}
	options := importOptions{
		source:    strings.TrimSpace(*source),
		batchSize: *batchSize,
	}
	if options.source == "" {
		return importOptions{}, errors.New(
			"-source or LEGACY_REGION_DB_PATH is required",
		)
	}
	if options.batchSize < 1 || options.batchSize > 5_000 {
		return importOptions{}, errors.New("-batch-size must be between 1 and 5000")
	}
	return options, nil
}

func importLegacyRegions(
	ctx context.Context,
	sourceDB *sql.DB,
	writer legacyLocationWriter,
	batchSize int,
) (map[string]int, error) {
	if sourceDB == nil || writer == nil {
		return nil, errors.New("legacy source and location writer are required")
	}
	if batchSize < 1 {
		return nil, errors.New("batch size must be positive")
	}

	counts := make(map[string]int, len(legacySources))
	for _, source := range legacySources {
		rows, err := sourceDB.QueryContext(ctx, source.query)
		if err != nil {
			return nil, fmt.Errorf("query legacy %s records: %w", source.level, err)
		}

		batch := make([]locations.ImportedProviderLocation, 0, batchSize)
		flush := func() error {
			if len(batch) == 0 {
				return nil
			}
			imported, err := writer.ImportLegacyProviderLocations(
				ctx,
				providerCode,
				batch,
			)
			if err != nil {
				return err
			}
			counts[source.level] += imported
			batch = batch[:0]
			return nil
		}

		for rows.Next() {
			var item locations.ImportedProviderLocation
			if err := rows.Scan(
				&item.ProviderLocationID,
				&item.ProviderLabel,
				&item.Province,
				&item.City,
				&item.District,
				&item.Subdistrict,
				&item.PostalCode,
				&item.ParentProviderLocationID,
			); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan legacy %s record: %w", source.level, err)
			}
			item.ProviderLocationID = strings.TrimSpace(item.ProviderLocationID)
			if _, err := strconv.ParseInt(item.ProviderLocationID, 10, 64); err != nil {
				rows.Close()
				return nil, fmt.Errorf(
					"legacy %s ID %q is invalid: %w",
					source.level,
					item.ProviderLocationID,
					err,
				)
			}
			item.SourceEndpoint = source.sourceEndpoint
			item.RetrievedAt = time.Now().UTC()
			batch = append(batch, item)
			if len(batch) == batchSize {
				if err := flush(); err != nil {
					rows.Close()
					return nil, fmt.Errorf("import legacy %s records: %w", source.level, err)
				}
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("iterate legacy %s records: %w", source.level, err)
		}
		rows.Close()
		if err := flush(); err != nil {
			return nil, fmt.Errorf("import legacy %s records: %w", source.level, err)
		}
	}
	return counts, nil
}
