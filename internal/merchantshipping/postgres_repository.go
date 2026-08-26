package merchantshipping

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) ActiveCatalog(
	ctx context.Context,
	tenantID string,
) (ProviderCatalog, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			active.provider_code,
			courier.code,
			service.code
		FROM tenant_active_shipping_providers active
		LEFT JOIN provider_courier_services provider_service
		  ON provider_service.provider_code = active.provider_code
		 AND provider_service.active
		LEFT JOIN courier_services service
		  ON service.id = provider_service.courier_service_id
		 AND service.active
		LEFT JOIN couriers courier
		  ON courier.id = service.courier_id
		 AND courier.active
		WHERE active.tenant_id = $1
		  AND active.provider_code IS NOT NULL
		ORDER BY courier.code, service.code
	`, tenantID)
	if err != nil {
		return ProviderCatalog{}, fmt.Errorf("list active provider service catalog: %w", err)
	}
	defer rows.Close()

	result := ProviderCatalog{Services: []Selection{}}
	for rows.Next() {
		var providerCode string
		var courierCode, serviceCode *string
		if err := rows.Scan(&providerCode, &courierCode, &serviceCode); err != nil {
			return ProviderCatalog{}, fmt.Errorf("scan active provider service catalog: %w", err)
		}
		result.ProviderCode = providerCode
		if courierCode != nil && serviceCode != nil {
			result.Services = append(result.Services, Selection{
				CourierCode: *courierCode,
				ServiceCode: *serviceCode,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return ProviderCatalog{}, fmt.Errorf("iterate active provider service catalog: %w", err)
	}
	if result.ProviderCode == "" {
		return ProviderCatalog{}, ErrShippingDisabled
	}
	return result, nil
}

func (r *PostgresRepository) Get(
	ctx context.Context,
	tenantID string,
) (Preference, error) {
	preference := Preference{
		Mode:          ModeCustom,
		EnabledGroups: []string{},
		Services:      []Selection{},
	}
	if err := r.pool.QueryRow(ctx, `
		SELECT provider_code
		FROM tenant_active_shipping_providers
		WHERE tenant_id = $1
		  AND provider_code IS NOT NULL
	`, tenantID).Scan(&preference.ProviderCode); errors.Is(err, pgx.ErrNoRows) {
		return preference, nil
	} else if err != nil {
		return Preference{}, fmt.Errorf("get active tenant shipping provider: %w", err)
	}
	var updatedAt time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT selection_mode, enabled_groups, version, updated_at
		FROM tenant_shipping_preferences
		WHERE tenant_id = $1
		  AND provider_code = $2
	`, tenantID, preference.ProviderCode).Scan(
		&preference.Mode,
		&preference.EnabledGroups,
		&preference.Version,
		&updatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return preference, nil
	}
	if err != nil {
		return Preference{}, fmt.Errorf("get tenant shipping preference: %w", err)
	}
	preference.Configured = true
	preference.UpdatedAt = &updatedAt

	rows, err := r.pool.Query(ctx, `
		SELECT courier.code, service.code
		FROM tenant_shipping_service_selections selection
		JOIN courier_services service ON service.id = selection.courier_service_id
		JOIN couriers courier ON courier.id = service.courier_id
		WHERE selection.tenant_id = $1
		  AND selection.provider_code = $2
		ORDER BY courier.code, service.code
	`, tenantID, preference.ProviderCode)
	if err != nil {
		return Preference{}, fmt.Errorf("list tenant shipping services: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var selection Selection
		if err := rows.Scan(&selection.CourierCode, &selection.ServiceCode); err != nil {
			return Preference{}, fmt.Errorf("scan tenant shipping service: %w", err)
		}
		preference.Services = append(preference.Services, selection)
	}
	if err := rows.Err(); err != nil {
		return Preference{}, fmt.Errorf("iterate tenant shipping services: %w", err)
	}
	return preference, nil
}

