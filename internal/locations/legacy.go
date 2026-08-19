package locations

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// LegacyRegion is a RajaOngkir region identifier projected onto the local
// canonical location tree. Provider IDs deliberately remain strings because
// other providers may use UUIDs or alphanumeric identifiers.
type LegacyRegion struct {
	ID                string
	CanonicalPublicID string
	Name              string
	PostalCode        string
	ProvinceID        string
	ProvinceName      string
	CityID            string
	CityName          string
	DistrictID        string
	DistrictName      string
}

// LegacyRepository is the compatibility contract used by the old Emisell
// region-service facade. Bare numeric IDs in this contract always belong to
// the requested provider and hierarchy level; they are never interpreted as
// Kemendagri compatibility IDs.
type LegacyRepository interface {
	SearchLegacy(
		ctx context.Context,
		providerCode string,
		search string,
		limit int,
		offset int,
	) ([]LegacyRegion, error)
	ListLegacyHierarchy(
		ctx context.Context,
		providerCode string,
		level string,
		parentProviderLocationID string,
	) ([]LegacyRegion, error)
	FindLegacyRegion(
		ctx context.Context,
		providerCode string,
		level string,
		providerLocationID string,
	) (LegacyRegion, error)
	ResolveLegacyPublicID(
		ctx context.Context,
		providerCode string,
		level string,
		providerLocationID string,
	) (string, error)
}

func (r *PostgresRepository) SearchLegacy(
	ctx context.Context,
	providerCode string,
	search string,
	limit int,
	offset int,
) ([]LegacyRegion, error) {
	providerCode = strings.ToLower(strings.TrimSpace(providerCode))
	terms := normalizeLocationSearchTerms(search)
	if providerCode == "" || len(terms) == 0 {
		return nil, ErrLocationNotFound
	}
	if limit < 1 || limit > 1000 || offset < 0 {
		return nil, errors.New("invalid legacy search pagination")
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, fmt.Errorf("begin legacy search: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SET LOCAL max_parallel_workers_per_gather = 0"); err != nil {
		return nil, fmt.Errorf("configure legacy search: %w", err)
	}

	rows, err := tx.Query(ctx, `
		WITH input_terms AS (
			SELECT DISTINCT term
			FROM unnest($2::text[]) AS term
		),
		location_candidate_terms AS (
			SELECT
				mapping.id AS mapping_id,
				term.term,
				CASE
					WHEN lower(coalesce(location.subdistrict, '')) = term.term THEN 0
					WHEN lower(coalesce(location.district, '')) = term.term THEN 1
					WHEN lower(coalesce(location.city, '')) LIKE '%' || term.term || '%' THEN 2
					WHEN lower(coalesce(location.province, '')) LIKE '%' || term.term || '%' THEN 3
					ELSE 4
				END AS match_rank,
				greatest(
					similarity(lower(coalesce(location.subdistrict, '')), term.term),
					similarity(lower(coalesce(location.district, '')), term.term),
					similarity(lower(coalesce(location.city, '')), term.term),
					similarity(lower(coalesce(location.province, '')), term.term)
				) AS match_score
			FROM input_terms term
			JOIN locations location
			  ON location.active
			 AND location.level = 'subdistrict'
			 AND location.search_text LIKE '%' || term.term || '%'
			JOIN provider_location_mappings mapping
			  ON mapping.location_id = location.id
			 AND mapping.provider_code = $1
			 AND mapping.granularity = 'subdistrict'
			 AND mapping.active
		),
		postal_candidate_terms AS (
			SELECT
				mapping.id AS mapping_id,
				term.term,
				0 AS match_rank,
				1::real AS match_score
			FROM input_terms term
			JOIN postal_codes postal
			  ON postal.code = term.term
			 AND postal.active
			JOIN location_postal_codes search_link
			  ON search_link.postal_code_id = postal.id
			 AND search_link.active
			JOIN provider_location_mappings mapping
			  ON mapping.location_id = search_link.location_id
			 AND mapping.provider_code = $1
			 AND mapping.granularity = 'subdistrict'
			 AND mapping.active
		),
		candidate_terms AS (
			SELECT * FROM location_candidate_terms
			UNION ALL
			SELECT * FROM postal_candidate_terms
		),
		matched_mappings AS (
			SELECT
				mapping_id AS id,
				count(DISTINCT term)::int AS matched_terms,
				min(match_rank) AS match_rank,
				max(match_score) AS match_score
			FROM candidate_terms
			GROUP BY mapping_id
		),
		ranked_mappings AS (
			SELECT *
			FROM matched_mappings
			ORDER BY
				matched_terms DESC,
				match_rank,
				match_score DESC,
				id
			LIMIT $3 OFFSET $4
		)
	`+legacyRegionSelect+`
		JOIN ranked_mappings matched ON matched.id = mapping.id
		WHERE mapping.provider_code = $1
		  AND mapping.granularity = 'subdistrict'
		  AND mapping.active
		ORDER BY
			matched.matched_terms DESC,
			matched.match_rank,
			matched.match_score DESC,
			lower(coalesce(mapping.provider_location_name, '')),
			mapping.provider_location_id
	`, providerCode, terms, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("search legacy locations: %w", err)
	}
	defer rows.Close()

	result := make([]LegacyRegion, 0)
	for rows.Next() {
		region, scanErr := scanLegacyRegion(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan legacy search location: %w", scanErr)
		}
		result = append(result, region)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate legacy search locations: %w", err)
	}
	rows.Close()
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit legacy search: %w", err)
	}
	return result, nil
}

