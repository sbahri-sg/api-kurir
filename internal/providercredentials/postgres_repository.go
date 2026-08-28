package providercredentials

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/emisell/api-kurir/internal/tenancy"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) CredentialType(
	ctx context.Context,
	providerCode string,
) (string, error) {
	var credentialType string
	err := r.pool.QueryRow(ctx, `
		SELECT credential_type
		FROM shipping_integration_providers
		WHERE code = $1
		  AND requires_credential
		  AND credential_type <> 'none'
	`, providerCode).Scan(&credentialType)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrUnsupportedProvider
	}
	if err != nil {
		return "", fmt.Errorf("get provider credential type: %w", err)
	}
	return credentialType, nil
}

func (r *PostgresRepository) CredentialDefinition(
	ctx context.Context,
	providerCode string,
) (string, []FieldDefinition, error) {
	var credentialType string
	var payload []byte
	err := r.pool.QueryRow(ctx, `
		SELECT credential_type, credential_schema
		FROM shipping_integration_providers
		WHERE code = $1
		  AND requires_credential
		  AND credential_type <> 'none'
	`, providerCode).Scan(&credentialType, &payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, ErrUnsupportedProvider
	}
	if err != nil {
		return "", nil, fmt.Errorf("get provider credential definition: %w", err)
	}
	fields := make([]FieldDefinition, 0)
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &fields); err != nil {
			return "", nil, fmt.Errorf("decode provider credential definition: %w", err)
		}
	}
	if len(fields) == 0 {
		fields = FieldsForCredentialType(credentialType)
	}
	return credentialType, fields, nil
}

func (r *PostgresRepository) CredentialEnvironment(
	ctx context.Context,
	providerCode string,
	capability string,
	executionEnvironment string,
) (string, error) {
	executionEnvironment = NormalizeEnvironment(executionEnvironment)
	capability = capabilityName(capability)
	var credentialEnvironment string
	err := r.pool.QueryRow(ctx, `
		SELECT policy ->> 'credential_environment'
		FROM shipping_integration_providers provider
		CROSS JOIN LATERAL jsonb_array_elements(
			provider.capability_environment_schema
		) policy
		WHERE provider.code = $1
		  AND policy ->> 'capability' = $2
		  AND policy ->> 'environment' = $3
		  AND policy ->> 'behavior' <> 'unavailable'
		LIMIT 1
	`, providerCode, capability, executionEnvironment).Scan(&credentialEnvironment)
	if errors.Is(err, pgx.ErrNoRows) {
		if executionEnvironment == EnvironmentLive {
			return EnvironmentLive, nil
		}
		return "", ErrEnvironmentUnavailable
	}
	if err != nil {
		return "", fmt.Errorf("resolve provider credential environment: %w", err)
	}
	credentialEnvironment = strings.ToLower(strings.TrimSpace(credentialEnvironment))
	if credentialEnvironment != EnvironmentLive && credentialEnvironment != EnvironmentSandbox {
		return "", ErrEnvironmentUnavailable
	}
	return credentialEnvironment, nil
}

func (r *PostgresRepository) List(ctx context.Context) ([]Credential, error) {
	return r.list(ctx, "", false)
}

func (r *PostgresRepository) ListForTenant(
	ctx context.Context,
	tenantID string,
) ([]Credential, error) {
	return r.list(ctx, tenantID, true)
}

