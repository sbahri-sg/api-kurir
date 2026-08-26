package admin

import (
	"context"
	"errors"
	"fmt"

	"github.com/emisell/api-kurir/internal/providercredentials"
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
		provider.credential_type,
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
	requiresCredential := input.CredentialType != providercredentials.CredentialTypeNone
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ShippingProvider{}, fmt.Errorf("begin create shipping provider: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `
		INSERT INTO shipping_integration_providers (
			code, name, logo_url, description, built_in,
			integration_type, distribution_type,
			requires_credential, credential_type, available, display_order
		)
		VALUES ($1, $2, $3, $4, false, $5, $6, $7, $8, false, $9)
	`, input.Code, input.Name, input.Logo, input.Description,
		input.IntegrationType, input.DistributionType,
		requiresCredential, input.CredentialType,
		input.DisplayOrder)
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
			"requires_credential": requiresCredential,
			"credential_type":     input.CredentialType,
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
	var previousCredentialType string
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
		), provider.integration_type, provider.distribution_type, provider.credential_type, provider.built_in,
		   provider.active_release_id::text
		FROM shipping_integration_providers provider
		WHERE provider.code = $1
		FOR UPDATE OF provider
	`, code).Scan(
		&previous,
		&previousIntegrationType,
		&previousDistributionType,
		&previousCredentialType,
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
	var credentialCount int64
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM provider_credentials
		WHERE provider_code = $1
	`, code).Scan(&credentialCount); err != nil {
		return ShippingProvider{}, fmt.Errorf("count provider credentials: %w", err)
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
	if input.CredentialType == "" {
		input.CredentialType = previousCredentialType
	}
	if input.IntegrationType == "built_in" {
		input.CredentialType = providercredentials.CredentialTypeNone
	} else if input.CredentialType == providercredentials.CredentialTypeNone {
		if input.IntegrationType == "managed_upstream" {
			input.CredentialType = providercredentials.CredentialTypeAPIKey
		}
	}
	if input.CredentialType != previousCredentialType &&
		(activeMerchantCount > 0 || credentialCount > 0) {
		return ShippingProvider{}, ErrResourceInUse
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
		    credential_type = $8,
		    available = $9,
		    display_order = $10,
		    updated_at = now()
		WHERE code = $1
	`, code, input.Name, input.Logo, input.Description, input.IntegrationType,
		input.DistributionType, input.CredentialType != providercredentials.CredentialTypeNone,
		input.CredentialType, input.Available, input.DisplayOrder)
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
				"credential_type":   input.CredentialType,
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

func (r *PostgresRepository) DeleteShippingProvider(
	ctx context.Context,
	code string,
	actorAlias string,
	requestID string,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete shipping provider: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var builtIn bool
	var activeReleaseID *string
	var metadata map[string]any
	err = tx.QueryRow(ctx, `
		SELECT built_in, active_release_id::text, jsonb_build_object(
			'code', code,
			'name', name,
			'integration_type', integration_type,
			'distribution_type', distribution_type,
			'available', available
		)
		FROM shipping_integration_providers
		WHERE code = $1
		FOR UPDATE
	`, code).Scan(&builtIn, &activeReleaseID, &metadata)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock shipping provider for delete: %w", err)
	}
	if builtIn || activeReleaseID != nil {
		return ErrConflict
	}

	var dependencyCount int64
	err = tx.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM tenant_active_shipping_providers WHERE provider_code = $1)
			+ (SELECT count(*) FROM provider_credentials WHERE provider_code = $1)
			+ (SELECT count(*) FROM tenant_shipping_preferences WHERE provider_code = $1)
			+ (SELECT count(*) FROM partner_integration_submissions
			   WHERE provider_code = $1 AND status IN ('published', 'suspended', 'superseded'))
			+ (SELECT count(*) FROM fulfillment_shipments WHERE provider_code = $1)
			+ (SELECT count(*) FROM tracking_shipments WHERE provider_code = $1)
			+ (SELECT count(*) FROM rate_snapshots WHERE provider_code = $1)
			+ (SELECT count(*) FROM provider_api_calls WHERE provider_code = $1)
			+ (SELECT count(*) FROM provider_quota_ledger WHERE provider_code = $1)
			+ (SELECT count(*) FROM provider_location_mappings WHERE provider_code = $1)
			+ (SELECT count(*) FROM location_postal_codes WHERE provider_code = $1)
			+ (SELECT count(*) FROM provider_location_sync_checkpoints WHERE provider_code = $1)
			+ (SELECT count(*) FROM courier_service_aliases WHERE provider_code = $1)
			+ (SELECT count(*) FROM couriers
			   WHERE provider_code = $1 OR rate_provider_code = $1 OR tracking_provider_code = $1)
	`, code).Scan(&dependencyCount)
	if err != nil {
		return fmt.Errorf("count shipping provider dependencies: %w", err)
	}
	if dependencyCount > 0 {
		return ErrResourceInUse
	}

	var purgedSubmissions int64
	var purgedAccessKeys int64
	var purgedExplorerCredentials int64
	if err := tx.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM partner_integration_submissions WHERE provider_code = $1),
			(SELECT count(*) FROM partner_access_keys WHERE provider_code = $1),
			(SELECT count(*) FROM partner_explorer_credentials WHERE provider_code = $1)
	`, code).Scan(&purgedSubmissions, &purgedAccessKeys, &purgedExplorerCredentials); err != nil {
		return fmt.Errorf("count shipping provider onboarding artifacts: %w", err)
	}

	if err := insertAudit(
		ctx,
		tx,
		actorAlias,
		"delete",
		"shipping_provider",
		code,
		requestID,
		map[string]any{
			"deleted":                     metadata,
			"purged_submissions":          purgedSubmissions,
			"purged_access_keys":          purgedAccessKeys,
			"purged_explorer_credentials": purgedExplorerCredentials,
		},
	); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM partner_access_keys WHERE provider_code = $1
	`, code); err != nil {
		return fmt.Errorf("purge shipping provider access keys: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM partner_integration_submissions WHERE provider_code = $1
	`, code); err != nil {
		return fmt.Errorf("purge shipping provider submissions: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM shipping_integration_providers WHERE code = $1
	`, code); err != nil {
		if isForeignKeyViolation(err) {
			return ErrResourceInUse
		}
		return fmt.Errorf("delete shipping provider: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit delete shipping provider: %w", err)
	}
	return nil
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
		&item.CredentialType,
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
	item.CredentialFields = providercredentials.FieldsForCredentialType(item.CredentialType)
	return item, nil
}
