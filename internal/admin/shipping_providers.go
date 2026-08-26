package admin

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

const shippingProviderSelect = `
	SELECT
		provider.code,
		provider.name,
		provider.logo_url,
		provider.description,
		provider.built_in,
		provider.integration_type,
		provider.distribution_type,
		provider.requires_credential,
		provider.available,
		provider.display_order,
		provider.active_release_id::text,
		COALESCE(active_release.version, ''),
		COALESCE(active_release.status, ''),
		count(DISTINCT release.id) AS release_count,
		count(DISTINCT credential.tenant_id) FILTER (
			WHERE credential.tenant_id IS NOT NULL
			  AND credential.active
			  AND credential.validation_status = 'valid'
		) AS installed_merchant_count,
		count(DISTINCT selection.tenant_id) AS active_merchant_count,
		count(DISTINCT credential.id) AS credential_count,
		provider.created_at,
		provider.updated_at
	FROM shipping_integration_providers provider
	LEFT JOIN provider_credentials credential
	  ON credential.provider_code = provider.code
	LEFT JOIN tenant_active_shipping_providers selection
	  ON selection.provider_code = provider.code
	LEFT JOIN partner_integration_submissions active_release
	  ON active_release.id = provider.active_release_id
	LEFT JOIN partner_integration_submissions release
	  ON release.provider_code = provider.code
`