func (r *PostgresRepository) list(
	ctx context.Context,
	tenantID string,
	filterTenant bool,
) ([]Credential, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			id::text,
			tenant_id,
			provider_code,
			environment_code,
			credential_alias,
			key_prefix || '••••' || key_last_four,
			daily_limit,
			active,
			validation_status,
			last_validated_at,
			last_selected_at,
			created_at,
			disabled_at
		FROM provider_credentials
		WHERE (NOT $2::boolean OR tenant_id = $1)
		ORDER BY active DESC, created_at DESC
	`, tenantID, filterTenant)
	if err != nil {
		return nil, fmt.Errorf("list provider credentials: %w", err)
	}
	defer rows.Close()

	items := make([]Credential, 0)
	for rows.Next() {
		item, err := scanCredential(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate provider credentials: %w", err)
	}
	return items, nil
}

func (r *PostgresRepository) Create(
	ctx context.Context,
	input CreateInput,
) (Credential, error) {
	input.Environment = NormalizeEnvironment(input.Environment)
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Credential{}, fmt.Errorf("begin provider credential create: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	wasSelected := false
	if input.TenantID != "" {
		if _, err := tx.Exec(ctx, `
			SELECT pg_advisory_xact_lock(
				hashtextextended('merchant-provider-credential:' || $1 || ':' || $2 || ':' || $3, 0)
			)
		`, input.TenantID, input.ProviderCode, input.Environment); err != nil {
			return Credential{}, fmt.Errorf("lock merchant provider credential: %w", err)
		}
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM tenant_active_shipping_providers selection
				JOIN provider_credentials credential
				  ON credential.id = selection.credential_id
				WHERE selection.tenant_id = $1
				  AND selection.provider_code = $2
				  AND credential.tenant_id = selection.tenant_id
				  AND credential.provider_code = selection.provider_code
				  AND credential.environment_code = $3
			)
		`, input.TenantID, input.ProviderCode, input.Environment).Scan(&wasSelected); err != nil {
			return Credential{}, fmt.Errorf("inspect active merchant provider: %w", err)
		}
	}

	var existingID string
	var existingActive bool
	err = tx.QueryRow(ctx, `
		SELECT id::text, active
		FROM provider_credentials
		WHERE tenant_id = $1
		  AND provider_code = $2
		  AND secret_fingerprint = $3
		  AND environment_code = $4
		FOR UPDATE
	`, input.TenantID, input.ProviderCode, input.SecretFingerprint, input.Environment).Scan(
		&existingID,
		&existingActive,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		existingID = ""
	} else if err != nil {
		return Credential{}, fmt.Errorf("find existing provider credential: %w", err)
	}

	var existingIDArg any
	if existingID != "" {
		existingIDArg = existingID
	}
	if input.TenantID != "" {
		if _, err := tx.Exec(ctx, `
			UPDATE provider_credentials
			SET active = false,
			    disabled_at = coalesce(disabled_at, now()),
			    updated_at = now()
			WHERE tenant_id = $1
			  AND provider_code = $2
			  AND environment_code = $3
			  AND active
			  AND ($4::uuid IS NULL OR id <> $4::uuid)
		`, input.TenantID, input.ProviderCode, input.Environment, existingIDArg); err != nil {
			return Credential{}, fmt.Errorf("replace merchant provider credential: %w", err)
		}
	}

	recordID := input.ID
	auditAction := "create"
	if existingID != "" {
		recordID = existingID
		if existingActive {
			auditAction = "refresh"
		} else {
			auditAction = "reactivate"
		}
		_, err = tx.Exec(ctx, `
			UPDATE provider_credentials
			SET credential_alias = $2,
			    environment_code = $3,
			    secret_ciphertext = $4,
			    key_prefix = $5,
			    key_last_four = $6,
			    daily_limit = $7,
			    active = true,
			    validation_status = 'valid',
			    last_validated_at = now(),
			    disabled_by = NULL,
			    disabled_at = NULL,
			    updated_at = now()
			WHERE id = $1::uuid
		`,
			recordID,
			input.CredentialAlias,
			input.Environment,
			input.SecretCiphertext,
			input.KeyPrefix,
			input.KeyLastFour,
			input.DailyLimit,
		)
		if err != nil {
			return Credential{}, fmt.Errorf("reactivate provider credential: %w", err)
		}
	} else {
		_, err = tx.Exec(ctx, `
			INSERT INTO provider_credentials (
				id,
				tenant_id,
				provider_code,
				environment_code,
				credential_alias,
				secret_ciphertext,
				secret_fingerprint,
				key_prefix,
				key_last_four,
				daily_limit,
				created_by
			)
			VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		`,
			recordID,
			input.TenantID,
			input.ProviderCode,
			input.Environment,
			input.CredentialAlias,
			input.SecretCiphertext,
			input.SecretFingerprint,
			input.KeyPrefix,
			input.KeyLastFour,
			input.DailyLimit,
			input.CreatedBy,
		)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return Credential{}, ErrDuplicate
			}
			return Credential{}, fmt.Errorf("insert provider credential: %w", err)
		}
	}
	if wasSelected {
		if _, err := tx.Exec(ctx, `
			UPDATE tenant_active_shipping_providers
			SET provider_code = $2,
			    credential_id = $3::uuid,
			    version = version + 1,
			    updated_by = $4,
			    updated_at = now()
			WHERE tenant_id = $1
			  AND (
				provider_code IS DISTINCT FROM $2
				OR credential_id IS DISTINCT FROM $3::uuid
			  )
		`, input.TenantID, input.ProviderCode, recordID, input.CreatedBy); err != nil {
			return Credential{}, fmt.Errorf("select replacement merchant credential: %w", err)
		}
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO provider_quota_ledger (
			tenant_id,
			provider_code,
			credential_alias,
			quota_date,
			daily_limit,
			used_count,
			reset_at
		)
		VALUES (
			$1,
			$2,
			$3,
			(now() AT TIME ZONE 'Asia/Jakarta')::date,
			$4,
			$5,
			(
				((now() AT TIME ZONE 'Asia/Jakarta')::date + 1)::timestamp
				AT TIME ZONE 'Asia/Jakarta'
			)
		)
		ON CONFLICT (tenant_id, provider_code, credential_alias, quota_date) DO UPDATE
		SET daily_limit = EXCLUDED.daily_limit,
		    used_count = LEAST(
				EXCLUDED.daily_limit,
				provider_quota_ledger.used_count + EXCLUDED.used_count
			),
		    reset_at = EXCLUDED.reset_at,
		    updated_at = now()
	`,
		input.TenantID,
		input.ProviderCode,
		input.CredentialAlias,
		input.DailyLimit,
		input.ValidationQuotaCost,
	)
	if err != nil {
		return Credential{}, fmt.Errorf("record provider credential validation hit: %w", err)
	}
	if err := insertCredentialAudit(
		ctx,
		tx,
		input.CreatedBy,
		auditAction,
		recordID,
		input.RequestID,
		map[string]any{
			"tenant_id":        input.TenantID,
			"provider_code":    input.ProviderCode,
			"environment":      input.Environment,
			"credential_alias": input.CredentialAlias,
			"key_prefix":       input.KeyPrefix,
		},
	); err != nil {
		return Credential{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Credential{}, fmt.Errorf("commit provider credential create: %w", err)
	}
	return r.get(ctx, recordID)
}

func (r *PostgresRepository) Disable(
	ctx context.Context,
	id, actor, requestID string,
) error {
	return r.disable(ctx, "", id, actor, requestID, false)
}

func (r *PostgresRepository) DisableForTenantProvider(
	ctx context.Context,
	tenantID, providerCode, actor, requestID string,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin merchant provider credential disable: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		SELECT pg_advisory_xact_lock(
			hashtextextended('merchant-provider-credential:' || $1 || ':' || $2, 0)
		)
	`, tenantID, providerCode); err != nil {
		return fmt.Errorf("lock merchant provider credential: %w", err)
	}

	rows, err := tx.Query(ctx, `
		SELECT id::text
		FROM provider_credentials
		WHERE tenant_id = $1
		  AND provider_code = $2
		  AND active
		FOR UPDATE
	`, tenantID, providerCode)
	if err != nil {
		return fmt.Errorf("find merchant provider credentials: %w", err)
	}
	ids := make([]string, 0, 2)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("scan merchant provider credential: %w", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) == 0 {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, `
		UPDATE provider_credentials
		SET active = false,
		    disabled_by = $2,
		    disabled_at = now(),
		    updated_at = now()
		WHERE id::text = ANY($1::text[])
	`, ids, actor); err != nil {
		return fmt.Errorf("disable merchant provider credential: %w", err)
	}
	for _, id := range ids {
		if err := insertCredentialAudit(
			ctx,
			tx,
			actor,
			"disable",
			id,
			requestID,
			map[string]any{
				"tenant_id":     tenantID,
				"provider_code": providerCode,
			},
		); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit merchant provider credential disable: %w", err)
	}
	return nil
}