func (r *PostgresRepository) SelectedCourierCodes(
	ctx context.Context,
	tenantID string,
) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT courier.code
		FROM tenant_shipping_service_selections selection
		JOIN tenant_shipping_preferences preference
		  ON preference.tenant_id = selection.tenant_id
		 AND preference.provider_code = selection.provider_code
		JOIN courier_services service
		  ON service.id = selection.courier_service_id
		JOIN couriers courier
		  ON courier.id = service.courier_id
		JOIN tenant_active_shipping_providers active
		  ON active.tenant_id = selection.tenant_id
		 AND active.provider_code = selection.provider_code
		JOIN provider_courier_services provider_service
		  ON provider_service.provider_code = selection.provider_code
		 AND provider_service.courier_service_id = selection.courier_service_id
		 AND provider_service.active
		WHERE selection.tenant_id = $1
		  AND preference.selection_mode = 'custom'
		  AND courier.active
		  AND service.active
		  AND service.service_group = ANY($2::text[])
		ORDER BY courier.code
	`, tenantID, SupportedGroups)
	if err != nil {
		return nil, fmt.Errorf("list selected tenant shipping couriers: %w", err)
	}
	defer rows.Close()

	codes := make([]string, 0)
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, fmt.Errorf("scan selected tenant shipping courier: %w", err)
		}
		codes = append(codes, code)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate selected tenant shipping couriers: %w", err)
	}
	return codes, nil
}

func (r *PostgresRepository) Replace(
	ctx context.Context,
	tenantID string,
	input UpdateInput,
) (Preference, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Preference{}, fmt.Errorf("begin tenant shipping preference update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var providerCode string
	if err := tx.QueryRow(ctx, `
		SELECT provider_code
		FROM tenant_active_shipping_providers
		WHERE tenant_id = $1
		  AND provider_code IS NOT NULL
		FOR UPDATE
	`, tenantID).Scan(&providerCode); errors.Is(err, pgx.ErrNoRows) {
		return Preference{}, ErrShippingDisabled
	} else if err != nil {
		return Preference{}, fmt.Errorf("lock active tenant shipping provider: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO tenant_shipping_preferences (
			tenant_id,
			provider_code,
			selection_mode,
			enabled_groups,
			updated_by
		)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (tenant_id, provider_code) DO UPDATE
		SET selection_mode = EXCLUDED.selection_mode,
		    enabled_groups = EXCLUDED.enabled_groups,
		    version = tenant_shipping_preferences.version + 1,
		    updated_by = EXCLUDED.updated_by,
		    updated_at = now()
	`, tenantID, providerCode, input.Mode, input.EnabledGroups, input.UpdatedBy)
	if err != nil {
		return Preference{}, fmt.Errorf("upsert tenant shipping preference: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM tenant_shipping_service_selections
		WHERE tenant_id = $1
		  AND provider_code = $2
	`, tenantID, providerCode); err != nil {
		return Preference{}, fmt.Errorf("clear tenant shipping services: %w", err)
	}
	for _, selection := range input.Services {
		tag, err := tx.Exec(ctx, `
			INSERT INTO tenant_shipping_service_selections (
				tenant_id,
				provider_code,
				courier_service_id
			)
			SELECT $1, $2, service.id
			FROM courier_services service
			JOIN couriers courier ON courier.id = service.courier_id
			JOIN provider_courier_services provider_service
			  ON provider_service.provider_code = $2
			 AND provider_service.courier_service_id = service.id
			 AND provider_service.active
			WHERE courier.code = $3
			  AND service.code = $4
			  AND courier.active
			  AND service.active
		`, tenantID, providerCode, selection.CourierCode, selection.ServiceCode)
		if err != nil {
			return Preference{}, fmt.Errorf("insert tenant shipping service: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return Preference{}, ErrUnknownService
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Preference{}, fmt.Errorf("commit tenant shipping preference: %w", err)
	}
	return r.Get(ctx, tenantID)
}