const legacyRegionSelect = `
	SELECT
		mapping.provider_location_id,
		location.public_id,
		coalesce(
			nullif(mapping.provider_location_name, ''),
			nullif(location.subdistrict, ''),
			nullif(location.district, ''),
			nullif(location.city, ''),
			nullif(location.province, ''),
			''
		),
		coalesce(
			postal.codes[1],
			CASE
				WHEN location.postal_code ~ '^[0-9]{5}$'
				 AND location.postal_code <> '00000'
				THEN location.postal_code
			END,
			''
		),
		coalesce(
			CASE mapping.granularity
				WHEN 'province' THEN mapping.provider_location_id
				WHEN 'city' THEN parent_mapping.provider_location_id
				WHEN 'district' THEN grandparent_mapping.provider_location_id
				WHEN 'subdistrict' THEN great_grandparent_mapping.provider_location_id
			END,
			''
		),
		coalesce(
			CASE mapping.granularity
				WHEN 'province' THEN mapping.provider_location_name
				WHEN 'city' THEN parent_mapping.provider_location_name
				WHEN 'district' THEN grandparent_mapping.provider_location_name
				WHEN 'subdistrict' THEN great_grandparent_mapping.provider_location_name
			END,
			location.province,
			''
		),
		coalesce(
			CASE mapping.granularity
				WHEN 'city' THEN mapping.provider_location_id
				WHEN 'district' THEN parent_mapping.provider_location_id
				WHEN 'subdistrict' THEN grandparent_mapping.provider_location_id
			END,
			''
		),
		coalesce(
			CASE mapping.granularity
				WHEN 'city' THEN mapping.provider_location_name
				WHEN 'district' THEN parent_mapping.provider_location_name
				WHEN 'subdistrict' THEN grandparent_mapping.provider_location_name
			END,
			location.city,
			''
		),
		coalesce(
			CASE mapping.granularity
				WHEN 'district' THEN mapping.provider_location_id
				WHEN 'subdistrict' THEN parent_mapping.provider_location_id
			END,
			''
		),
		coalesce(
			CASE mapping.granularity
				WHEN 'district' THEN mapping.provider_location_name
				WHEN 'subdistrict' THEN parent_mapping.provider_location_name
			END,
			location.district,
			''
		)
	FROM provider_location_mappings mapping
	JOIN locations location
	  ON location.id = mapping.location_id
	 AND location.active
	LEFT JOIN locations parent
	  ON parent.id = location.parent_id
	 AND parent.active
	LEFT JOIN provider_location_mappings parent_mapping
	  ON parent_mapping.location_id = parent.id
	 AND parent_mapping.provider_code = mapping.provider_code
	 AND parent_mapping.granularity = CASE mapping.granularity
		WHEN 'city' THEN 'province'
		WHEN 'district' THEN 'city'
		WHEN 'subdistrict' THEN 'district'
		ELSE ''
	 END
	 AND parent_mapping.active
	LEFT JOIN locations grandparent
	  ON grandparent.id = parent.parent_id
	 AND grandparent.active
	LEFT JOIN provider_location_mappings grandparent_mapping
	  ON grandparent_mapping.location_id = grandparent.id
	 AND grandparent_mapping.provider_code = mapping.provider_code
	 AND grandparent_mapping.granularity = CASE mapping.granularity
		WHEN 'district' THEN 'province'
		WHEN 'subdistrict' THEN 'city'
		ELSE ''
	 END
	 AND grandparent_mapping.active
	LEFT JOIN locations great_grandparent
	  ON great_grandparent.id = grandparent.parent_id
	 AND great_grandparent.active
	LEFT JOIN provider_location_mappings great_grandparent_mapping
	  ON great_grandparent_mapping.location_id = great_grandparent.id
	 AND great_grandparent_mapping.provider_code = mapping.provider_code
	 AND great_grandparent_mapping.granularity = CASE mapping.granularity
		WHEN 'subdistrict' THEN 'province'
		ELSE ''
	 END
	 AND great_grandparent_mapping.active
	LEFT JOIN LATERAL (
		SELECT array_agg(DISTINCT code.code ORDER BY code.code) AS codes
		FROM location_postal_codes link
		JOIN postal_codes code ON code.id = link.postal_code_id
		WHERE link.location_id = location.id
		  AND link.active
		  AND code.active
	) postal ON true
`

