package locations

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ImportLegacyProviderLocations performs a set-based import of one hierarchy
// level from the old region-service dump. Existing mappings keep their current
// canonical location; new IDs receive a stable internal location and can later
// be shared with mappings from other providers.
func (r *PostgresRepository) ImportLegacyProviderLocations(
	ctx context.Context,
	providerCode string,
	items []ImportedProviderLocation,
) (int, error) {
	providerCode = strings.ToLower(strings.TrimSpace(providerCode))
	if providerCode == "" {
		return 0, errors.New("provider code is required")
	}
	if len(items) == 0 {
		return 0, nil
	}

	level := providerLocationLevel(items[0])
	rows := make([][]any, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		item.ProviderLocationID = strings.TrimSpace(item.ProviderLocationID)
		if item.ProviderLocationID == "" {
			return 0, errors.New("provider location ID is required")
		}
		if providerLocationLevel(item) != level {
			return 0, errors.New("legacy import batch must contain one hierarchy level")
		}
		if _, duplicate := seen[item.ProviderLocationID]; duplicate {
			return 0, fmt.Errorf(
				"duplicate legacy %s ID %q in batch",
				level,
				item.ProviderLocationID,
			)
		}
		seen[item.ProviderLocationID] = struct{}{}
		retrievedAt := item.RetrievedAt.UTC()
		if item.RetrievedAt.IsZero() {
			retrievedAt = time.Now().UTC()
		}
		rows = append(rows, []any{
			item.ProviderLocationID,
			strings.TrimSpace(item.ProviderLabel),
			strings.TrimSpace(item.Province),
			strings.TrimSpace(item.City),
			strings.TrimSpace(item.District),
			strings.TrimSpace(item.Subdistrict),
			normalizePostalCode(item.PostalCode),
			strings.TrimSpace(item.ParentProviderLocationID),
			strings.TrimSpace(item.SourceEndpoint),
			retrievedAt,
		})
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin legacy provider import: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE legacy_region_stage (
			provider_location_id text PRIMARY KEY,
			provider_location_name text NOT NULL,
			province text NOT NULL,
			city text NOT NULL,
			district text NOT NULL,
			subdistrict text NOT NULL,
			postal_code text NOT NULL,
			parent_provider_location_id text NOT NULL,
			source_endpoint text NOT NULL,
			retrieved_at timestamptz NOT NULL
		) ON COMMIT DROP
	`); err != nil {
		return 0, fmt.Errorf("create legacy region staging table: %w", err)
	}

	if _, err := tx.CopyFrom(
		ctx,
		pgx.Identifier{"legacy_region_stage"},
		[]string{
			"provider_location_id",
			"provider_location_name",
			"province",
			"city",
			"district",
			"subdistrict",
			"postal_code",
			"parent_provider_location_id",
			"source_endpoint",
			"retrieved_at",
		},
		pgx.CopyFromRows(rows),
	); err != nil {
		return 0, fmt.Errorf("copy legacy %s staging rows: %w", level, err)
	}

	if level != "province" {
		var missingParents int
		if err := tx.QueryRow(ctx, `
			SELECT count(*)
			FROM legacy_region_stage stage
			LEFT JOIN provider_location_mappings parent_mapping
			  ON parent_mapping.provider_code = $1
			 AND parent_mapping.granularity = $2
			 AND parent_mapping.provider_location_id = stage.parent_provider_location_id
			 AND parent_mapping.active
			WHERE parent_mapping.id IS NULL
		`, providerCode, parentLocationLevel(level)).Scan(&missingParents); err != nil {
			return 0, fmt.Errorf("validate legacy %s parents: %w", level, err)
		}
		if missingParents > 0 {
			return 0, fmt.Errorf(
				"legacy %s import has %d unmapped parent IDs",
				level,
				missingParents,
			)
		}

		// A provider mapping may have been moved from a generated legacy
		// location to an official canonical location by a later sync. Existing
		// child mappings must follow that move as well; otherwise hierarchy
		// lookups can only see children whose location happened to use the new
		// parent already. Re-importing the dump is intentionally self-healing.
		if _, err := tx.Exec(ctx, `
			UPDATE locations location
			SET parent_id = parent_mapping.location_id,
			    updated_at = now()
			FROM legacy_region_stage stage
			JOIN provider_location_mappings existing
			  ON existing.provider_code = $1
			 AND existing.granularity = $2
			 AND existing.provider_location_id = stage.provider_location_id
			 AND existing.active
			JOIN provider_location_mappings parent_mapping
			  ON parent_mapping.provider_code = $1
			 AND parent_mapping.granularity = $3
			 AND parent_mapping.provider_location_id = stage.parent_provider_location_id
			 AND parent_mapping.active
			WHERE location.id = existing.location_id
			  AND location.parent_id IS DISTINCT FROM parent_mapping.location_id
		`, providerCode, level, parentLocationLevel(level)); err != nil {
			return 0, fmt.Errorf("repair legacy %s parent hierarchy: %w", level, err)
		}
	}

	if _, err := tx.Exec(ctx, `
		WITH staged AS (
			SELECT
				stage.*,
				existing.location_id AS existing_location_id,
				parent_mapping.location_id AS parent_location_id,
				'loc_legacy_' || $1 || '_' || $2 || '_' ||
				substring(
					encode(
						digest($1 || ':' || $2 || ':' || stage.provider_location_id, 'sha256'),
						'hex'
					),
					1,
					16
				) AS generated_public_id
			FROM legacy_region_stage stage
			LEFT JOIN provider_location_mappings existing
			  ON existing.provider_code = $1
			 AND existing.granularity = $2
			 AND existing.provider_location_id = stage.provider_location_id
			LEFT JOIN provider_location_mappings parent_mapping
			  ON parent_mapping.provider_code = $1
			 AND parent_mapping.granularity = $3
			 AND parent_mapping.provider_location_id = stage.parent_provider_location_id
			 AND parent_mapping.active
		),
		inserted_locations AS (
			INSERT INTO locations (
				public_id,
				parent_id,
				level,
				province,
				city,
				district,
				subdistrict,
				postal_code,
				active
			)
			SELECT
				staged.generated_public_id,
				staged.parent_location_id,
				$2,
				nullif(staged.province, ''),
				nullif(staged.city, ''),
				nullif(staged.district, ''),
				nullif(staged.subdistrict, ''),
				nullif(staged.postal_code, ''),
				true
			FROM staged
			WHERE staged.existing_location_id IS NULL
			ON CONFLICT (public_id) DO UPDATE
			SET parent_id = coalesce(EXCLUDED.parent_id, locations.parent_id),
			    province = coalesce(EXCLUDED.province, locations.province),
			    city = coalesce(EXCLUDED.city, locations.city),
			    district = coalesce(EXCLUDED.district, locations.district),
			    subdistrict = coalesce(EXCLUDED.subdistrict, locations.subdistrict),
			    postal_code = coalesce(EXCLUDED.postal_code, locations.postal_code),
			    active = true,
			    updated_at = now()
			RETURNING id, public_id
		),
		resolved AS (
			SELECT
				staged.*,
				coalesce(staged.existing_location_id, inserted_locations.id) AS location_id
			FROM staged
			LEFT JOIN inserted_locations
			  ON inserted_locations.public_id = staged.generated_public_id
		)
		INSERT INTO provider_location_mappings (
			location_id,
			provider_code,
			provider_location_id,
			provider_location_name,
			granularity,
			verified_at,
			source_type,
			source_endpoint,
			normalized_response_hash,
			retrieved_at,
			confidence,
			active
		)
		SELECT
			resolved.location_id,
			$1,
			resolved.provider_location_id,
			nullif(resolved.provider_location_name, ''),
			$2,
			now(),
			'legacy_import',
			nullif(resolved.source_endpoint, ''),
			encode(
				digest(
					concat_ws(
						E'\\x1f',
						$1,
						$2,
						resolved.provider_location_id,
						resolved.provider_location_name,
						resolved.province,
						resolved.city,
						resolved.district,
						resolved.subdistrict,
						resolved.postal_code
					),
					'sha256'
				),
				'hex'
			),
			resolved.retrieved_at,
			1,
			true
		FROM resolved
		ON CONFLICT (provider_code, granularity, provider_location_id) DO UPDATE
		SET provider_location_name = EXCLUDED.provider_location_name,
		    verified_at = EXCLUDED.verified_at,
		    source_type = EXCLUDED.source_type,
		    source_endpoint = EXCLUDED.source_endpoint,
		    normalized_response_hash = EXCLUDED.normalized_response_hash,
		    retrieved_at = EXCLUDED.retrieved_at,
		    confidence = EXCLUDED.confidence,
		    active = true,
		    updated_at = now()
	`, providerCode, level, parentLocationLevel(level)); err != nil {
		return 0, fmt.Errorf("upsert legacy %s locations and mappings: %w", level, err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO postal_codes (code, active)
		SELECT DISTINCT stage.postal_code, true
		FROM legacy_region_stage stage
		WHERE stage.postal_code ~ '^[0-9]{5}$'
		  AND stage.postal_code <> '00000'
		ON CONFLICT (code) DO UPDATE
		SET active = true,
		    updated_at = now()
	`); err != nil {
		return 0, fmt.Errorf("upsert legacy %s postal codes: %w", level, err)
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
			mapping.location_id,
			postal.id,
			$1,
			stage.provider_location_id,
			$2,
			'legacy_import',
			nullif(stage.source_endpoint, ''),
			mapping.normalized_response_hash,
			stage.retrieved_at,
			now(),
			true
		FROM legacy_region_stage stage
		JOIN provider_location_mappings mapping
		  ON mapping.provider_code = $1
		 AND mapping.granularity = $2
		 AND mapping.provider_location_id = stage.provider_location_id
		JOIN postal_codes postal ON postal.code = stage.postal_code
		WHERE stage.postal_code ~ '^[0-9]{5}$'
		  AND stage.postal_code <> '00000'
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
	`, providerCode, level); err != nil {
		return 0, fmt.Errorf("map legacy %s postal codes: %w", level, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit legacy %s import: %w", level, err)
	}
	return len(items), nil
}
