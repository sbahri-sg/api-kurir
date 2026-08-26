package partnerexplorer

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

func (r *PostgresRepository) SubmissionArtifact(
	ctx context.Context,
	id, providerCode string,
) (SubmissionArtifact, error) {
	var item SubmissionArtifact
	var scanReport []byte
	err := r.pool.QueryRow(ctx, `
		SELECT submission.id::text, submission.provider_code, provider.name,
		       submission.version, submission.status, submission.scan_report,
		       submission.artifact
		FROM partner_integration_submissions submission
		JOIN shipping_integration_providers provider
		  ON provider.code = submission.provider_code
		WHERE submission.id = $1::uuid
		  AND submission.provider_code = $2
	`, id, providerCode).Scan(
		&item.ID, &item.ProviderCode, &item.ProviderName, &item.Version,
		&item.Status, &scanReport, &item.Payload,
	)
	if errors.Is(err, pgx.ErrNoRows) || postgresCode(err) == "22P02" {
		return SubmissionArtifact{}, ErrSubmissionNotFound
	}
	if err != nil {
		return SubmissionArtifact{}, fmt.Errorf("get partner explorer submission: %w", err)
	}
	var report struct {
		Passed   bool `json:"passed"`
		Manifest struct {
			BaseURL string `json:"base_url"`
		} `json:"manifest"`
	}
	if err := json.Unmarshal(scanReport, &report); err != nil {
		return SubmissionArtifact{}, fmt.Errorf("decode partner explorer scan report: %w", err)
	}
	item.ScanPassed = report.Passed
	item.ProductionURL = report.Manifest.BaseURL
	return item, nil
}

func (r *PostgresRepository) Credential(
	ctx context.Context,
	providerCode, credentialCode string,
) (StoredCredential, error) {
	var item StoredCredential
	err := r.pool.QueryRow(ctx, `
		SELECT provider_code, credential_code, display_key, auth_header, auth_prefix,
		       secret_ciphertext, created_at, updated_at
		FROM partner_explorer_credentials
		WHERE provider_code = $1 AND credential_code = $2
	`, providerCode, credentialCode).Scan(
		&item.ProviderCode, &item.CredentialCode, &item.DisplayKey, &item.AuthHeader, &item.AuthPrefix,
		&item.SecretCiphertext, &item.CreatedAt, &item.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return StoredCredential{}, ErrCredentialNotFound
	}
	if err != nil {
		return StoredCredential{}, fmt.Errorf("get partner explorer credential: %w", err)
	}
	return item, nil
}

