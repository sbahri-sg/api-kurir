package providercredentials

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

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

func (r *PostgresRepository) List(ctx context.Context) ([]Credential, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			id::text,
			provider_code,
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
		ORDER BY active DESC, created_at DESC
	`)
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
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Credential{}, fmt.Errorf("begin provider credential create: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `
		INSERT INTO provider_credentials (
			id,
			provider_code,
			credential_alias,
			secret_ciphertext,
			secret_fingerprint,
			key_prefix,
			key_last_four,
			daily_limit,
			created_by
		)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9)
	`,
		input.ID,
		input.ProviderCode,
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
	_, err = tx.Exec(ctx, `
		INSERT INTO provider_quota_ledger (
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
			(now() AT TIME ZONE 'Asia/Jakarta')::date,
			$3,
			1,
			(
				((now() AT TIME ZONE 'Asia/Jakarta')::date + 1)::timestamp
				AT TIME ZONE 'Asia/Jakarta'
			)
		)
		ON CONFLICT (provider_code, credential_alias, quota_date) DO UPDATE
		SET daily_limit = EXCLUDED.daily_limit,
		    used_count = LEAST(
				EXCLUDED.daily_limit,
				provider_quota_ledger.used_count + 1
			),
		    reset_at = EXCLUDED.reset_at,
		    updated_at = now()
	`,
		input.ProviderCode,
		input.CredentialAlias,
		input.DailyLimit,
	)
	if err != nil {
		return Credential{}, fmt.Errorf("record provider credential validation hit: %w", err)
	}
	if err := insertCredentialAudit(
		ctx,
		tx,
		input.CreatedBy,
		"create",
		input.ID,
		input.RequestID,
		map[string]any{
			"provider_code":    input.ProviderCode,
			"credential_alias": input.CredentialAlias,
			"key_prefix":       input.KeyPrefix,
		},
	); err != nil {
		return Credential{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Credential{}, fmt.Errorf("commit provider credential create: %w", err)
	}
	return r.get(ctx, input.ID)
}

func (r *PostgresRepository) Disable(
	ctx context.Context,
	id, actor, requestID string,
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
		  AND active
	`, id, actor)
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
	var item StoredCredential
	err := r.pool.QueryRow(ctx, `
		WITH candidate AS (
			SELECT credential.id
			FROM provider_credentials credential
			LEFT JOIN provider_quota_ledger quota
			  ON quota.provider_code = credential.provider_code
			 AND quota.credential_alias = credential.credential_alias
			 AND quota.quota_date = (now() AT TIME ZONE 'Asia/Jakarta')::date
			WHERE credential.provider_code = $1
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
			credential.provider_code,
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
	`, providerCode).Scan(
		&item.ID,
		&item.ProviderCode,
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
				  AND active
				  AND validation_status = 'valid'
			)
		`, providerCode).Scan(&hasActiveCredential)
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

func (r *PostgresRepository) get(ctx context.Context, id string) (Credential, error) {
	item, err := scanCredential(r.pool.QueryRow(ctx, `
		SELECT
			id::text,
			provider_code,
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
		&item.ProviderCode,
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
