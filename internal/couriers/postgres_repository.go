package couriers

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) List(ctx context.Context) ([]Courier, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			c.code,
			c.name,
			c.provider_code,
			c.supports_domestic_cost,
			c.supports_international_cost,
			c.supports_tracking,
			coalesce(c.catalog_source, ''),
			coalesce(to_char(c.catalog_verified_at, 'YYYY-MM-DD'), ''),
			cs.code,
			cs.name,
			cs.service_group,
			cs.service_type,
			CASE
				WHEN cs.id IS NULL THEN NULL
				WHEN EXISTS (
					SELECT 1
					FROM rate_cards rc
					WHERE rc.courier_service_id = cs.id
					  AND rc.effective_from <= now()
					  AND (rc.effective_until IS NULL OR rc.effective_until > now())
				) THEN 'local_rate_card'
				ELSE 'provider_quote'
			END
		FROM couriers c
		LEFT JOIN courier_services cs
		  ON cs.courier_id = c.id
		 AND cs.active
		WHERE c.active
		ORDER BY c.name, cs.code
	`)
	if err != nil {
		return nil, fmt.Errorf("list couriers: %w", err)
	}
	defer rows.Close()

	courierByCode := make(map[string]int)
	result := make([]Courier, 0)
	for rows.Next() {
		var courier Courier
		var serviceCode, serviceName, serviceGroup, serviceType, calculationMode *string
		if err := rows.Scan(
			&courier.Code,
			&courier.Name,
			&courier.ProviderCode,
			&courier.SupportsDomesticCost,
			&courier.SupportsInternationalCost,
			&courier.SupportsTracking,
			&courier.CatalogSource,
			&courier.CatalogVerifiedAt,
			&serviceCode,
			&serviceName,
			&serviceGroup,
			&serviceType,
			&calculationMode,
		); err != nil {
			return nil, fmt.Errorf("scan courier: %w", err)
		}
		index, exists := courierByCode[courier.Code]
		if !exists {
			index = len(result)
			courierByCode[courier.Code] = index
			courier.Services = make([]Service, 0, 2)
			result = append(result, Courier{
				Code:                      courier.Code,
				Name:                      courier.Name,
				ProviderCode:              courier.ProviderCode,
				SupportsDomesticCost:      courier.SupportsDomesticCost,
				SupportsInternationalCost: courier.SupportsInternationalCost,
				SupportsTracking:          courier.SupportsTracking,
				CatalogSource:             courier.CatalogSource,
				CatalogVerifiedAt:         courier.CatalogVerifiedAt,
				Services:                  courier.Services,
			})
		}
		if serviceCode != nil {
			result[index].Services = append(result[index].Services, Service{
				Code:            *serviceCode,
				Name:            *serviceName,
				Group:           *serviceGroup,
				ServiceType:     *serviceType,
				CalculationMode: *calculationMode,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate couriers: %w", err)
	}
	return result, nil
}
