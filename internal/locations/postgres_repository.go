package locations

import (
	"context"
	"fmt"
	"strings"

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
	limit int,
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
		)
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
		FROM ranked
		JOIN locations location ON location.id = ranked.id
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
	`, escaped, limit)
	if err != nil {
		return nil, fmt.Errorf("search locations: %w", err)
	}
	defer rows.Close()

	result := make([]Location, 0)
	for rows.Next() {
		var location Location
		if err := rows.Scan(
			&location.PublicID,
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

func escapeLike(value string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`%`, `\%`,
		`_`, `\_`,
	)
	return replacer.Replace(value)
}
