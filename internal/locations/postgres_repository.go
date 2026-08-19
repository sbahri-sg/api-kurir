package locations

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Search(
	ctx context.Context,
	search string,
	limit, offset int,
) ([]Location, error) {
	terms := normalizeLocationSearchTerms(search)
	if len(terms) == 0 {
		return []Location{}, nil
	}
	rows, err := r.pool.Query(ctx, `
		WITH input_terms AS (
			SELECT DISTINCT term
			FROM unnest($1::text[]) AS term
		),
		location_matches AS (
			SELECT
				location.id,
				count(DISTINCT term.term)::int AS matched_terms,
				min(
					CASE
						WHEN lower(coalesce(location.subdistrict, '')) = term.term THEN 0
						WHEN lower(coalesce(location.district, '')) = term.term THEN 1
						WHEN lower(coalesce(location.city, '')) LIKE '%' || term.term || '%' THEN 2
						WHEN lower(coalesce(location.province, '')) LIKE '%' || term.term || '%' THEN 3
						ELSE 4
					END
				) AS match_rank,
				max(
					greatest(
						similarity(lower(coalesce(location.subdistrict, '')), term.term),
						similarity(lower(coalesce(location.district, '')), term.term),
						similarity(lower(coalesce(location.city, '')), term.term),
						similarity(lower(coalesce(location.province, '')), term.term)
					)
				) AS match_score
			FROM locations location
			JOIN input_terms term
			  ON location.search_text LIKE '%' || term.term || '%'
			WHERE location.active
			  AND location.level = 'subdistrict'
			  AND location.official_region_code IS NOT NULL
			GROUP BY location.id
		),
		postal_matches AS (
			SELECT
				location.id,
				count(DISTINCT term.term)::int AS matched_terms,
				0 AS match_rank,
				1::real AS match_score
			FROM input_terms term
			JOIN postal_codes postal
			  ON postal.active
			 AND postal.code LIKE '%' || term.term || '%'
			JOIN location_postal_codes link
			  ON link.postal_code_id = postal.id
			 AND link.active
			JOIN locations location
			  ON location.id = link.location_id
			 AND location.active
			 AND location.level = 'subdistrict'
			 AND location.official_region_code IS NOT NULL
			GROUP BY location.id
		),
		candidates AS (
			SELECT id, matched_terms, match_rank, match_score
			FROM location_matches

			UNION ALL

			SELECT id, matched_terms, match_rank, match_score
			FROM postal_matches
		),
		ranked AS (
			SELECT
				id,
				max(matched_terms) AS matched_terms,
				min(match_rank) AS match_rank,
				max(match_score) AS match_score
			FROM candidates
			GROUP BY id
			ORDER BY
				max(matched_terms) DESC,
				min(match_rank),
				max(match_score) DESC,
				id
			LIMIT $2
			OFFSET $3
		)
		SELECT
			location.public_id,
			location.compatibility_id,
			coalesce(province.public_id, ''),
			coalesce(city.public_id, ''),
			coalesce(district.public_id, ''),
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
		FROM ranked
		JOIN locations location ON location.id = ranked.id
		LEFT JOIN locations district
		  ON district.id = location.parent_id
		 AND district.level = 'district'
		 AND district.active
		LEFT JOIN locations city
		  ON city.id = district.parent_id
		 AND city.level = 'city'
		 AND city.active
		LEFT JOIN locations province
		  ON province.id = city.parent_id
		 AND province.level = 'province'
		 AND province.active
		LEFT JOIN LATERAL (
			SELECT array_agg(DISTINCT code.code ORDER BY code.code) AS codes
			FROM location_postal_codes link
			JOIN postal_codes code ON code.id = link.postal_code_id
			WHERE link.location_id = location.id
			  AND link.active
			  AND code.active
		) postal ON true
		ORDER BY
			ranked.matched_terms DESC,
			ranked.match_rank,
			ranked.match_score DESC,
			location.city,
			location.district,
			location.subdistrict
		`, terms, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("search locations: %w", err)
	}
	defer rows.Close()

	result := make([]Location, 0)
	for rows.Next() {
		var location Location
		if err := rows.Scan(
			&location.PublicID,
			&location.CompatibilityID,
			&location.ProvinceID,
			&location.CityID,
			&location.DistrictID,
			&location.SubdistrictID,
			&location.Province,
			&location.City,
			&location.District,
			&location.Subdistrict,
			&location.PostalCode,
			&location.PostalCodes,
		); err != nil {
			return nil, fmt.Errorf("scan location: %w", err)
		}
		result = append(result, location)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate locations: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) ListHierarchy(
	ctx context.Context,
	level string,
	parentPublicID string,
) ([]HierarchyLocation, error) {
	level = strings.TrimSpace(strings.ToLower(level))
	switch level {
	case "province":
		parentPublicID = ""
	case "city", "district", "subdistrict":
		parentPublicID = strings.TrimSpace(parentPublicID)
	default:
		return nil, fmt.Errorf("unsupported location hierarchy level %q", level)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT
			location.public_id,
			location.compatibility_id,
			coalesce(
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
			)
		FROM locations location
		LEFT JOIN locations parent
		  ON parent.id = location.parent_id
		 AND parent.active
		LEFT JOIN LATERAL (
			SELECT array_agg(DISTINCT code.code ORDER BY code.code) AS codes
			FROM location_postal_codes link
			JOIN postal_codes code ON code.id = link.postal_code_id
			WHERE link.location_id = location.id
			  AND link.active
			  AND code.active
		) postal ON true
		WHERE location.active
		  AND location.official_region_code IS NOT NULL
		  AND location.level = $1
		  AND (
				($1 = 'province' AND location.parent_id IS NULL)
				OR
				(
					$1 <> 'province'
					AND (
						parent.public_id = $2
						OR parent.compatibility_id::text = $2
					)
				)
		  )
		ORDER BY 2, location.public_id
	`, level, parentPublicID)
	if err != nil {
		return nil, fmt.Errorf("list %s locations: %w", level, err)
	}
	defer rows.Close()

	result := make([]HierarchyLocation, 0)
	for rows.Next() {
		var location HierarchyLocation
		if err := rows.Scan(
			&location.PublicID,
			&location.CompatibilityID,
			&location.Name,
			&location.PostalCode,
		); err != nil {
			return nil, fmt.Errorf("scan %s location: %w", level, err)
		}
		result = append(result, location)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s locations: %w", level, err)
	}
	return result, nil
}

func (r *PostgresRepository) ResolvePublicID(
	ctx context.Context,
	identifier string,
	level string,
) (string, error) {
	identifier = strings.TrimSpace(identifier)
	level = strings.TrimSpace(strings.ToLower(level))
	if identifier == "" {
		return "", ErrLocationNotFound
	}
	switch level {
	case "province", "city", "district", "subdistrict":
	default:
		return "", fmt.Errorf("unsupported location hierarchy level %q", level)
	}

	var publicID string
	err := r.pool.QueryRow(ctx, `
		SELECT public_id
		FROM locations
		WHERE active
		  AND official_region_code IS NOT NULL
		  AND level = $2
		  AND (
				public_id = $1
				OR compatibility_id::text = $1
		  )
		LIMIT 1
	`, identifier, level).Scan(&publicID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrLocationNotFound
	}
	if err != nil {
		return "", fmt.Errorf("resolve %s location %q: %w", level, identifier, err)
	}
	return publicID, nil
}

func escapeLike(value string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`%`, `\%`,
		`_`, `\_`,
	)
	return replacer.Replace(value)
}