func (r *PostgresRepository) UpsertCredential(
	ctx context.Context,
	input CredentialInput,
) (Credential, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Credential{}, fmt.Errorf("begin partner explorer credential upsert: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var item Credential
	err = tx.QueryRow(ctx, `
		INSERT INTO partner_explorer_credentials (
			provider_code, credential_code, display_key, auth_header, auth_prefix,
			secret_ciphertext, created_by, updated_by
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $7)
		ON CONFLICT (provider_code, credential_code) DO UPDATE
		SET display_key = EXCLUDED.display_key,
		    auth_header = EXCLUDED.auth_header,
		    auth_prefix = EXCLUDED.auth_prefix,
		    secret_ciphertext = EXCLUDED.secret_ciphertext,
		    updated_by = EXCLUDED.updated_by,
		    updated_at = now()
		RETURNING provider_code, credential_code, display_key, auth_header, auth_prefix,
		          created_at, updated_at
	`, input.ProviderCode, input.CredentialCode, input.DisplayKey, input.AuthHeader, input.AuthPrefix,
		input.SecretCiphertext, input.Actor,
	).Scan(
		&item.ProviderCode, &item.CredentialCode, &item.DisplayKey, &item.AuthHeader, &item.AuthPrefix,
		&item.CreatedAt, &item.UpdatedAt,
	)
	if postgresCode(err) == "23503" {
		return Credential{}, ErrSubmissionNotFound
	}
	if err != nil {
		return Credential{}, fmt.Errorf("upsert partner explorer credential: %w", err)
	}
	details, _ := json.Marshal(map[string]any{
		"provider_code":   input.ProviderCode,
		"credential_code": input.CredentialCode,
		"display_key":     input.DisplayKey,
		"auth_header":     input.AuthHeader,
	})
	if _, err := tx.Exec(ctx, `
		INSERT INTO admin_audit_logs (
			actor_alias, action, resource_type, resource_id, request_id, details_json
		)
		VALUES ($1, 'upsert', 'partner_explorer_credential', $2, $3, $4::jsonb)
	`, input.Actor, input.ProviderCode+":"+input.CredentialCode, input.RequestID, string(details)); err != nil {
		return Credential{}, fmt.Errorf("audit partner explorer credential: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Credential{}, fmt.Errorf("commit partner explorer credential: %w", err)
	}
	return item, nil
}

func (r *PostgresRepository) DeleteCredential(
	ctx context.Context,
	providerCode, credentialCode, actor, requestID string,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin partner explorer credential delete: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	result, err := tx.Exec(ctx, `
		DELETE FROM partner_explorer_credentials
		WHERE provider_code = $1 AND credential_code = $2
	`, providerCode, credentialCode)
	if err != nil {
		return fmt.Errorf("delete partner explorer credential: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrCredentialNotFound
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO admin_audit_logs (
			actor_alias, action, resource_type, resource_id, request_id, details_json
		)
		VALUES ($1, 'delete', 'partner_explorer_credential', $2, $3,
		        jsonb_build_object('provider_code', $4::text, 'credential_code', $5::text))
	`, actor, providerCode+":"+credentialCode, requestID, providerCode, credentialCode); err != nil {
		return fmt.Errorf("audit partner explorer credential delete: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit partner explorer credential delete: %w", err)
	}
	return nil
}

func (r *PostgresRepository) ReserveRunAttempt(
	ctx context.Context,
	keyID string,
	maximum int,
) error {
	var attempts int
	err := r.pool.QueryRow(ctx, `
		INSERT INTO partner_explorer_rate_limits (
			key_id, bucket_start, attempts, updated_at
		)
		VALUES ($1::uuid, date_trunc('hour', now()), 1, now())
		ON CONFLICT (key_id, bucket_start) DO UPDATE
		SET attempts = partner_explorer_rate_limits.attempts + 1,
		    updated_at = now()
		WHERE partner_explorer_rate_limits.attempts < $2
		RETURNING attempts
	`, keyID, maximum).Scan(&attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrRateLimit
	}
	if postgresCode(err) == "22P02" || postgresCode(err) == "23503" {
		return ErrSubmissionNotFound
	}
	if err != nil {
		return fmt.Errorf("reserve partner explorer attempt: %w", err)
	}
	return nil
}

func (r *PostgresRepository) CreateRun(ctx context.Context, input RunInput) (Run, error) {
	var item Run
	err := r.pool.QueryRow(ctx, `
		INSERT INTO partner_explorer_runs (
			submission_id, provider_code, operation_id, method, path,
			environment, response_status, duration_ms, outcome, error_code,
			response_preview, created_by
		)
		VALUES ($1::uuid, $2, $3, $4, $5, 'official', $6, $7, $8, $9, $10, $11)
		RETURNING id::text, submission_id::text, provider_code, operation_id,
		          method, path, environment, response_status, duration_ms,
		          outcome, error_code, response_preview, created_by, created_at
	`, input.SubmissionID, input.ProviderCode, input.OperationID, input.Method,
		input.Path, input.ResponseStatus, input.DurationMS, input.Outcome,
		input.ErrorCode, input.ResponsePreview, input.CreatedBy,
	).Scan(
		&item.ID, &item.SubmissionID, &item.ProviderCode, &item.OperationID,
		&item.Method, &item.Path, &item.Environment, &item.ResponseStatus,
		&item.DurationMS, &item.Outcome, &item.ErrorCode,
		&item.ResponsePreview, &item.CreatedBy, &item.CreatedAt,
	)
	if postgresCode(err) == "23503" || postgresCode(err) == "22P02" {
		return Run{}, ErrSubmissionNotFound
	}
	if err != nil {
		return Run{}, fmt.Errorf("create partner explorer run: %w", err)
	}
	return item, nil
}

func (r *PostgresRepository) ListRuns(
	ctx context.Context,
	submissionID, providerCode string,
	limit int,
) ([]Run, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, submission_id::text, provider_code, operation_id,
		       method, path, environment, response_status, duration_ms,
		       outcome, error_code, response_preview, created_by, created_at
		FROM partner_explorer_runs
		WHERE submission_id = $1::uuid AND provider_code = $2
		ORDER BY created_at DESC
		LIMIT $3
	`, submissionID, providerCode, limit)
	if postgresCode(err) == "22P02" {
		return nil, ErrSubmissionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("list partner explorer runs: %w", err)
	}
	defer rows.Close()
	items := make([]Run, 0)
	for rows.Next() {
		var item Run
		if err := rows.Scan(
			&item.ID, &item.SubmissionID, &item.ProviderCode, &item.OperationID,
			&item.Method, &item.Path, &item.Environment, &item.ResponseStatus,
			&item.DurationMS, &item.Outcome, &item.ErrorCode,
			&item.ResponsePreview, &item.CreatedBy, &item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan partner explorer run: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func postgresCode(err error) string {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		return postgresError.Code
	}
	return ""
}