func (r *PostgresRepository) ListLegacyHierarchy(
	ctx context.Context,
	providerCode string,
	level string,
	parentProviderLocationID string,
) ([]LegacyRegion, error) {
	providerCode, level, parentProviderLocationID, err := normalizeLegacyLookup(
		providerCode,
		level,
		parentProviderLocationID,
		false,
	)
	if err != nil {
		return nil, err
	}

	rows, err := r.pool.Query(ctx, legacyRegionSelect+`
		WHERE mapping.provider_code = $1
		  AND mapping.granularity = $2
		  AND mapping.active
		  AND (
			($2 = 'province' AND location.parent_id IS NULL)
			OR
			($2 <> 'province' AND parent_mapping.provider_location_id = $3)
		  )
		ORDER BY
			CASE
				WHEN mapping.provider_location_id ~ '^[0-9]+$' THEN 0
				ELSE 1
			END,
			CASE
				WHEN mapping.provider_location_id ~ '^[0-9]+$'
				THEN mapping.provider_location_id::numeric
			END,
			lower(coalesce(mapping.provider_location_name, '')),
			mapping.provider_location_id
	`, providerCode, level, parentProviderLocationID)
	if err != nil {
		return nil, fmt.Errorf("list legacy %s locations: %w", level, err)
	}
	defer rows.Close()

	result := make([]LegacyRegion, 0)
	for rows.Next() {
		region, scanErr := scanLegacyRegion(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan legacy %s location: %w", level, scanErr)
		}
		result = append(result, region)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate legacy %s locations: %w", level, err)
	}
	return result, nil
}

func (r *PostgresRepository) FindLegacyRegion(
	ctx context.Context,
	providerCode string,
	level string,
	providerLocationID string,
) (LegacyRegion, error) {
	providerCode, level, providerLocationID, err := normalizeLegacyLookup(
		providerCode,
		level,
		providerLocationID,
		true,
	)
	if err != nil {
		return LegacyRegion{}, err
	}

	region, err := scanLegacyRegion(r.pool.QueryRow(ctx, legacyRegionSelect+`
		WHERE mapping.provider_code = $1
		  AND mapping.granularity = $2
		  AND mapping.provider_location_id = $3
		  AND mapping.active
		LIMIT 1
	`, providerCode, level, providerLocationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return LegacyRegion{}, ErrLocationNotFound
	}
	if err != nil {
		return LegacyRegion{}, fmt.Errorf("find legacy %s location: %w", level, err)
	}
	return region, nil
}

func (r *PostgresRepository) ResolveLegacyPublicID(
	ctx context.Context,
	providerCode string,
	level string,
	providerLocationID string,
) (string, error) {
	providerCode, level, providerLocationID, err := normalizeLegacyLookup(
		providerCode,
		level,
		providerLocationID,
		true,
	)
	if err != nil {
		return "", err
	}

	var publicID string
	err = r.pool.QueryRow(ctx, `
		SELECT location.public_id
		FROM provider_location_mappings mapping
		JOIN locations location
		  ON location.id = mapping.location_id
		 AND location.active
		WHERE mapping.provider_code = $1
		  AND mapping.granularity = $2
		  AND mapping.provider_location_id = $3
		  AND mapping.active
		LIMIT 1
	`, providerCode, level, providerLocationID).Scan(&publicID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrLocationNotFound
	}
	if err != nil {
		return "", fmt.Errorf("resolve legacy %s location: %w", level, err)
	}
	return publicID, nil
}

type legacyRegionScanner interface {
	Scan(dest ...any) error
}

func scanLegacyRegion(row legacyRegionScanner) (LegacyRegion, error) {
	var region LegacyRegion
	err := row.Scan(
		&region.ID,
		&region.CanonicalPublicID,
		&region.Name,
		&region.PostalCode,
		&region.ProvinceID,
		&region.ProvinceName,
		&region.CityID,
		&region.CityName,
		&region.DistrictID,
		&region.DistrictName,
	)
	return region, err
}

func normalizeLegacyLookup(
	providerCode string,
	level string,
	identifier string,
	requireIdentifier bool,
) (string, string, string, error) {
	providerCode = strings.ToLower(strings.TrimSpace(providerCode))
	level = strings.ToLower(strings.TrimSpace(level))
	identifier = strings.TrimSpace(identifier)
	if providerCode == "" {
		return "", "", "", errors.New("provider code is required")
	}
	switch level {
	case "province":
		if !requireIdentifier {
			identifier = ""
		}
	case "city", "district", "subdistrict":
	default:
		return "", "", "", fmt.Errorf("unsupported legacy hierarchy level %q", level)
	}
	if requireIdentifier && identifier == "" {
		return "", "", "", ErrLocationNotFound
	}
	if !requireIdentifier && level != "province" && identifier == "" {
		return "", "", "", ErrLocationNotFound
	}
	return providerCode, level, identifier, nil
}
