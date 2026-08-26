package partnerpackages

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const submissionSelect = `
	SELECT
		submission.id::text,
		submission.provider_code,
		provider.name,
		submission.version,
		submission.status,
		COALESCE(provider.active_release_id = submission.id, false) AS is_active_release,
		submission.file_name,
		submission.content_type,
		submission.artifact_size,
		submission.artifact_sha256,
		submission.scan_report,
		submission.required_scopes,
		submission.review_note,
		submission.submitted_by,
		submission.reviewed_by,
		submission.created_at,
		submission.updated_at
	FROM partner_integration_submissions submission
	JOIN shipping_integration_providers provider
	  ON provider.code = submission.provider_code
`

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) List(ctx context.Context, filter Filter) ([]Submission, error) {
	rows, err := r.pool.Query(ctx, submissionSelect+`
		WHERE ($1 = '' OR submission.provider_code = $1)
		  AND ($2 = '' OR submission.status = $2)
		ORDER BY submission.created_at DESC
		LIMIT $3 OFFSET $4
	`, filter.ProviderCode, filter.Status, filter.Limit, filter.Offset)
	if err != nil {
		return nil, fmt.Errorf("list partner integration submissions: %w", err)
	}
	defer rows.Close()

	items := make([]Submission, 0)
	for rows.Next() {
		item, err := scanSubmission(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate partner integration submissions: %w", err)
	}
	return items, nil
}

func (r *PostgresRepository) Get(ctx context.Context, id string) (Submission, error) {
	item, err := scanSubmission(r.pool.QueryRow(ctx, submissionSelect+`
		WHERE submission.id = $1::uuid
	`, id))
	if errors.Is(err, pgx.ErrNoRows) || isInvalidTextRepresentation(err) {
		return Submission{}, ErrNotFound
	}
	return item, err
}

func (r *PostgresRepository) Create(
	ctx context.Context,
	input UploadInput,
	digest string,
	report ScanReport,
	requiredScopes []string,
	status string,
) (Submission, error) {
	reportJSON, err := json.Marshal(report)
	if err != nil {
		return Submission{}, fmt.Errorf("encode partner package scan report: %w", err)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Submission{}, fmt.Errorf("begin partner integration submission: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Serialize quota checks per provider so concurrent uploads cannot both pass
	// against the same remaining capacity.
	if _, err := tx.Exec(ctx, `
		SELECT pg_advisory_xact_lock(hashtextextended('partner-package:' || $1, 0))
	`, input.ProviderCode); err != nil {
		return Submission{}, fmt.Errorf("lock partner package quota: %w", err)
	}
	var submissionCount int
	var storageBytes int64
	if err := tx.QueryRow(ctx, `
		SELECT count(*)::int, COALESCE(sum(artifact_size), 0)::bigint
		FROM partner_integration_submissions
		WHERE provider_code = $1
	`, input.ProviderCode).Scan(&submissionCount, &storageBytes); err != nil {
		return Submission{}, fmt.Errorf("read partner package quota: %w", err)
	}
	if submissionCount >= MaxProviderSubmissions ||
		storageBytes+int64(len(input.Payload)) > MaxProviderStorageBytes {
		return Submission{}, ErrStorageQuota
	}

	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO partner_integration_submissions (
			provider_code, version, status, file_name, content_type,
			artifact_size, artifact_sha256, artifact, scan_report,
			required_scopes, submitted_by
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10, $11)
		RETURNING id::text
	`,
		input.ProviderCode,
		input.Version,
		status,
		input.FileName,
		input.ContentType,
		len(input.Payload),
		digest,
		input.Payload,
		string(reportJSON),
		requiredScopes,
		input.SubmittedBy,
	).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return Submission{}, ErrConflict
		}
		if isForeignKeyViolation(err) {
			return Submission{}, ErrNotFound
		}
		return Submission{}, fmt.Errorf("create partner integration submission: %w", err)
	}
	auditDetails, err := json.Marshal(map[string]any{
		"provider_code":   input.ProviderCode,
		"version":         input.Version,
		"status":          status,
		"artifact_size":   len(input.Payload),
		"artifact_sha256": digest,
		"scan_passed":     report.Passed,
		"required_scopes": requiredScopes,
	})
	if err != nil {
		return Submission{}, fmt.Errorf("encode partner submission create audit: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO admin_audit_logs (
			actor_alias, action, resource_type, resource_id, request_id, details_json
		)
		VALUES ($1, 'create', 'partner_integration_submission', $2, $3, $4::jsonb)
	`, input.SubmittedBy, id, input.RequestID, string(auditDetails))
	if err != nil {
		return Submission{}, fmt.Errorf("insert partner submission create audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Submission{}, fmt.Errorf("commit partner integration submission: %w", err)
	}
	return r.Get(ctx, id)
}

func (r *PostgresRepository) ReserveUploadAttempt(
	ctx context.Context,
	keyID string,
	maximum int,
) error {
	if maximum <= 0 {
		return ErrUploadRateLimit
	}
	var attempts int
	err := r.pool.QueryRow(ctx, `
		INSERT INTO partner_upload_rate_limits (
			key_id, bucket_start, attempts, updated_at
		)
		VALUES ($1::uuid, date_trunc('hour', now()), 1, now())
		ON CONFLICT (key_id, bucket_start) DO UPDATE
		SET attempts = partner_upload_rate_limits.attempts + 1,
		    updated_at = now()
		WHERE partner_upload_rate_limits.attempts < $2
		RETURNING attempts
	`, keyID, maximum).Scan(&attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrUploadRateLimit
	}
	if isInvalidTextRepresentation(err) || isForeignKeyViolation(err) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("reserve partner upload attempt: %w", err)
	}
	return nil
}

func (r *PostgresRepository) UpdateStatus(
	ctx context.Context,
	id string,
	update StatusUpdate,
) (Submission, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Submission{}, fmt.Errorf("begin partner submission status update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var providerCode string
	err = tx.QueryRow(ctx, `
		SELECT provider_code
		FROM partner_integration_submissions
		WHERE id = $1::uuid
	`, id).Scan(&providerCode)
	if errors.Is(err, pgx.ErrNoRows) || isInvalidTextRepresentation(err) {
		return Submission{}, ErrNotFound
	}
	if err != nil {
		return Submission{}, fmt.Errorf("get partner integration submission provider: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		SELECT pg_advisory_xact_lock(hashtextextended($1, 0))
	`, "partner-submission:"+providerCode); err != nil {
		return Submission{}, fmt.Errorf("lock partner integration provider: %w", err)
	}

	var previousStatus string
	err = tx.QueryRow(ctx, `
		SELECT status
		FROM partner_integration_submissions
		WHERE id = $1::uuid
		FOR UPDATE
	`, id).Scan(&previousStatus)
	if errors.Is(err, pgx.ErrNoRows) || isInvalidTextRepresentation(err) {
		return Submission{}, ErrNotFound
	}
	if err != nil {
		return Submission{}, fmt.Errorf("lock partner integration submission: %w", err)
	}
	if previousStatus != update.ExpectedStatus {
		return Submission{}, ErrStatusConflict
	}

	supersededSubmissionIDs := make([]string, 0)
	if update.Status == "published" {
		rows, supersedeErr := tx.Query(ctx, `
			UPDATE partner_integration_submissions
			SET status = 'superseded',
			    reviewed_by = $3,
			    review_note = CASE
			        WHEN review_note = '' THEN 'Digantikan oleh versi yang lebih baru.'
			        ELSE review_note
			    END,
			    updated_at = now()
			WHERE provider_code = $1
			  AND status = 'published'
			  AND id <> $2::uuid
			RETURNING id::text
		`, providerCode, id, update.ReviewedBy)
		if supersedeErr != nil {
			return Submission{}, fmt.Errorf("supersede previous partner submission: %w", supersedeErr)
		}
		for rows.Next() {
			var supersededID string
			if err := rows.Scan(&supersededID); err != nil {
				rows.Close()
				return Submission{}, fmt.Errorf("scan superseded partner submission: %w", err)
			}
			supersededSubmissionIDs = append(supersededSubmissionIDs, supersededID)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return Submission{}, fmt.Errorf("iterate superseded partner submissions: %w", err)
		}
		rows.Close()
	}

	_, err = tx.Exec(ctx, `
		UPDATE partner_integration_submissions
		SET status = $2,
		    review_note = $3,
		    reviewed_by = $4,
		    updated_at = now()
		WHERE id = $1::uuid
	`, id, update.Status, update.ReviewNote, update.ReviewedBy)
	if err != nil {
		return Submission{}, fmt.Errorf("update partner integration submission status: %w", err)
	}
	activeRelease := false
	deactivatedMerchantCount := int64(0)
	if update.Status == "published" {
		result, releaseErr := tx.Exec(ctx, `
			UPDATE shipping_integration_providers
			SET active_release_id = $2::uuid,
			    updated_at = now()
			WHERE code = $1
			  AND integration_type = 'partner_hosted'
		`, providerCode, id)
		if releaseErr != nil {
			return Submission{}, fmt.Errorf("activate partner integration release: %w", releaseErr)
		}
		if result.RowsAffected() == 0 {
			return Submission{}, ErrProviderType
		}
		activeRelease = true
	} else if previousStatus == "published" {
		_, releaseErr := tx.Exec(ctx, `
			UPDATE shipping_integration_providers
			SET active_release_id = NULL,
			    available = false,
			    updated_at = now()
			WHERE code = $1
			  AND active_release_id = $2::uuid
		`, providerCode, id)
		if releaseErr != nil {
			return Submission{}, fmt.Errorf("deactivate partner integration release: %w", releaseErr)
		}
		merchantResult, merchantErr := tx.Exec(ctx, `
			UPDATE tenant_active_shipping_providers
			SET provider_code = NULL,
			    credential_id = NULL,
			    release_id = NULL,
			    granted_scopes = '{}'::text[],
			    version = version + 1,
			    updated_by = 'system:release-suspended',
			    updated_at = now()
			WHERE release_id = $1::uuid
		`, id)
		if merchantErr != nil {
			return Submission{}, fmt.Errorf("deactivate merchants for suspended release: %w", merchantErr)
		}
		deactivatedMerchantCount = merchantResult.RowsAffected()
	}
	details, err := json.Marshal(map[string]any{
		"provider_code":              providerCode,
		"previous_status":            previousStatus,
		"status":                     update.Status,
		"review_note":                update.ReviewNote,
		"superseded_submission_ids":  supersededSubmissionIDs,
		"active_release":             activeRelease,
		"deactivated_merchant_count": deactivatedMerchantCount,
	})
	if err != nil {
		return Submission{}, fmt.Errorf("encode partner submission audit: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO admin_audit_logs (
			actor_alias, action, resource_type, resource_id, request_id, details_json
		)
		VALUES ($1, 'update_status', 'partner_integration_submission', $2, $3, $4::jsonb)
	`, update.ReviewedBy, id, update.RequestID, string(details))
	if err != nil {
		return Submission{}, fmt.Errorf("insert partner submission audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Submission{}, fmt.Errorf("commit partner submission status update: %w", err)
	}
	return r.Get(ctx, id)
}

func (r *PostgresRepository) Artifact(
	ctx context.Context,
	id, actor, requestID string,
) (Artifact, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Artifact{}, fmt.Errorf("begin partner integration artifact download: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var artifact Artifact
	err = tx.QueryRow(ctx, `
		SELECT file_name, content_type, artifact
		FROM partner_integration_submissions
		WHERE id = $1::uuid
	`, id).Scan(&artifact.FileName, &artifact.ContentType, &artifact.Payload)
	if errors.Is(err, pgx.ErrNoRows) || isInvalidTextRepresentation(err) {
		return Artifact{}, ErrNotFound
	}
	if err != nil {
		return Artifact{}, fmt.Errorf("get partner integration artifact: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO admin_audit_logs (
			actor_alias, action, resource_type, resource_id, request_id, details_json
		)
		VALUES ($1, 'download_artifact', 'partner_integration_submission', $2, $3, '{}'::jsonb)
	`, actor, id, requestID)
	if err != nil {
		return Artifact{}, fmt.Errorf("insert partner artifact download audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Artifact{}, fmt.Errorf("commit partner artifact download audit: %w", err)
	}
	return artifact, nil
}

const accessKeySelect = `
	SELECT
		access_key.id::text,
		access_key.provider_code,
		provider.name,
		access_key.display_key,
		access_key.active,
		access_key.created_by,
		access_key.created_at,
		access_key.last_used_at,
		access_key.revoked_by,
		access_key.revoked_at
	FROM partner_access_keys access_key
	JOIN shipping_integration_providers provider
	  ON provider.code = access_key.provider_code
`

func (r *PostgresRepository) ListAccessKeys(
	ctx context.Context,
	providerCode string,
) ([]AccessKey, error) {
	rows, err := r.pool.Query(ctx, accessKeySelect+`
		WHERE access_key.provider_code = $1
		ORDER BY access_key.created_at DESC
	`, providerCode)
	if err != nil {
		return nil, fmt.Errorf("list partner access keys: %w", err)
	}
	defer rows.Close()

	items := make([]AccessKey, 0)
	for rows.Next() {
		item, scanErr := scanAccessKey(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate partner access keys: %w", err)
	}
	return items, nil
}

func (r *PostgresRepository) CreateAccessKey(
	ctx context.Context,
	input AccessKeyCreateInput,
) (AccessKey, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return AccessKey{}, fmt.Errorf("begin partner access key creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var integrationType string
	err = tx.QueryRow(ctx, `
		SELECT integration_type
		FROM shipping_integration_providers
		WHERE code = $1
		FOR SHARE
	`, input.ProviderCode).Scan(&integrationType)
	if errors.Is(err, pgx.ErrNoRows) {
		return AccessKey{}, ErrNotFound
	}
	if err != nil {
		return AccessKey{}, fmt.Errorf("get partner access provider: %w", err)
	}
	if integrationType != "partner_hosted" {
		return AccessKey{}, ErrProviderType
	}

	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO partner_access_keys (
			provider_code, display_key, secret_hash, created_by
		)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text
	`, input.ProviderCode, input.DisplayKey, input.SecretHash, input.CreatedBy).Scan(&id)
	if err != nil {
		if isForeignKeyViolation(err) {
			return AccessKey{}, ErrNotFound
		}
		return AccessKey{}, fmt.Errorf("create partner access key: %w", err)
	}
	details, err := json.Marshal(map[string]any{
		"provider_code": input.ProviderCode,
		"display_key":   input.DisplayKey,
	})
	if err != nil {
		return AccessKey{}, fmt.Errorf("encode partner access key audit: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO admin_audit_logs (
			actor_alias, action, resource_type, resource_id, request_id, details_json
		)
		VALUES ($1, 'create', 'partner_access_key', $2, $3, $4::jsonb)
	`, input.CreatedBy, id, input.RequestID, string(details))
	if err != nil {
		return AccessKey{}, fmt.Errorf("insert partner access key audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return AccessKey{}, fmt.Errorf("commit partner access key creation: %w", err)
	}
	return r.getAccessKey(ctx, id, input.ProviderCode)
}

func (r *PostgresRepository) RevokeAccessKey(
	ctx context.Context,
	input AccessKeyRevokeInput,
) (AccessKey, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return AccessKey{}, fmt.Errorf("begin partner access key revocation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	result, err := tx.Exec(ctx, `
		UPDATE partner_access_keys
		SET active = false,
		    revoked_by = $3,
		    revoked_at = now()
		WHERE id = $1::uuid
		  AND provider_code = $2
		  AND active = true
	`, input.KeyID, input.ProviderCode, input.RevokedBy)
	if isInvalidTextRepresentation(err) {
		return AccessKey{}, ErrNotFound
	}
	if err != nil {
		return AccessKey{}, fmt.Errorf("revoke partner access key: %w", err)
	}
	if result.RowsAffected() == 0 {
		return AccessKey{}, ErrNotFound
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO admin_audit_logs (
			actor_alias, action, resource_type, resource_id, request_id, details_json
		)
		VALUES ($1, 'revoke', 'partner_access_key', $2, $3, jsonb_build_object('provider_code', $4::text))
	`, input.RevokedBy, input.KeyID, input.RequestID, input.ProviderCode)
	if err != nil {
		return AccessKey{}, fmt.Errorf("insert partner access key revoke audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return AccessKey{}, fmt.Errorf("commit partner access key revocation: %w", err)
	}
	return r.getAccessKey(ctx, input.KeyID, input.ProviderCode)
}

func (r *PostgresRepository) AuthenticateAccessKey(
	ctx context.Context,
	secretHash string,
) (AccessIdentity, error) {
	var identity AccessIdentity
	err := r.pool.QueryRow(ctx, `
		UPDATE partner_access_keys access_key
		SET last_used_at = now()
		FROM shipping_integration_providers provider
		WHERE access_key.secret_hash = $1
		  AND access_key.active = true
		  AND access_key.provider_code = provider.code
		  AND provider.integration_type = 'partner_hosted'
		RETURNING access_key.id::text, access_key.provider_code, provider.name
	`, secretHash).Scan(&identity.KeyID, &identity.ProviderCode, &identity.ProviderName)
	if errors.Is(err, pgx.ErrNoRows) {
		return AccessIdentity{}, ErrNotFound
	}
	if err != nil {
		return AccessIdentity{}, fmt.Errorf("authenticate partner access key: %w", err)
	}
	return identity, nil
}

func (r *PostgresRepository) getAccessKey(
	ctx context.Context,
	id, providerCode string,
) (AccessKey, error) {
	item, err := scanAccessKey(r.pool.QueryRow(ctx, accessKeySelect+`
		WHERE access_key.id = $1::uuid
		  AND access_key.provider_code = $2
	`, id, providerCode))
	if errors.Is(err, pgx.ErrNoRows) || isInvalidTextRepresentation(err) {
		return AccessKey{}, ErrNotFound
	}
	return item, err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSubmission(row rowScanner) (Submission, error) {
	var item Submission
	var scanReportJSON []byte
	err := row.Scan(
		&item.ID,
		&item.ProviderCode,
		&item.ProviderName,
		&item.Version,
		&item.Status,
		&item.IsActiveRelease,
		&item.FileName,
		&item.ContentType,
		&item.ArtifactSize,
		&item.ArtifactSHA256,
		&scanReportJSON,
		&item.RequiredScopes,
		&item.ReviewNote,
		&item.SubmittedBy,
		&item.ReviewedBy,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return Submission{}, fmt.Errorf("scan partner integration submission: %w", err)
	}
	if err := json.Unmarshal(scanReportJSON, &item.ScanReport); err != nil {
		return Submission{}, fmt.Errorf("decode partner package scan report: %w", err)
	}
	return item, nil
}

func scanAccessKey(row rowScanner) (AccessKey, error) {
	var item AccessKey
	err := row.Scan(
		&item.ID,
		&item.ProviderCode,
		&item.ProviderName,
		&item.DisplayKey,
		&item.Active,
		&item.CreatedBy,
		&item.CreatedAt,
		&item.LastUsedAt,
		&item.RevokedBy,
		&item.RevokedAt,
	)
	if err != nil {
		return AccessKey{}, fmt.Errorf("scan partner access key: %w", err)
	}
	return item, nil
}

func postgresErrorCode(err error) string {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		return postgresError.Code
	}
	return ""
}

func isUniqueViolation(err error) bool {
	return postgresErrorCode(err) == "23505"
}

func isForeignKeyViolation(err error) bool {
	return postgresErrorCode(err) == "23503"
}

func isInvalidTextRepresentation(err error) bool {
	return postgresErrorCode(err) == "22P02"
}
