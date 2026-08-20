package webhooksettings

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

func (r *PostgresRepository) Get(ctx context.Context) (StoredSettings, error) {
	var stored StoredSettings
	var updatedAt time.Time
	var prefix, lastFour string
	err := r.pool.QueryRow(ctx, `
		SELECT
			callback_url,
			enabled,
			secret_ciphertext,
			secret_fingerprint,
			secret_prefix,
			secret_last_four,
			last_test_at,
			last_test_success,
			last_test_http_status,
			last_test_error,
			updated_by,
			updated_at
		FROM tracking_webhook_settings
		WHERE id = 1
	`).Scan(
		&stored.CallbackURL,
		&stored.Enabled,
		&stored.SecretCiphertext,
		&stored.SecretFingerprint,
		&prefix,
		&lastFour,
		&stored.LastTestAt,
		&stored.LastTestSuccess,
		&stored.LastTestHTTPStatus,
		&stored.LastTestError,
		&stored.UpdatedBy,
		&updatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return StoredSettings{}, ErrNotConfigured
	}
	if err != nil {
		return StoredSettings{}, fmt.Errorf("get tracking webhook settings: %w", err)
	}

	stored.Configured = true
	stored.SecretConfigured = len(stored.SecretCiphertext) > 0
	stored.SecretHint = maskedSecretHint(prefix, lastFour)
	stored.Source = "database"
	stored.UpdatedAt = &updatedAt
	return stored, nil
}

func (r *PostgresRepository) UpsertConfig(
	ctx context.Context,
	callbackURL string,
	enabled bool,
	actor string,
) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO tracking_webhook_settings (
			id, callback_url, enabled, created_by, updated_by
		)
		VALUES (1, $1, $2, $3, $3)
		ON CONFLICT (id) DO UPDATE
		SET callback_url = EXCLUDED.callback_url,
		    enabled = EXCLUDED.enabled,
		    updated_by = EXCLUDED.updated_by,
		    updated_at = now()
	`, callbackURL, enabled, actor)
	if err != nil {
		return fmt.Errorf("upsert tracking webhook settings: %w", err)
	}
	return nil
}

func (r *PostgresRepository) RotateSecret(
	ctx context.Context,
	input SecretInput,
) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO tracking_webhook_settings (
			id,
			secret_ciphertext,
			secret_fingerprint,
			secret_prefix,
			secret_last_four,
			created_by,
			updated_by
		)
		VALUES (1, $1, $2, $3, $4, $5, $5)
		ON CONFLICT (id) DO UPDATE
		SET secret_ciphertext = EXCLUDED.secret_ciphertext,
		    secret_fingerprint = EXCLUDED.secret_fingerprint,
		    secret_prefix = EXCLUDED.secret_prefix,
		    secret_last_four = EXCLUDED.secret_last_four,
		    updated_by = EXCLUDED.updated_by,
		    updated_at = now()
	`, input.Ciphertext, input.Fingerprint, input.Prefix, input.LastFour, input.Actor)
	if err != nil {
		return fmt.Errorf("rotate tracking webhook secret: %w", err)
	}
	return nil
}

func (r *PostgresRepository) RecordTest(
	ctx context.Context,
	testedAt time.Time,
	success bool,
	httpStatus int,
	message string,
) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE tracking_webhook_settings
		SET last_test_at = $1,
		    last_test_success = $2,
		    last_test_http_status = NULLIF($3, 0),
		    last_test_error = $4,
		    updated_at = now()
		WHERE id = 1
	`, testedAt, success, httpStatus, message)
	if err != nil {
		return fmt.Errorf("record tracking webhook test: %w", err)
	}
	return nil
}

func maskedSecretHint(prefix, lastFour string) string {
	if prefix == "" || lastFour == "" {
		return ""
	}
	return prefix + "••••••••" + lastFour
}
