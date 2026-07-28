package locations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrProviderMappingNotFound = errors.New("provider location mapping not found")
var ErrLocationNotFound = errors.New("location not found")

type ImportedProviderLocation struct {
	ProviderLocationID       string
	ProviderLabel            string
	Province                 string
	City                     string
	District                 string
	Subdistrict              string
	PostalCode               string
	ParentProviderLocationID string
	SourceEndpoint           string
	RetrievedAt              time.Time
}

func (r *PostgresRepository) ResolveProviderLocationPair(
	ctx context.Context,
	originPublicID string,
	destinationPublicID string,
	providerCode string,
) (string, string, error) {
	var originProviderID, destinationProviderID string
	err := r.pool.QueryRow(ctx, `
		SELECT origin_mapping.provider_location_id, destination_mapping.provider_location_id
		FROM locations origin
		JOIN provider_location_mappings origin_mapping
		  ON origin_mapping.location_id = origin.id
		 AND origin_mapping.provider_code = $3
		 AND origin_mapping.active
		CROSS JOIN locations destination
		JOIN provider_location_mappings destination_mapping
		  ON destination_mapping.location_id = destination.id
		 AND destination_mapping.provider_code = $3
		 AND destination_mapping.active
		WHERE origin.public_id = $1
		  AND destination.public_id = $2
		  AND origin.active
		  AND destination.active
		ORDER BY
			CASE origin_mapping.granularity
				WHEN 'subdistrict' THEN 4
				WHEN 'district' THEN 3
				WHEN 'city' THEN 2
				ELSE 1
			END DESC,
			CASE destination_mapping.granularity
				WHEN 'subdistrict' THEN 4
				WHEN 'district' THEN 3
				WHEN 'city' THEN 2
				ELSE 1
			END DESC
		LIMIT 1
	`, originPublicID, destinationPublicID, providerCode).Scan(
		&originProviderID,
		&destinationProviderID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrProviderMappingNotFound
	}
	if err != nil {
		return "", "", fmt.Errorf("resolve provider location pair: %w", err)
	}
	return originProviderID, destinationProviderID, nil
}

func (r *PostgresRepository) ResolveProviderLocation(
	ctx context.Context,
	locationPublicID string,
	providerCode string,
) (string, error) {
	var providerLocationID string
	err := r.pool.QueryRow(ctx, `
		SELECT mapping.provider_location_id
		FROM locations location
		JOIN provider_location_mappings mapping
		  ON mapping.location_id = location.id
		 AND mapping.provider_code = $2
		 AND mapping.granularity = 'subdistrict'
		 AND mapping.active
		WHERE location.public_id = $1
		  AND location.level = 'subdistrict'
		  AND location.active
		LIMIT 1
	`,
		strings.TrimSpace(locationPublicID),
		strings.TrimSpace(strings.ToLower(providerCode)),
	).Scan(&providerLocationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrProviderMappingNotFound
	}
	if err != nil {
		return "", fmt.Errorf("resolve provider location: %w", err)
	}
	return providerLocationID, nil
}

func (r *PostgresRepository) FindByPublicID(
	ctx context.Context,
	publicID string,
) (Location, error) {
	var location Location
	err := r.pool.QueryRow(ctx, `
		SELECT
			location.public_id,
			coalesce(location.province, ''),
			coalesce(location.city, ''),
			coalesce(location.district, ''),
			coalesce(location.subdistrict, ''),
			coalesce(
				postal.codes[1],
				CASE
					WHEN location.postal_code ~ '^[0-9]{5}$'
					 AND location.postal_code <> '00000'
					THEN location.postal_code
				END,
				''
			),
			coalesce(postal.codes, ARRAY[]::text[])
		FROM locations location
		LEFT JOIN LATERAL (
			SELECT array_agg(DISTINCT code.code ORDER BY code.code) AS codes
			FROM location_postal_codes link
			JOIN postal_codes code ON code.id = link.postal_code_id
			WHERE link.location_id = location.id
			  AND link.active
			  AND code.active
		) postal ON true
		WHERE location.public_id = $1
		  AND location.level = 'subdistrict'
		  AND location.active
	`, strings.TrimSpace(publicID)).Scan(
		&location.PublicID,
		&location.Province,
		&location.City,
		&location.District,
		&location.Subdistrict,
		&location.PostalCode,
		&location.PostalCodes,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Location{}, ErrLocationNotFound
	}
	if err != nil {
		return Location{}, fmt.Errorf("find location by public ID: %w", err)
	}
	return location, nil
}