func (r *PostgresRepository) ListShippingProviders(
	ctx context.Context,
) ([]ShippingProvider, error) {
	rows, err := r.pool.Query(ctx, shippingProviderSelect+`
		GROUP BY provider.code, active_release.id
		ORDER BY provider.display_order, provider.name
	`)
	if err != nil {
		return nil, fmt.Errorf("list admin shipping providers: %w", err)
	}
	defer rows.Close()

	items := make([]ShippingProvider, 0)
	for rows.Next() {
		item, err := scanShippingProvider(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate admin shipping providers: %w", err)
	}
	return items, nil
}

func (r *PostgresRepository) CreateShippingProvider(
	ctx context.Context,
	input ShippingProviderCreateInput,
	actorAlias string,
	requestID string,
) (ShippingProvider, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ShippingProvider{}, fmt.Errorf("begin create shipping provider: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `
		INSERT INTO shipping_integration_providers (
			code, name, logo_url, description, built_in,
			integration_type, distribution_type,
			requires_credential, available, display_order
		)
		VALUES ($1, $2, $3, $4, false, $5, $6, $7, false, $8)
	`, input.Code, input.Name, input.Logo, input.Description,
		input.IntegrationType, input.DistributionType,
		input.IntegrationType == "managed_upstream", input.DisplayOrder)
	if err != nil {
		if isUniqueViolation(err) {
			return ShippingProvider{}, ErrConflict
		}
		return ShippingProvider{}, fmt.Errorf("create shipping provider: %w", err)
	}
	if err := insertAudit(
		ctx,
		tx,
		actorAlias,
		"create",
		"shipping_provider",
		input.Code,
		requestID,
		map[string]any{
			"name": input.Name, "available": false,
			"integration_type":    input.IntegrationType,
			"distribution_type":   input.DistributionType,
			"requires_credential": input.IntegrationType == "managed_upstream",
			"display_order":       input.DisplayOrder,
		},
	); err != nil {
		return ShippingProvider{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ShippingProvider{}, fmt.Errorf("commit create shipping provider: %w", err)
	}
	return r.getShippingProvider(ctx, input.Code)
}

func (r *PostgresRepository) UpdateShippingProvider(
	ctx context.Context,
	code string,
	input ShippingProviderUpdateInput,
	actorAlias string,
	requestID string,
) (ShippingProvider, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ShippingProvider{}, fmt.Errorf("begin update shipping provider: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var previous map[string]any
	var previousIntegrationType string
	var previousDistributionType string
	var builtIn bool
	var activeReleaseID *string
	err = tx.QueryRow(ctx, `
		SELECT jsonb_build_object(
			'name', provider.name,
			'logo', provider.logo_url,
			'description', provider.description,
			'integration_type', provider.integration_type,
			'distribution_type', provider.distribution_type,
			'available', provider.available,
			'display_order', provider.display_order
		), provider.integration_type, provider.distribution_type, provider.built_in,
		   provider.active_release_id::text
		FROM shipping_integration_providers provider
		WHERE provider.code = $1
		FOR UPDATE OF provider
	`, code).Scan(
		&previous,
		&previousIntegrationType,
		&previousDistributionType,
		&builtIn,
		&activeReleaseID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ShippingProvider{}, ErrNotFound
	}
	if err != nil {
		return ShippingProvider{}, fmt.Errorf("lock shipping provider: %w", err)
	}
	var activeMerchantCount int64
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM tenant_active_shipping_providers
		WHERE provider_code = $1
	`, code).Scan(&activeMerchantCount); err != nil {
		return ShippingProvider{}, fmt.Errorf("count active provider merchants: %w", err)
	}
	if !input.Available && activeMerchantCount > 0 {
		return ShippingProvider{}, ErrResourceInUse
	}
	if input.IntegrationType == "" {
		input.IntegrationType = previousIntegrationType
	}
	if input.DistributionType == "" {
		input.DistributionType = previousDistributionType
	}
	if builtIn && (input.IntegrationType != "built_in" || input.DistributionType != "built_in") {
		return ShippingProvider{}, ErrConflict
	}
	if !builtIn && (input.IntegrationType == "built_in" || input.DistributionType == "built_in") {
		return ShippingProvider{}, ErrConflict
	}
	if input.IntegrationType != previousIntegrationType {
		var releaseCount int64
		if err := tx.QueryRow(ctx, `
			SELECT count(*)
			FROM partner_integration_submissions
			WHERE provider_code = $1
		`, code).Scan(&releaseCount); err != nil {
			return ShippingProvider{}, fmt.Errorf("count provider releases: %w", err)
		}
		if activeMerchantCount > 0 || releaseCount > 0 {
			return ShippingProvider{}, ErrResourceInUse
		}
	}
	if input.Available && input.IntegrationType == "partner_hosted" && activeReleaseID == nil {
		return ShippingProvider{}, ErrConflict
	}

	_, err = tx.Exec(ctx, `
		UPDATE shipping_integration_providers
		SET name = $2,
		    logo_url = $3,
		    description = $4,
		    integration_type = $5,
		    distribution_type = $6,
		    requires_credential = $7,
		    available = $8,
		    display_order = $9,
		    updated_at = now()
		WHERE code = $1
	`, code, input.Name, input.Logo, input.Description, input.IntegrationType,
		input.DistributionType, input.IntegrationType == "managed_upstream",
		input.Available, input.DisplayOrder)
	if err != nil {
		return ShippingProvider{}, fmt.Errorf("update shipping provider: %w", err)
	}
	if err := insertAudit(
		ctx,
		tx,
		actorAlias,
		"update",
		"shipping_provider",
		code,
		requestID,
		map[string]any{
			"before": previous,
			"after": map[string]any{
				"name": input.Name, "logo": input.Logo,
				"description":       input.Description,
				"integration_type":  input.IntegrationType,
				"distribution_type": input.DistributionType,
				"available":         input.Available,
				"display_order":     input.DisplayOrder,
			},
		},
	); err != nil {
		return ShippingProvider{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ShippingProvider{}, fmt.Errorf("commit update shipping provider: %w", err)
	}
	return r.getShippingProvider(ctx, code)
}

func (r *PostgresRepository) getShippingProvider(
	ctx context.Context,
	code string,
) (ShippingProvider, error) {
	item, err := scanShippingProvider(r.pool.QueryRow(ctx, shippingProviderSelect+`
		WHERE provider.code = $1
		GROUP BY provider.code, active_release.id
	`, code))
	if errors.Is(err, pgx.ErrNoRows) {
		return ShippingProvider{}, ErrNotFound
	}
	return item, err
}

func scanShippingProvider(row rowScanner) (ShippingProvider, error) {
	var item ShippingProvider
	if err := row.Scan(
		&item.Code,
		&item.Name,
		&item.Logo,
		&item.Description,
		&item.BuiltIn,
		&item.IntegrationType,
		&item.DistributionType,
		&item.RequiresCredential,
		&item.Available,
		&item.DisplayOrder,
		&item.ActiveReleaseID,
		&item.ActiveReleaseVersion,
		&item.ActiveReleaseStatus,
		&item.ReleaseCount,
		&item.InstalledMerchantCount,
		&item.ActiveMerchantCount,
		&item.CredentialCount,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return ShippingProvider{}, fmt.Errorf("scan admin shipping provider: %w", err)
	}
	return item, nil
}
