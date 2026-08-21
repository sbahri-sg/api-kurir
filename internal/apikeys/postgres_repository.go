package apikeys

import (
	"context"
	"encoding/json"
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

func (r *PostgresRepository) List(
	ctx context.Context,
	limit, offset int,
) ([]APIKey, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			id::text,
			key_prefix,
			key_last_four,
			key_prefix || '••••' || key_last_four,
			scopes,
			active,
			last_used_at,
			created_by,
			created_at,
			revoked_by,
			revoked_at
		FROM customer_api_keys
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list customer api keys: %w", err)
	}
	defer rows.Close()

	items := make([]APIKey, 0)
	for rows.Next() {
		item, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate customer api keys: %w", err)
	}
	return items, nil
}

func (r *PostgresRepository) Create(
	ctx context.Context,
	input CreateInput,
) (APIKey, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return APIKey{}, fmt.Errorf("begin customer api key create: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO customer_api_keys (
			key_prefix,
			key_last_four,
			key_hash,
			scopes,
			created_by
		)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id::text
	`,
		input.KeyPrefix,
		input.KeyLast4,
		input.KeyHash,
		input.Scopes,
		input.CreatedBy,
	).Scan(&id)
	if err != nil {
		return APIKey{}, fmt.Errorf("insert customer api key: %w", err)
	}
	if err := insertAudit(
		ctx,
		tx,
		input.CreatedBy,
		"create",
		"customer_api_key",
		id,
		input.RequestID,
		map[string]any{
			"key_prefix": input.KeyPrefix, "scopes": input.Scopes,
		},
	); err != nil {
		return APIKey{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return APIKey{}, fmt.Errorf("commit customer api key create: %w", err)
	}
	return r.get(ctx, id)
}

func (r *PostgresRepository) Revoke(
	ctx context.Context,
	id, actor, requestID string,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin customer api key revoke: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	commandTag, err := tx.Exec(ctx, `
		UPDATE customer_api_keys
		SET active = false,
		    revoked_by = $2,
		    revoked_at = now(),
		    updated_at = now()
		WHERE id = $1::uuid
		  AND active
	`, id, actor)
	if err != nil {
		return fmt.Errorf("revoke customer api key: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if err := insertAudit(
		ctx,
		tx,
		actor,
		"revoke",
		"customer_api_key",
		id,
		requestID,
		nil,
	); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit customer api key revoke: %w", err)
	}
	return nil
}

func (r *PostgresRepository) Authenticate(
	ctx context.Context,
	keyHash []byte,
) (bool, error) {
	var id string
	err := r.pool.QueryRow(ctx, `
		UPDATE customer_api_keys
		SET last_used_at = now()
		WHERE key_hash = $1
		  AND active
		RETURNING id::text
	`, keyHash).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("authenticate customer api key: %w", err)
	}
	return true, nil
}

func (r *PostgresRepository) AuthenticateScope(
	ctx context.Context,
	keyHash []byte,
	scope string,
) (bool, error) {
	var id string
	err := r.pool.QueryRow(ctx, `
		UPDATE customer_api_keys
		SET last_used_at = now()
		WHERE key_hash = $1
		  AND active
		  AND $2 = ANY(scopes)
		RETURNING id::text
	`, keyHash, scope).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("authorize customer api key scope: %w", err)
	}
	return true, nil
}

func (r *PostgresRepository) get(ctx context.Context, id string) (APIKey, error) {
	item, err := scanAPIKey(r.pool.QueryRow(ctx, `
		SELECT
			id::text,
			key_prefix,
			key_last_four,
			key_prefix || '••••' || key_last_four,
			scopes,
			active,
			last_used_at,
			created_by,
			created_at,
			revoked_by,
			revoked_at
		FROM customer_api_keys
		WHERE id = $1::uuid
	`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return APIKey{}, ErrNotFound
	}
	return item, err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAPIKey(row rowScanner) (APIKey, error) {
	var item APIKey
	if err := row.Scan(
		&item.ID,
		&item.KeyPrefix,
		&item.KeyLastFour,
		&item.DisplayKey,
		&item.Scopes,
		&item.Active,
		&item.LastUsedAt,
		&item.CreatedBy,
		&item.CreatedAt,
		&item.RevokedBy,
		&item.RevokedAt,
	); err != nil {
		return APIKey{}, fmt.Errorf("scan customer api key: %w", err)
	}
	item.Kind = keyKindFromScopes(item.Scopes)
	return item, nil
}

func insertAudit(
	ctx context.Context,
	tx pgx.Tx,
	actor, action, resourceType, resourceID, requestID string,
	details map[string]any,
) error {
	if details == nil {
		details = map[string]any{}
	}
	encoded, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("encode customer api key audit: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO admin_audit_logs (
			actor_alias, action, resource_type, resource_id, request_id, details_json
		)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb)
	`, actor, action, resourceType, resourceID, requestID, string(encoded))
	if err != nil {
		return fmt.Errorf("insert customer api key audit: %w", err)
	}
	return nil
}
