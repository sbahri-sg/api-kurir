package merchantproviders

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Catalog(
	ctx context.Context,
	tenantID string,
) (Catalog, error) {
	return catalogWithQuerier(ctx, r.pool, tenantID)
}

func (r *PostgresRepository) Activate(
	ctx context.Context,
	tenantID string,
	providerCode string,
	input ChangeInput,
) (Catalog, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Catalog{}, fmt.Errorf("begin shipping provider activation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockTenant(ctx, tx, tenantID); err != nil {
		return Catalog{}, err
	}

	currentVersion, err := lockSelection(ctx, tx, tenantID)
	if err != nil {
		return Catalog{}, err
	}
	if input.ExpectedVersion != nil && currentVersion != *input.ExpectedVersion {
		return Catalog{}, ErrVersionConflict
	}

	var available, builtIn, requiresCredential bool
	err = tx.QueryRow(ctx, `
		SELECT available, built_in, requires_credential
		FROM shipping_integration_providers
		WHERE code = $1
	`, providerCode).Scan(&available, &builtIn, &requiresCredential)
	if errors.Is(err, pgx.ErrNoRows) {
		return Catalog{}, ErrProviderNotFound
	}
	if err != nil {
		return Catalog{}, fmt.Errorf("get shipping provider: %w", err)
	}
	if !available {
		return Catalog{}, ErrProviderUnavailable
	}

	var credentialID any
	if requiresCredential {
		if input.CredentialID == "" {
			return Catalog{}, ErrCredentialRequired
		}
		var valid bool
		err = tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM provider_credentials
				WHERE id = $1::uuid
				  AND tenant_id = $2
				  AND provider_code = $3
				  AND active
				  AND validation_status = 'valid'
			)
		`, input.CredentialID, tenantID, providerCode).Scan(&valid)
		if err != nil {
			return Catalog{}, fmt.Errorf("validate shipping provider credential: %w", err)
		}
		if !valid {
			return Catalog{}, ErrCredentialUnavailable
		}
		credentialID = input.CredentialID
	} else {
		if !builtIn || input.CredentialID != "" {
			return Catalog{}, ErrInvalidCredential
		}
		credentialID = nil
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO tenant_active_shipping_providers (
			tenant_id,
			provider_code,
			credential_id,
			updated_by
		)
		VALUES ($1, $2, $3::uuid, $4)
		ON CONFLICT (tenant_id) DO UPDATE
		SET provider_code = EXCLUDED.provider_code,
		    credential_id = EXCLUDED.credential_id,
		    version = tenant_active_shipping_providers.version + 1,
		    updated_by = EXCLUDED.updated_by,
		    updated_at = now()
	`, tenantID, providerCode, credentialID, input.UpdatedBy)
	if err != nil {
		return Catalog{}, fmt.Errorf("activate shipping provider: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Catalog{}, fmt.Errorf("commit shipping provider activation: %w", err)
	}
	return r.Catalog(ctx, tenantID)
}

