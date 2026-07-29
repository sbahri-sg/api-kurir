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
	escaped := escapeLike(strings.ToLower(strings.TrimSpace(search)))
	rows, err := r.pool.Query(ctx, `
		WITH candidates AS (
			SELECT
				location.id,
				CASE
					WHEN location.search_text LIKE $1 || '%' ESCAPE '\' THEN 1
					ELSE 2
				END AS match_rank,
				similarity(location.search_text, $1) AS match_score
			FROM locations location
			WHERE location.active
			  AND location.level = 'subdistrict'
			  AND location.official_region_code IS NOT NULL
			  AND location.search_text LIKE '%' || $1 || '%' ESCAPE '\'

			UNION ALL

			SELECT
				location.id,
				CASE WHEN postal.code = $1 THEN 0 ELSE 1 END AS match_rank,
				1::real AS match_score
			FROM postal_codes postal
			JOIN location_postal_codes link
			  ON link.postal_code_id = postal.id
			 AND link.active
			JOIN locations location
			 ON location.id = link.location_id
			 AND location.active
			 AND location.level = 'subdistrict'
			 AND location.official_region_code IS NOT NULL
			WHERE postal.active
			  AND postal.code LIKE '%' || $1 || '%' ESCAPE '\'
		),
		ranked AS (
			SELECT
				id,
				min(match_rank) AS match_rank,
				max(match_score) AS match_score
			FROM candidates
			GROUP BY id
			ORDER BY
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
			ranked.match_rank,
			ranked.match_score DESC,
			location.city,
			location.district,
			location.subdistrict
		`, escaped, limit, offset)
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