func (r *PostgresRepository) ActiveStoredForTenantProvider(
	ctx context.Context,
	tenantID, providerCode, environment string,
) (StoredCredential, error) {
	environment = NormalizeEnvironment(environment)
	var item StoredCredential
	err := r.pool.QueryRow(ctx, `
		SELECT
			id::text,
			tenant_id,
			provider_code,
			environment_code,
			credential_alias,
			key_prefix || '••••' || key_last_four,
			daily_limit,
			active,
			validation_status,
			last_validated_at,
			last_selected_at,
			created_at,
			disabled_at,
			secret_ciphertext,
			secret_fingerprint
		FROM provider_credentials
		WHERE tenant_id = $1
		  AND provider_code = $2
		  AND environment_code = $3
		  AND active
		ORDER BY created_at DESC
		LIMIT 1
	`, tenantID, providerCode, environment).Scan(
		&item.ID,
		&item.TenantID,
		&item.ProviderCode,
		&item.Environment,
		&item.CredentialAlias,
		&item.DisplayKey,
		&item.DailyLimit,
		&item.Active,
		&item.ValidationStatus,
		&item.LastValidatedAt,
		&item.LastSelectedAt,
		&item.CreatedAt,
		&item.DisabledAt,
		&item.SecretCiphertext,
		&item.Fingerprint,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return StoredCredential{}, ErrNotFound
	}
	if err != nil {
		return StoredCredential{}, fmt.Errorf("get active merchant provider credential: %w", err)
	}
	return item, nil
}