func (r *PostgresRepository) Deactivate(
	ctx context.Context,
	tenantID string,
	providerCode string,
	input ChangeInput,
) (Catalog, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Catalog{}, fmt.Errorf("begin shipping provider deactivation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockTenant(ctx, tx, tenantID); err != nil {
		return Catalog{}, err
	}

	currentVersion, currentProvider, err := lockCurrentSelection(ctx, tx, tenantID)
	if err != nil {
		return Catalog{}, err
	}
	if input.ExpectedVersion != nil && currentVersion != *input.ExpectedVersion {
		return Catalog{}, ErrVersionConflict
	}
	if currentProvider != providerCode {
		if err := tx.Commit(ctx); err != nil {
			return Catalog{}, fmt.Errorf("commit idempotent provider deactivation: %w", err)
		}
		return r.Catalog(ctx, tenantID)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO tenant_active_shipping_providers (
			tenant_id,
			provider_code,
			credential_id,
			updated_by
		)
		VALUES ($1, $2, NULL, $3)
		ON CONFLICT (tenant_id) DO UPDATE
		SET provider_code = EXCLUDED.provider_code,
		    credential_id = NULL,
		    version = tenant_active_shipping_providers.version + 1,
		    updated_by = EXCLUDED.updated_by,
		    updated_at = now()
	`, tenantID, DefaultProviderCode, input.UpdatedBy)
	if err != nil {
		return Catalog{}, fmt.Errorf("fallback to default shipping provider: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Catalog{}, fmt.Errorf("commit shipping provider deactivation: %w", err)
	}
	return r.Catalog(ctx, tenantID)
}

type rowQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func catalogWithQuerier(
	ctx context.Context,
	querier rowQuerier,
	tenantID string,
) (Catalog, error) {
	result := Catalog{
		ActiveProviderCode: DefaultProviderCode,
		Providers:          make([]Provider, 0),
	}
	rows, err := querier.Query(ctx, `
		SELECT
			provider.code,
			provider.name,
			provider.built_in,
			provider.requires_credential,
			provider.available,
			provider.built_in OR EXISTS (
				SELECT 1
				FROM provider_credentials credential
				WHERE credential.tenant_id = $1
				  AND credential.provider_code = provider.code
				  AND credential.active
				  AND credential.validation_status = 'valid'
			) AS installed,
			COALESCE(selection.provider_code = provider.code, provider.code = $2),
			CASE
				WHEN selection.provider_code = provider.code
				THEN selection.credential_id::text
				ELSE NULL
			END,
			COALESCE(selection.provider_code, $2),
			selection.credential_id::text,
			COALESCE(selection.version, 0)
		FROM shipping_integration_providers provider
		LEFT JOIN tenant_active_shipping_providers selection
		  ON selection.tenant_id = $1
		ORDER BY provider.display_order, provider.name
	`, tenantID, DefaultProviderCode)
	if err != nil {
		return Catalog{}, fmt.Errorf("list shipping integration providers: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item Provider
		var activeProvider string
		var activeCredential *string
		var version int64
		if err := rows.Scan(
			&item.Code,
			&item.Name,
			&item.BuiltIn,
			&item.RequiresCredential,
			&item.Available,
			&item.Installed,
			&item.Active,
			&item.CredentialID,
			&activeProvider,
			&activeCredential,
			&version,
		); err != nil {
			return Catalog{}, fmt.Errorf("scan shipping integration provider: %w", err)
		}
		result.ActiveProviderCode = activeProvider
		result.ActiveCredentialID = activeCredential
		result.Version = version
		result.Providers = append(result.Providers, item)
	}
	if err := rows.Err(); err != nil {
		return Catalog{}, fmt.Errorf("iterate shipping integration providers: %w", err)
	}
	return result, nil
}

func lockSelection(ctx context.Context, tx pgx.Tx, tenantID string) (int64, error) {
	version, _, err := lockCurrentSelection(ctx, tx, tenantID)
	return version, err
}

func lockTenant(ctx context.Context, tx pgx.Tx, tenantID string) error {
	if _, err := tx.Exec(ctx, `
		SELECT pg_advisory_xact_lock(hashtextextended($1, 0))
	`, tenantID); err != nil {
		return fmt.Errorf("lock tenant shipping provider: %w", err)
	}
	return nil
}

func lockCurrentSelection(
	ctx context.Context,
	tx pgx.Tx,
	tenantID string,
) (int64, string, error) {
	var version int64
	var providerCode string
	err := tx.QueryRow(ctx, `
		SELECT version, provider_code
		FROM tenant_active_shipping_providers
		WHERE tenant_id = $1
		FOR UPDATE
	`, tenantID).Scan(&version, &providerCode)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, DefaultProviderCode, nil
	}
	if err != nil {
		return 0, "", fmt.Errorf("lock shipping provider selection: %w", err)
	}
	return version, providerCode, nil
}