func (r *PostgresRepository) SaveProviderMapping(
	ctx context.Context,
	locationPublicID string,
	providerCode string,
	providerLocationID string,
	providerLocationName string,
	sourceEndpoint string,
) error {
	providerCode = strings.TrimSpace(strings.ToLower(providerCode))
	providerLocationID = strings.TrimSpace(providerLocationID)
	if providerCode == "" || providerLocationID == "" {
		return errors.New("provider code and location ID are required")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin provider mapping save: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var locationID string
	err = tx.QueryRow(ctx, `
		SELECT id::text
		FROM locations
		WHERE public_id = $1
		  AND level = 'subdistrict'
		  AND active
	`, strings.TrimSpace(locationPublicID)).Scan(&locationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrLocationNotFound
	}
	if err != nil {
		return fmt.Errorf("find provider mapping location: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		DELETE FROM provider_location_mappings
		WHERE location_id = $1::uuid
		  AND provider_code = $2
		  AND granularity = 'subdistrict'
		  AND provider_location_id <> $3
	`, locationID, providerCode, providerLocationID); err != nil {
		return fmt.Errorf("remove superseded provider mapping: %w", err)
	}

	hashInput := strings.Join([]string{
		providerCode,
		providerLocationID,
		strings.TrimSpace(providerLocationName),
		strings.TrimSpace(locationPublicID),
	}, "\x1f")
	mappingHash := sha256.Sum256([]byte(hashInput))
	if _, err := tx.Exec(ctx, `
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
		VALUES (
			$1::uuid,
			$2,
			$3,
			nullif($4, ''),
			'subdistrict',
			now(),
			'aggregator_direct',
			nullif($5, ''),
			$6,
			now(),
			1,
			true
		)
		ON CONFLICT (
			provider_code,
			granularity,
			provider_location_id
		) DO UPDATE
		SET location_id = EXCLUDED.location_id,
		    provider_location_name = EXCLUDED.provider_location_name,
		    verified_at = now(),
		    source_type = EXCLUDED.source_type,
		    source_endpoint = EXCLUDED.source_endpoint,
		    normalized_response_hash = EXCLUDED.normalized_response_hash,
		    retrieved_at = EXCLUDED.retrieved_at,
		    confidence = 1,
		    active = true,
		    updated_at = now()
	`,
		locationID,
		providerCode,
		providerLocationID,
		strings.TrimSpace(providerLocationName),
		strings.TrimSpace(sourceEndpoint),
		hex.EncodeToString(mappingHash[:]),
	); err != nil {
		return fmt.Errorf("upsert provider mapping: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit provider mapping save: %w", err)
	}
	return nil
}

func (r *PostgresRepository) UpsertProviderLocations(
	ctx context.Context,
	providerCode string,
	items []ImportedProviderLocation,
) (int, error) {
	providerCode = strings.TrimSpace(strings.ToLower(providerCode))
	if providerCode == "" {
		return 0, errors.New("provider code is required")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin provider location import: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	imported := 0
	for _, item := range items {
		item.ProviderLocationID = strings.TrimSpace(item.ProviderLocationID)
		if item.ProviderLocationID == "" {
			continue
		}

		item.PostalCode = normalizePostalCode(item.PostalCode)
		level := providerLocationLevel(item)
		parentLocationID, err := resolveParentLocationID(
			ctx,
			tx,
			providerCode,
			item.ParentProviderLocationID,
			level,
		)
		if err != nil {
			return 0, err
		}

		locationID, err := findMappedLocationID(
			ctx,
			tx,
			providerCode,
			item.ProviderLocationID,
			level,
		)
		if err != nil {
			return 0, err
		}
		if locationID == "" {
			locationID, err = findLocationByPath(ctx, tx, item)
			if err != nil {
				return 0, err
			}
		}

		if locationID == "" {
			publicID := canonicalPublicID(item)
			if err := tx.QueryRow(ctx, `
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
			VALUES ($1, $2::uuid, $3, $4, $5, $6, $7, $8, true)
			ON CONFLICT (public_id) DO UPDATE
			SET parent_id = coalesce(EXCLUDED.parent_id, locations.parent_id),
			    level = EXCLUDED.level,
			    province = EXCLUDED.province,
			    city = EXCLUDED.city,
			    district = EXCLUDED.district,
			    subdistrict = EXCLUDED.subdistrict,
			    postal_code = coalesce(EXCLUDED.postal_code, locations.postal_code),
			    active = true,
			    updated_at = now()
			RETURNING id::text
		`,
				publicID,
				parentLocationID,
				level,
				nullIfEmpty(item.Province),
				nullIfEmpty(item.City),
				nullIfEmpty(item.District),
				nullIfEmpty(item.Subdistrict),
				nullIfEmpty(item.PostalCode),
			).Scan(&locationID); err != nil {
				return 0, fmt.Errorf("upsert provider location %s: %w", item.ProviderLocationID, err)
			}
		} else {
			if _, err := tx.Exec(ctx, `
				UPDATE locations
				SET parent_id = coalesce($2::uuid, parent_id),
				    level = $3,
				    province = coalesce(nullif($4, ''), province),
				    city = coalesce(nullif($5, ''), city),
				    district = coalesce(nullif($6, ''), district),
				    subdistrict = coalesce(nullif($7, ''), subdistrict),
				    postal_code = coalesce(nullif($8, ''), postal_code),
				    active = true,
				    updated_at = now()
				WHERE id = $1::uuid
			`,
				locationID,
				parentLocationID,
				level,
				strings.TrimSpace(item.Province),
				strings.TrimSpace(item.City),
				strings.TrimSpace(item.District),
				strings.TrimSpace(item.Subdistrict),
				item.PostalCode,
			); err != nil {
				return 0, fmt.Errorf(
					"update provider location %s: %w",
					item.ProviderLocationID,
					err,
				)
			}
		}

		retrievedAt := item.RetrievedAt
		if retrievedAt.IsZero() {
			retrievedAt = time.Now()
		}
		responseHash := providerLocationHash(providerCode, item)
		if _, err := tx.Exec(ctx, `
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
			VALUES (
				$1::uuid,
				$2,
				$3,
				$4,
				$5,
				now(),
				'aggregator_direct',
				nullif($6, ''),
				$7,
				$8,
				1,
				true
			)
			ON CONFLICT (
				provider_code,
				granularity,
				provider_location_id
			) DO UPDATE
			SET location_id = EXCLUDED.location_id,
			    provider_location_name = EXCLUDED.provider_location_name,
			    granularity = EXCLUDED.granularity,
			    verified_at = now(),
			    source_type = EXCLUDED.source_type,
			    source_endpoint = EXCLUDED.source_endpoint,
			    normalized_response_hash = EXCLUDED.normalized_response_hash,
			    retrieved_at = EXCLUDED.retrieved_at,
			    confidence = EXCLUDED.confidence,
			    active = true,
			    updated_at = now()
		`,
			locationID,
			providerCode,
			item.ProviderLocationID,
			nullIfEmpty(item.ProviderLabel),
			level,
			strings.TrimSpace(item.SourceEndpoint),
			responseHash,
			retrievedAt,
		); err != nil {
			return 0, fmt.Errorf("upsert provider mapping %s: %w", item.ProviderLocationID, err)
		}

		if item.PostalCode != "" {
			if err := upsertPostalCode(
				ctx,
				tx,
				locationID,
				providerCode,
				item.ProviderLocationID,
				item.PostalCode,
				level,
				item.SourceEndpoint,
				responseHash,
				retrievedAt,
			); err != nil {
				return 0, err
			}
		}
		imported++
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit provider location import: %w", err)
	}
	return imported, nil
}

func (r *PostgresRepository) ListProviderChildren(
	ctx context.Context,
	providerCode string,
	parentProviderLocationID string,
	level string,
) ([]ImportedProviderLocation, error) {
	providerCode = strings.TrimSpace(strings.ToLower(providerCode))
	parentProviderLocationID = strings.TrimSpace(parentProviderLocationID)

	query := `
		SELECT
			mapping.provider_location_id,
			coalesce(mapping.provider_location_name, ''),
			coalesce(location.province, ''),
			coalesce(location.city, ''),
			coalesce(location.district, ''),
			coalesce(location.subdistrict, ''),
			coalesce(location.postal_code, '')
		FROM provider_location_mappings mapping
		JOIN locations location ON location.id = mapping.location_id
		WHERE mapping.provider_code = $1
		  AND mapping.granularity = $3
		  AND mapping.active
		  AND location.active
		  AND (
			($2 = '' AND location.parent_id IS NULL)
			OR EXISTS (
				SELECT 1
				FROM provider_location_mappings parent_mapping
					WHERE parent_mapping.location_id = location.parent_id
					  AND parent_mapping.provider_code = $1
					  AND parent_mapping.provider_location_id = $2
					  AND parent_mapping.granularity = $4
					  AND parent_mapping.active
			)
		  )
		ORDER BY mapping.provider_location_id
	`
	rows, err := r.pool.Query(
		ctx,
		query,
		providerCode,
		parentProviderLocationID,
		level,
		parentLocationLevel(level),
	)
	if err != nil {
		return nil, fmt.Errorf("list provider location children: %w", err)
	}
	defer rows.Close()

	result := make([]ImportedProviderLocation, 0)
	for rows.Next() {
		var item ImportedProviderLocation
		if err := rows.Scan(
			&item.ProviderLocationID,
			&item.ProviderLabel,
			&item.Province,
			&item.City,
			&item.District,
			&item.Subdistrict,
			&item.PostalCode,
		); err != nil {
			return nil, fmt.Errorf("scan provider location child: %w", err)
		}
		item.ParentProviderLocationID = parentProviderLocationID
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate provider location children: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) SyncCheckpointCompleted(
	ctx context.Context,
	providerCode string,
	credentialAlias string,
	endpointLevel string,
	parentProviderLocationID string,
) (bool, error) {
	_, completed, err := r.SyncCheckpointItemCount(
		ctx,
		providerCode,
		credentialAlias,
		endpointLevel,
		parentProviderLocationID,
	)
	return completed, err
}

func (r *PostgresRepository) SyncCheckpointItemCount(
	ctx context.Context,
	providerCode string,
	credentialAlias string,
	endpointLevel string,
	parentProviderLocationID string,
) (int, bool, error) {
	var itemCount int
	err := r.pool.QueryRow(ctx, `
		SELECT item_count
		FROM provider_location_sync_checkpoints
		WHERE provider_code = $1
		  AND credential_alias = $2
		  AND endpoint_level = $3
		  AND parent_provider_location_id = $4
	`,
		strings.TrimSpace(strings.ToLower(providerCode)),
		strings.TrimSpace(credentialAlias),
		endpointLevel,
		strings.TrimSpace(parentProviderLocationID),
	).Scan(&itemCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("read location sync checkpoint: %w", err)
	}
	return itemCount, true, nil
}

func (r *PostgresRepository) MarkSyncCheckpointCompleted(
	ctx context.Context,
	providerCode string,
	credentialAlias string,
	endpointLevel string,
	parentProviderLocationID string,
	itemCount int,
	responseHash string,
) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO provider_location_sync_checkpoints (
			provider_code,
			credential_alias,
			endpoint_level,
			parent_provider_location_id,
			item_count,
			response_hash,
			completed_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, now())
		ON CONFLICT (
			provider_code,
			credential_alias,
			endpoint_level,
			parent_provider_location_id
		) DO UPDATE
		SET item_count = EXCLUDED.item_count,
		    response_hash = EXCLUDED.response_hash,
		    completed_at = EXCLUDED.completed_at,
		    updated_at = now()
	`,
		strings.TrimSpace(strings.ToLower(providerCode)),
		strings.TrimSpace(credentialAlias),
		endpointLevel,
		strings.TrimSpace(parentProviderLocationID),
		itemCount,
		responseHash,
	)
	if err != nil {
		return fmt.Errorf("write location sync checkpoint: %w", err)
	}
	return nil
}

func (r *PostgresRepository) DeactivateMissingProviderChildren(
	ctx context.Context,
	providerCode string,
	parentProviderLocationID string,
	level string,
	activeProviderLocationIDs []string,
) error {
	providerCode = strings.TrimSpace(strings.ToLower(providerCode))
	parentProviderLocationID = strings.TrimSpace(parentProviderLocationID)
	if activeProviderLocationIDs == nil {
		activeProviderLocationIDs = []string{}
	}

	rows, err := r.pool.Query(ctx, `
		UPDATE provider_location_mappings mapping
		SET active = false,
		    updated_at = now()
		FROM locations location
		WHERE location.id = mapping.location_id
		  AND mapping.provider_code = $1
		  AND mapping.granularity = $3
		  AND mapping.active
		  AND NOT (mapping.provider_location_id = ANY($4::text[]))
		  AND (
			($2 = '' AND location.parent_id IS NULL)
			OR EXISTS (
				SELECT 1
				FROM provider_location_mappings parent_mapping
					WHERE parent_mapping.location_id = location.parent_id
					  AND parent_mapping.provider_code = $1
					  AND parent_mapping.provider_location_id = $2
					  AND parent_mapping.granularity = $5
				)
		  )
		RETURNING mapping.provider_location_id
	`,
		providerCode,
		parentProviderLocationID,
		level,
		activeProviderLocationIDs,
		parentLocationLevel(level),
	)
	if err != nil {
		return fmt.Errorf("deactivate missing provider locations: %w", err)
	}
	defer rows.Close()

	missingIDs := make([]string, 0)
	for rows.Next() {
		var providerLocationID string
		if err := rows.Scan(&providerLocationID); err != nil {
			return fmt.Errorf("scan deactivated provider location: %w", err)
		}
		missingIDs = append(missingIDs, providerLocationID)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate deactivated provider locations: %w", err)
	}
	if len(missingIDs) == 0 {
		return nil
	}

	if _, err := r.pool.Exec(ctx, `
		UPDATE location_postal_codes
		SET active = false,
		    updated_at = now()
			WHERE provider_code = $1
			  AND provider_granularity = $3
			  AND provider_location_id = ANY($2::text[])
			  AND active
		`, providerCode, missingIDs, level); err != nil {
		return fmt.Errorf("deactivate missing provider postal codes: %w", err)
	}
	return nil
}

func resolveParentLocationID(
	ctx context.Context,
	tx pgx.Tx,
	providerCode string,
	parentProviderLocationID string,
	childLevel string,
) (any, error) {
	parentProviderLocationID = strings.TrimSpace(parentProviderLocationID)
	if parentProviderLocationID == "" {
		return nil, nil
	}
	var locationID string
	err := tx.QueryRow(ctx, `
		SELECT location_id::text
		FROM provider_location_mappings
		WHERE provider_code = $1
		  AND provider_location_id = $2
		  AND granularity = $3
		  AND active
	`,
		providerCode,
		parentProviderLocationID,
		parentLocationLevel(childLevel),
	).Scan(&locationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf(
			"parent provider location %s is not imported",
			parentProviderLocationID,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("resolve parent provider location: %w", err)
	}
	return locationID, nil
}

func findMappedLocationID(
	ctx context.Context,
	tx pgx.Tx,
	providerCode string,
	providerLocationID string,
	level string,
) (string, error) {
	var locationID string
	err := tx.QueryRow(ctx, `
		SELECT location_id::text
		FROM provider_location_mappings
		WHERE provider_code = $1
		  AND provider_location_id = $2
		  AND granularity = $3
		LIMIT 1
	`, providerCode, providerLocationID, level).Scan(&locationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("find mapped provider location: %w", err)
	}
	return locationID, nil
}

func findLocationByPath(
	ctx context.Context,
	tx pgx.Tx,
	item ImportedProviderLocation,
) (string, error) {
	var locationID string
	err := tx.QueryRow(ctx, `
		SELECT id::text
		FROM locations
		WHERE lower(coalesce(province, '')) = lower($1)
		  AND lower(coalesce(city, '')) = lower($2)
		  AND lower(coalesce(district, '')) = lower($3)
		  AND lower(coalesce(subdistrict, '')) = lower($4)
		ORDER BY
			(official_region_code IS NOT NULL) DESC,
			active DESC,
			updated_at DESC
		LIMIT 1
	`,
		strings.TrimSpace(item.Province),
		strings.TrimSpace(item.City),
		strings.TrimSpace(item.District),
		strings.TrimSpace(item.Subdistrict),
	).Scan(&locationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("find location by hierarchy: %w", err)
	}
	return locationID, nil
}

func upsertPostalCode(
	ctx context.Context,
	tx pgx.Tx,
	locationID string,
	providerCode string,
	providerLocationID string,
	postalCode string,
	level string,
	sourceEndpoint string,
	responseHash string,
	retrievedAt time.Time,
) error {
	var postalCodeID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO postal_codes (code)
		VALUES ($1)
		ON CONFLICT (code) DO UPDATE
		SET active = true,
		    updated_at = now()
		RETURNING id::text
	`, postalCode).Scan(&postalCodeID); err != nil {
		return fmt.Errorf("upsert postal code %s: %w", postalCode, err)
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
		VALUES (
			$1::uuid,
			$2::uuid,
			$3,
			$4,
			$5,
			'aggregator_direct',
			nullif($6, ''),
			$7,
			$8,
			now(),
			true
		)
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
	`,
		locationID,
		postalCodeID,
		providerCode,
		providerLocationID,
		level,
		strings.TrimSpace(sourceEndpoint),
		responseHash,
		retrievedAt,
	); err != nil {
		return fmt.Errorf("map postal code %s to location: %w", postalCode, err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE location_postal_codes
		SET active = false,
		    updated_at = now()
		WHERE location_id = $1::uuid
		  AND provider_code = $2
		  AND provider_location_id = $3
		  AND provider_granularity = $5
		  AND postal_code_id <> $4::uuid
		  AND active
	`,
		locationID,
		providerCode,
		providerLocationID,
		postalCodeID,
		level,
	); err != nil {
		return fmt.Errorf("deactivate superseded postal code: %w", err)
	}
	return nil
}

func canonicalPublicID(item ImportedProviderLocation) string {
	identity := strings.Join([]string{
		normalizeLocationName(item.Province),
		normalizeLocationName(item.City),
		normalizeLocationName(item.District),
		normalizeLocationName(item.Subdistrict),
	}, "|")
	hash := sha256.Sum256([]byte(identity))
	return "loc_idn_" + hex.EncodeToString(hash[:8])
}

func providerLocationHash(providerCode string, item ImportedProviderLocation) string {
	payload := strings.Join([]string{
		strings.TrimSpace(strings.ToLower(providerCode)),
		strings.TrimSpace(item.ProviderLocationID),
		strings.TrimSpace(item.ProviderLabel),
		strings.TrimSpace(item.Province),
		strings.TrimSpace(item.City),
		strings.TrimSpace(item.District),
		strings.TrimSpace(item.Subdistrict),
		normalizePostalCode(item.PostalCode),
	}, "\x1f")
	hash := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(hash[:])
}

func normalizeLocationName(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}

func normalizePostalCode(value string) string {
	value = strings.TrimSpace(value)
	if len(value) != 5 || value == "00000" {
		return ""
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return ""
		}
	}
	return value
}

func providerLocationLevel(item ImportedProviderLocation) string {
	switch {
	case strings.TrimSpace(item.Subdistrict) != "":
		return "subdistrict"
	case strings.TrimSpace(item.District) != "":
		return "district"
	case strings.TrimSpace(item.City) != "":
		return "city"
	default:
		return "province"
	}
}

func parentLocationLevel(level string) string {
	switch level {
	case "city":
		return "province"
	case "district":
		return "city"
	case "subdistrict":
		return "district"
	default:
		return ""
	}
}

func nullIfEmpty(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}