func (r *PostgresRepository) disable(
	ctx context.Context,
	tenantID, id, actor, requestID string,
	filterTenant bool,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin provider credential disable: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		UPDATE provider_credentials
		SET active = false,
		    disabled_by = $2,
		    disabled_at = now(),
		    updated_at = now()
		WHERE id = $1::uuid
		  AND (NOT $3::boolean OR tenant_id = $4)
		  AND active
	`, id, actor, filterTenant, tenantID)
	if err != nil {
		return fmt.Errorf("disable provider credential: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if err := insertCredentialAudit(
		ctx,
		tx,
		actor,
		"disable",
		id,
		requestID,
		nil,
	); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit provider credential disable: %w", err)
	}
	return nil
}

func (r *PostgresRepository) ResolveActive(
	ctx context.Context,
	providerCode string,
) (StoredCredential, error) {
	tenantID := tenancy.TenantID(ctx)
	providerCredentialID := tenancy.ProviderCredentialID(ctx)
	environment := ExecutionEnvironment(ctx)
	var item StoredCredential
	err := r.pool.QueryRow(ctx, `
		WITH candidate AS (
			SELECT credential.id
			FROM provider_credentials credential
			LEFT JOIN provider_quota_ledger quota
			  ON quota.tenant_id = credential.tenant_id
			 AND quota.provider_code = credential.provider_code
			 AND quota.credential_alias = credential.credential_alias
			 AND quota.quota_date = (now() AT TIME ZONE 'Asia/Jakarta')::date
			WHERE credential.provider_code = $1
			  AND credential.tenant_id = $2
			  AND credential.environment_code = $3
			  AND ($4 = '' OR $3 <> 'live' OR credential.id::text = $4)
			  AND (
				$2 = '' OR $3 <> 'live' OR EXISTS (
					SELECT 1
					FROM tenant_active_shipping_providers selection
					WHERE selection.tenant_id = credential.tenant_id
					  AND selection.provider_code = credential.provider_code
					  AND selection.credential_id = credential.id
				)
			  )
			  AND credential.active
			  AND credential.validation_status = 'valid'
			  AND coalesce(quota.used_count + quota.reserved_count, 0)
			      < credential.daily_limit
			ORDER BY
				coalesce(quota.used_count + quota.reserved_count, 0)::numeric
					/ credential.daily_limit::numeric,
				credential.last_selected_at NULLS FIRST,
				credential.created_at
			LIMIT 1
			FOR UPDATE OF credential SKIP LOCKED
		)
		UPDATE provider_credentials credential
		SET last_selected_at = now(),
		    updated_at = now()
		FROM candidate
		WHERE credential.id = candidate.id
		RETURNING
			credential.id::text,
			credential.tenant_id,
			credential.provider_code,
			credential.environment_code,
			credential.credential_alias,
			credential.key_prefix || '••••' || credential.key_last_four,
			credential.daily_limit,
			credential.active,
			credential.validation_status,
			credential.last_validated_at,
			credential.last_selected_at,
			credential.created_at,
			credential.disabled_at,
			credential.secret_ciphertext,
			credential.secret_fingerprint
	`, providerCode, tenantID, environment, providerCredentialID).Scan(
		&item.ID,
		&item.TenantID,
		&item.ProviderCode,
		&item.Environment,
		&item.CredentialAlias,
		&item.DisplayKey,
		&item.DailyLimit,
		&item.Active,
		&item.ValidationStatus,
		&item.LastValidatedAt,
		&item.LastSelectedAt,
		&item.CreatedAt,
		&item.DisabledAt,
		&item.SecretCiphertext,
		&item.Fingerprint,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		var hasActiveCredential bool
		existsErr := r.pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM provider_credentials
				WHERE provider_code = $1
				  AND tenant_id = $2
				  AND environment_code = $3
				  AND ($4 = '' OR $3 <> 'live' OR id::text = $4)
				  AND (
					$2 = '' OR $3 <> 'live' OR EXISTS (
						SELECT 1
						FROM tenant_active_shipping_providers selection
						WHERE selection.tenant_id = provider_credentials.tenant_id
						  AND selection.provider_code = provider_credentials.provider_code
						  AND selection.credential_id = provider_credentials.id
					)
				  )
				  AND active
				  AND validation_status = 'valid'
			)
		`, providerCode, tenantID, environment, providerCredentialID).Scan(&hasActiveCredential)
		if existsErr != nil {
			return StoredCredential{}, fmt.Errorf(
				"check active provider credential: %w",
				existsErr,
			)
		}
		if hasActiveCredential {
			return StoredCredential{}, ErrAllCredentialsExhausted
		}
		return StoredCredential{}, ErrNoActiveCredential
	}
	if err != nil {
		return StoredCredential{}, fmt.Errorf("resolve active provider credential: %w", err)
	}
	return item, nil
}

