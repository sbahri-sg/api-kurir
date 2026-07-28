package locations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

type OfficialLocationRecord struct {
	Code        string
	ParentCode  string
	PublicID    string
	Level       string
	Province    string
	City        string
	District    string
	Subdistrict string
	PostalCode  string
}

type DatasetMetadata struct {
	SourceCode     string
	DatasetVersion string
	SourceURL      string
	SourceCommit   string
	SHA256         string
	RecordCount    int
	Metadata       map[string]any
}

type OfficialImportSummary struct {
	Provinces    int
	Cities       int
	Districts    int
	Subdistricts int
	PostalLinks  int
}

func OfficialPublicID(code string) string {
	return "loc_idn_" + strings.ReplaceAll(strings.TrimSpace(code), ".", "_")
}

func (r *PostgresRepository) ImportOfficialLocations(
	ctx context.Context,
	records []OfficialLocationRecord,
	datasets []DatasetMetadata,
	postalSourceURL string,
) (OfficialImportSummary, error) {
	if len(records) == 0 {
		return OfficialImportSummary{}, errors.New("official location dataset is empty")
	}
	if strings.TrimSpace(postalSourceURL) == "" {
		return OfficialImportSummary{}, errors.New("postal source URL is required")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return OfficialImportSummary{}, fmt.Errorf("begin official location import: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE official_location_stage (
			code text PRIMARY KEY,
			parent_code text,
			public_id text NOT NULL,
			level text NOT NULL,
			province text NOT NULL,
			city text,
			district text,
			subdistrict text,
			postal_code varchar(5)
		) ON COMMIT DROP
	`); err != nil {
		return OfficialImportSummary{}, fmt.Errorf("create official location stage: %w", err)
	}

	copied, err := tx.CopyFrom(
		ctx,
		pgx.Identifier{"official_location_stage"},
		[]string{
			"code",
			"parent_code",
			"public_id",
			"level",
			"province",
			"city",
			"district",
			"subdistrict",
			"postal_code",
		},
		pgx.CopyFromSlice(len(records), func(index int) ([]any, error) {
			record := records[index]
			return []any{
				record.Code,
				nullIfEmpty(record.ParentCode),
				record.PublicID,
				record.Level,
				record.Province,
				nullIfEmpty(record.City),
				nullIfEmpty(record.District),
				nullIfEmpty(record.Subdistrict),
				nullIfEmpty(record.PostalCode),
			}, nil
		}),
	)
	if err != nil {
		return OfficialImportSummary{}, fmt.Errorf("copy official location stage: %w", err)
	}
	if copied != int64(len(records)) {
		return OfficialImportSummary{}, fmt.Errorf(
			"copy official location stage: got %d rows, expected %d",
			copied,
			len(records),
		)
	}

	for _, level := range []string{"province", "city", "district", "subdistrict"} {
		if _, err := tx.Exec(ctx, `
			INSERT INTO locations (
				public_id,
				parent_id,
				level,
				province,
				city,
				district,
				subdistrict,
				postal_code,
				official_region_code,
				active
			)
			SELECT
				stage.public_id,
				parent.id,
				stage.level,
				stage.province,
				stage.city,
				stage.district,
				stage.subdistrict,
				stage.postal_code,
				stage.code,
				true
			FROM official_location_stage stage
			LEFT JOIN locations parent
			  ON parent.official_region_code = stage.parent_code
			WHERE stage.level = $1
			ON CONFLICT (official_region_code)
				WHERE official_region_code IS NOT NULL
			DO UPDATE
			SET parent_id = EXCLUDED.parent_id,
			    level = EXCLUDED.level,
			    province = EXCLUDED.province,
			    city = EXCLUDED.city,
			    district = EXCLUDED.district,
			    subdistrict = EXCLUDED.subdistrict,
			    postal_code = EXCLUDED.postal_code,
			    active = true,
			    updated_at = now()
		`, level); err != nil {
			return OfficialImportSummary{}, fmt.Errorf(
				"upsert official %s locations: %w",
				level,
				err,
			)
		}
	}

	var missingParents int
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM official_location_stage stage
		LEFT JOIN locations parent
		  ON parent.official_region_code = stage.parent_code
		WHERE stage.parent_code IS NOT NULL
		  AND parent.id IS NULL
	`).Scan(&missingParents); err != nil {
		return OfficialImportSummary{}, fmt.Errorf("validate official parents: %w", err)
	}
	if missingParents != 0 {
		return OfficialImportSummary{}, fmt.Errorf(
			"official location import has %d missing parents",
			missingParents,
		)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE locations location
		SET active = false,
		    updated_at = now()
		WHERE location.official_region_code IS NOT NULL
		  AND NOT EXISTS (
			SELECT 1
			FROM official_location_stage stage
			WHERE stage.code = location.official_region_code
		  )
	`); err != nil {
		return OfficialImportSummary{}, fmt.Errorf(
			"deactivate superseded official locations: %w",
			err,
		)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO postal_codes (code, active)
		SELECT DISTINCT postal_code, true
		FROM official_location_stage
		WHERE postal_code IS NOT NULL
		ON CONFLICT (code) DO UPDATE
		SET active = true,
		    updated_at = now()
	`); err != nil {
		return OfficialImportSummary{}, fmt.Errorf("upsert official postal codes: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE location_postal_codes
		SET active = false,
		    updated_at = now()
		WHERE provider_code = 'posindonesia'
		  AND source_type = 'official_registry'
	`); err != nil {
		return OfficialImportSummary{}, fmt.Errorf(
			"deactivate superseded official postal links: %w",
			err,
		)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO location_postal_codes (
			location_id,
			postal_code_id,
			provider_code,
			provider_location_id,
			provider_granularity,
			source_type,
			source_endpoint,
			normalized_response_hash,
			retrieved_at,
			verified_at,
			active
		)
		SELECT
			location.id,
			postal.id,
			'posindonesia',
			stage.code,
			'subdistrict',
			'official_registry',
			$1,
			encode(digest(stage.code || ':' || stage.postal_code, 'sha256'), 'hex'),
			now(),
			now(),
			true
		FROM official_location_stage stage
		JOIN locations location
		  ON location.official_region_code = stage.code
		JOIN postal_codes postal
		  ON postal.code = stage.postal_code
		WHERE stage.level = 'subdistrict'
		  AND stage.postal_code IS NOT NULL
		ON CONFLICT (location_id, postal_code_id, provider_code) DO UPDATE
		SET provider_location_id = EXCLUDED.provider_location_id,
		    provider_granularity = EXCLUDED.provider_granularity,
		    source_type = EXCLUDED.source_type,
		    source_endpoint = EXCLUDED.source_endpoint,
		    normalized_response_hash = EXCLUDED.normalized_response_hash,
		    retrieved_at = EXCLUDED.retrieved_at,
		    verified_at = EXCLUDED.verified_at,
		    active = true,
		    updated_at = now()
	`, postalSourceURL); err != nil {
		return OfficialImportSummary{}, fmt.Errorf("upsert official postal links: %w", err)
	}

	for _, dataset := range datasets {
		metadataJSON, err := json.Marshal(dataset.Metadata)
		if err != nil {
			return OfficialImportSummary{}, fmt.Errorf(
				"encode %s metadata: %w",
				dataset.SourceCode,
				err,
			)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE location_dataset_imports
			SET active = false
			WHERE source_code = $1
		`, dataset.SourceCode); err != nil {
			return OfficialImportSummary{}, fmt.Errorf(
				"deactivate old %s dataset: %w",
				dataset.SourceCode,
				err,
			)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO location_dataset_imports (
				source_code,
				dataset_version,
				source_url,
				source_commit,
				sha256,
				record_count,
				active,
				imported_at,
				metadata
			)
			VALUES ($1, $2, $3, $4, $5, $6, true, now(), $7::jsonb)
			ON CONFLICT (source_code, source_commit, sha256) DO UPDATE
			SET dataset_version = EXCLUDED.dataset_version,
			    source_url = EXCLUDED.source_url,
			    record_count = EXCLUDED.record_count,
			    active = true,
			    imported_at = now(),
			    metadata = EXCLUDED.metadata
		`,
			dataset.SourceCode,
			dataset.DatasetVersion,
			dataset.SourceURL,
			dataset.SourceCommit,
			dataset.SHA256,
			dataset.RecordCount,
			string(metadataJSON),
		); err != nil {
			return OfficialImportSummary{}, fmt.Errorf(
				"record %s dataset: %w",
				dataset.SourceCode,
				err,
			)
		}
	}

	summary := OfficialImportSummary{}
	if err := tx.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE level = 'province'),
			count(*) FILTER (WHERE level = 'city'),
			count(*) FILTER (WHERE level = 'district'),
			count(*) FILTER (WHERE level = 'subdistrict')
		FROM locations
		WHERE active
		  AND official_region_code IS NOT NULL
	`).Scan(
		&summary.Provinces,
		&summary.Cities,
		&summary.Districts,
		&summary.Subdistricts,
	); err != nil {
		return OfficialImportSummary{}, fmt.Errorf("summarize official locations: %w", err)
	}
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM location_postal_codes
		WHERE provider_code = 'posindonesia'
		  AND source_type = 'official_registry'
		  AND active
	`).Scan(&summary.PostalLinks); err != nil {
		return OfficialImportSummary{}, fmt.Errorf("summarize official postal links: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return OfficialImportSummary{}, fmt.Errorf("commit official location import: %w", err)
	}
	return summary, nil
}
