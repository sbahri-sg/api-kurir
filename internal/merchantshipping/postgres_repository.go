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

func (r *PostgresRepository) Get(
	ctx context.Context,
	tenantID string,
) (Preference, error) {
	preference := Preference{
		Mode:          ModeCustom,
		EnabledGroups: []string{},
		Services:      []Selection{},
	}
	var updatedAt time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT selection_mode, enabled_groups, version, updated_at
		FROM tenant_shipping_preferences
		WHERE tenant_id = $1
	`, tenantID).Scan(
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
		ORDER BY courier.code, service.code
	`, tenantID)
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

	_, err = tx.Exec(ctx, `
		INSERT INTO tenant_shipping_preferences (
			tenant_id,
			selection_mode,
			enabled_groups,
			updated_by
		)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (tenant_id) DO UPDATE
		SET selection_mode = EXCLUDED.selection_mode,
		    enabled_groups = EXCLUDED.enabled_groups,
		    version = tenant_shipping_preferences.version + 1,
		    updated_by = EXCLUDED.updated_by,
		    updated_at = now()
	`, tenantID, input.Mode, input.EnabledGroups, input.UpdatedBy)
	if err != nil {
		return Preference{}, fmt.Errorf("upsert tenant shipping preference: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM tenant_shipping_service_selections WHERE tenant_id = $1
	`, tenantID); err != nil {
		return Preference{}, fmt.Errorf("clear tenant shipping services: %w", err)
	}
	for _, selection := range input.Services {
		tag, err := tx.Exec(ctx, `
			INSERT INTO tenant_shipping_service_selections (
				tenant_id,
				courier_service_id
			)
			SELECT $1, service.id
			FROM courier_services service
			JOIN couriers courier ON courier.id = service.courier_id
			WHERE courier.code = $2
			  AND service.code = $3
			  AND courier.active
			  AND service.active
		`, tenantID, selection.CourierCode, selection.ServiceCode)
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