func (r *PostgresRepository) ActiveCredentialID(
	ctx context.Context,
	tenantID, providerCode string,
) (string, error) {
	var credentialID string
	err := r.pool.QueryRow(ctx, `
		SELECT credential.id::text
		FROM tenant_active_shipping_providers selection
		JOIN provider_credentials credential
		  ON credential.id = selection.credential_id
		WHERE selection.tenant_id = $1
		  AND selection.provider_code = $2
		  AND credential.tenant_id = selection.tenant_id
		  AND credential.provider_code = selection.provider_code
		  AND credential.active
		  AND credential.validation_status = 'valid'
	`, tenantID, providerCode).Scan(&credentialID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNoActiveCredential
	}
	if err != nil {
		return "", fmt.Errorf("resolve active merchant provider credential ID: %w", err)
	}
	return credentialID, nil
}

func (r *PostgresRepository) get(ctx context.Context, id string) (Credential, error) {
	item, err := scanCredential(r.pool.QueryRow(ctx, `
		SELECT
			id::text,
			tenant_id,
			provider_code,
			environment_code,
			credential_alias,
			key_prefix || '••••' || key_last_four,
			daily_limit,
			active,
			validation_status,
			last_validated_at,
			last_selected_at,
			created_at,
			disabled_at
		FROM provider_credentials
		WHERE id = $1::uuid
	`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Credential{}, ErrNotFound
	}
	return item, err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanCredential(row rowScanner) (Credential, error) {
	var item Credential
	if err := row.Scan(
		&item.ID,
		&item.TenantID,
		&item.ProviderCode,
		&item.Environment,
		&item.CredentialAlias,
		&item.DisplayKey,
		&item.DailyLimit,
		&item.Active,
		&item.ValidationStatus,
		&item.LastValidatedAt,
		&item.LastSelectedAt,
		&item.CreatedAt,
		&item.DisabledAt,
	); err != nil {
		return Credential{}, fmt.Errorf("scan provider credential: %w", err)
	}
	return item, nil
}

func insertCredentialAudit(
	ctx context.Context,
	tx pgx.Tx,
	actor, action, resourceID, requestID string,
	details map[string]any,
) error {
	if details == nil {
		details = map[string]any{}
	}
	encoded, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("encode provider credential audit: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO admin_audit_logs (
			actor_alias, action, resource_type, resource_id, request_id, details_json
		)
		VALUES ($1, $2, 'provider_credential', $3, $4, $5::jsonb)
	`, actor, action, resourceID, requestID, string(encoded))
	if err != nil {
		return fmt.Errorf("insert provider credential audit: %w", err)
	}
	return nil
}
