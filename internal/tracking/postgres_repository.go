package tracking

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/emisell/api-kurir/internal/tenancy"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// MigrateLegacyWaybills decrypts rows written before the plaintext waybill
// column existed. The legacy ciphertext is removed after a successful update.
// It is safe to call this method from more than one application instance.
func (r *PostgresRepository) MigrateLegacyWaybills(
	ctx context.Context,
	cipher *Cipher,
) (int64, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, courier_code, waybill_ciphertext
		FROM tracking_shipments
		WHERE waybill IS NULL AND waybill_ciphertext IS NOT NULL
	`)
	if err != nil {
		return 0, fmt.Errorf("list legacy tracking waybills: %w", err)
	}
	type legacyWaybill struct {
		id, courierCode string
		ciphertext      []byte
	}
	legacy := make([]legacyWaybill, 0)
	for rows.Next() {
		var item legacyWaybill
		if err := rows.Scan(&item.id, &item.courierCode, &item.ciphertext); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan legacy tracking waybill: %w", err)
		}
		legacy = append(legacy, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("iterate legacy tracking waybills: %w", err)
	}
	rows.Close()

	var migrated int64
	for _, item := range legacy {
		plaintext, err := cipher.Decrypt(item.ciphertext, []byte(item.courierCode))
		if err != nil {
			return migrated, fmt.Errorf("decrypt legacy tracking waybill %s: %w", item.id, err)
		}
		waybill := string(plaintext)
		zeroBytes(plaintext)
		tag, err := r.pool.Exec(ctx, `
			UPDATE tracking_shipments
			SET waybill = $2,
			    waybill_ciphertext = NULL,
			    updated_at = now()
			WHERE id = $1::uuid AND waybill IS NULL
		`, item.id, waybill)
		if err != nil {
			return migrated, fmt.Errorf("store plaintext tracking waybill %s: %w", item.id, err)
		}
		migrated += tag.RowsAffected()
	}
	return migrated, nil
}

func (r *PostgresRepository) Register(
	ctx context.Context,
	courierCode string,
	waybillHash string,
	waybill string,
	waybillMasked string,
	providerContextCiphertext []byte,
) (Shipment, error) {
	return r.register(
		ctx,
		courierCode,
		waybillHash,
		waybill,
		waybillMasked,
		providerContextCiphertext,
		true,
	)
}

func (r *PostgresRepository) RegisterImmediate(
	ctx context.Context,
	courierCode string,
	waybillHash string,
	waybill string,
	waybillMasked string,
	providerContextCiphertext []byte,
) (Shipment, error) {
	return r.register(
		ctx,
		courierCode,
		waybillHash,
		waybill,
		waybillMasked,
		providerContextCiphertext,
		false,
	)
}

func (r *PostgresRepository) register(
	ctx context.Context,
	courierCode string,
	waybillHash string,
	waybill string,
	waybillMasked string,
	providerContextCiphertext []byte,
	enqueue bool,
) (Shipment, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Shipment{}, fmt.Errorf("begin register tracking shipment: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	identity, _ := tenancy.FromContext(ctx)
	providerCredentialID := identity.ProviderCredentialID
	if identity.TenantID != "" && providerCredentialID == "" {
		err := tx.QueryRow(ctx, `
			SELECT coalesce(credential_id::text, '')
			FROM tenant_active_shipping_providers
			WHERE tenant_id = $1
		`, identity.TenantID).Scan(&providerCredentialID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return Shipment{}, fmt.Errorf("resolve tracking provider credential: %w", err)
		}
	}

	var shipmentID string
	err = tx.QueryRow(ctx, `
		INSERT INTO tracking_shipments (
			tenant_id,
			provider_credential_id,
			courier_code,
			waybill_hash,
			waybill,
			waybill_masked,
			provider_context_ciphertext
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (tenant_id, courier_code, waybill_hash) DO UPDATE
		SET waybill = EXCLUDED.waybill,
		    waybill_masked = EXCLUDED.waybill_masked,
		    waybill_ciphertext = NULL,
		    polling_enabled = true,
		    provider_credential_id = CASE
		        WHEN EXCLUDED.provider_credential_id <> ''
		        THEN EXCLUDED.provider_credential_id
		        ELSE tracking_shipments.provider_credential_id
		    END,
		    provider_context_ciphertext = coalesce(
		        EXCLUDED.provider_context_ciphertext,
		        tracking_shipments.provider_context_ciphertext
		    ),
		    updated_at = now()
		RETURNING id::text
	`,
		identity.TenantID,
		providerCredentialID,
		courierCode,
		waybillHash,
		waybill,
		waybillMasked,
		providerContextCiphertext,
	).Scan(&shipmentID)
	if err != nil {
		return Shipment{}, fmt.Errorf("upsert tracking shipment: %w", err)
	}

	refreshQueued := false
	if enqueue {
		tag, err := tx.Exec(ctx, `
			INSERT INTO tracking_refresh_jobs (shipment_id)
			SELECT id
			FROM tracking_shipments
			WHERE id = $1::uuid
			  AND NOT is_final
			  AND validation_status <> 'invalid'
			  AND provider_hit_count < provider_hit_limit
			  AND (next_refresh_at IS NULL OR next_refresh_at <= now())
			ON CONFLICT (shipment_id) WHERE status IN ('pending', 'running') DO NOTHING
		`, shipmentID)
		if err != nil {
			return Shipment{}, fmt.Errorf("enqueue tracking refresh: %w", err)
		}
		refreshQueued = tag.RowsAffected() == 1
	}
	if err := tx.Commit(ctx); err != nil {
		return Shipment{}, fmt.Errorf("commit register tracking shipment: %w", err)
	}
	shipment, err := r.getShipment(ctx, shipmentID)
	if err != nil {
		return Shipment{}, err
	}
	shipment.RefreshQueued = refreshQueued || shipment.RefreshQueued
	return shipment, nil
}

func (r *PostgresRepository) Claim(
	ctx context.Context,
	workerID string,
	courierCodes []string,
) (Job, error) {
	if len(courierCodes) == 0 {
		return Job{}, ErrNoJobAvailable
	}
	var job Job
	err := r.pool.QueryRow(ctx, `
		WITH candidate AS (
			SELECT job.id
			FROM tracking_refresh_jobs job
			JOIN tracking_shipments shipment ON shipment.id = job.shipment_id
			WHERE (
				(job.status = 'pending' AND job.available_at <= now()) OR
				(job.status = 'running' AND job.locked_at < now() - interval '5 minutes')
			)
			  AND shipment.polling_enabled
			  AND shipment.courier_code = ANY($2::text[])
			ORDER BY job.priority, job.available_at, job.created_at
			FOR UPDATE OF job SKIP LOCKED
			LIMIT 1
		)
		UPDATE tracking_refresh_jobs job
		SET status = 'running',
		    locked_at = now(),
		    locked_by = $1,
		    updated_at = now()
		FROM candidate, tracking_shipments shipment
		WHERE job.id = candidate.id
		  AND shipment.id = job.shipment_id
		RETURNING
			job.id::text,
			job.shipment_id::text,
			shipment.tenant_id,
			shipment.provider_credential_id,
			shipment.courier_code,
			coalesce(shipment.waybill, ''),
			shipment.waybill_ciphertext,
			shipment.provider_context_ciphertext,
			job.attempt_count,
			job.max_attempts,
			shipment.not_found_count,
			shipment.provider_hit_count,
			shipment.provider_hit_limit
	`, workerID, courierCodes).Scan(
		&job.ID,
		&job.ShipmentID,
		&job.TenantID,
		&job.ProviderCredentialID,
		&job.CourierCode,
		&job.Waybill,
		&job.WaybillCiphertext,
		&job.ProviderContextCiphertext,
		&job.AttemptCount,
		&job.MaxAttempts,
		&job.NotFoundCount,
		&job.ProviderHitCount,
		&job.ProviderHitLimit,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNoJobAvailable
	}
	if err != nil {
		return Job{}, fmt.Errorf("claim tracking refresh job: %w", err)
	}
	return job, nil
}

func (r *PostgresRepository) Complete(
	ctx context.Context,
	job Job,
	result Result,
) error {
	summaryJSON, err := json.Marshal(result.Summary)
	if err != nil {
		return fmt.Errorf("encode tracking summary: %w", err)
	}
	eventsJSON, err := json.Marshal(result.Events)
	if err != nil {
		return fmt.Errorf("encode tracking events: %w", err)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin complete tracking refresh: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var previousStatus, previousValidation string
	if err := tx.QueryRow(ctx, `
		SELECT normalized_status, validation_status
		FROM tracking_shipments
		WHERE id = $1::uuid
		FOR UPDATE
	`, job.ShipmentID).Scan(&previousStatus, &previousValidation); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("lock tracking shipment: %w", err)
	}

	tag, err := tx.Exec(ctx, `
		UPDATE tracking_shipments
		SET normalized_status = $2,
		    status_label = $3,
		    summary_json = $4::jsonb,
		    events_json = $5::jsonb,
		    provider_code = $6,
		    provider_fetched_at = $7,
		    next_refresh_at = $8,
		    is_final = $9,
		    validation_status = 'valid',
		    validation_checked_at = $7,
		    not_found_count = 0,
		    provider_hit_count = provider_hit_count + 1,
		    status_changed_at = CASE
		        WHEN normalized_status <> $2 THEN $7
		        ELSE status_changed_at
		    END,
		    last_error_code = NULL,
		    updated_at = now()
		WHERE id = $1::uuid
	`,
		job.ShipmentID,
		result.NormalizedStatus,
		result.StatusLabel,
		string(summaryJSON),
		string(eventsJSON),
		result.ProviderCode,
		result.FetchedAt,
		result.NextRefreshAt,
		result.IsFinal,
	)
	if err != nil {
		return fmt.Errorf("update tracking shipment: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	if previousStatus != result.NormalizedStatus {
		if _, err := tx.Exec(ctx, `
			INSERT INTO tracking_status_history (
				shipment_id, normalized_status, status_label,
				provider_code, provider_fetched_at
			)
			VALUES ($1::uuid, $2, $3, $4, $5)
		`, job.ShipmentID, result.NormalizedStatus, result.StatusLabel,
			result.ProviderCode, result.FetchedAt); err != nil {
			return fmt.Errorf("insert tracking status history: %w", err)
		}
	}
	if eventType := changedTrackingEvent(previousStatus, previousValidation, result); eventType != "" {
		if err := enqueueTrackingWebhook(ctx, tx, job.ShipmentID, eventType); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE tracking_refresh_jobs
		SET status = 'completed',
		    locked_at = NULL,
		    locked_by = NULL,
		    updated_at = now()
		WHERE id = $1::uuid
	`, job.ID); err != nil {
		return fmt.Errorf("complete tracking refresh job: %w", err)
	}
	if !result.IsFinal && result.NextRefreshAt != nil {
		if _, err := tx.Exec(ctx, `
			INSERT INTO tracking_refresh_jobs (shipment_id, available_at)
			SELECT id, $2
			FROM tracking_shipments
			WHERE id = $1::uuid AND polling_enabled
			ON CONFLICT (shipment_id) WHERE status IN ('pending', 'running') DO NOTHING
		`, job.ShipmentID, result.NextRefreshAt); err != nil {
			return fmt.Errorf("schedule next tracking refresh: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tracking refresh: %w", err)
	}
	return nil
}

func (r *PostgresRepository) CompleteImmediate(
	ctx context.Context,
	shipmentID string,
	result Result,
) error {
	summaryJSON, err := json.Marshal(result.Summary)
	if err != nil {
		return fmt.Errorf("encode immediate tracking summary: %w", err)
	}
	eventsJSON, err := json.Marshal(result.Events)
	if err != nil {
		return fmt.Errorf("encode immediate tracking events: %w", err)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin immediate tracking completion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var previousStatus, previousValidation string
	if err := tx.QueryRow(ctx, `
		SELECT normalized_status, validation_status
		FROM tracking_shipments
		WHERE id = $1::uuid
		FOR UPDATE
	`, shipmentID).Scan(&previousStatus, &previousValidation); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("lock immediate tracking shipment: %w", err)
	}

	tag, err := tx.Exec(ctx, `
		UPDATE tracking_shipments
		SET normalized_status = $2,
		    status_label = $3,
		    summary_json = $4::jsonb,
		    events_json = $5::jsonb,
		    provider_code = $6,
		    provider_fetched_at = $7,
		    next_refresh_at = $8,
		    is_final = $9,
		    validation_status = 'valid',
		    validation_checked_at = $7,
		    not_found_count = 0,
		    provider_hit_count = provider_hit_count + 1,
		    status_changed_at = CASE
		        WHEN normalized_status <> $2 THEN $7
		        ELSE status_changed_at
		    END,
		    last_error_code = NULL,
		    updated_at = now()
		WHERE id = $1::uuid
	`,
		shipmentID,
		result.NormalizedStatus,
		result.StatusLabel,
		string(summaryJSON),
		string(eventsJSON),
		result.ProviderCode,
		result.FetchedAt,
		result.NextRefreshAt,
		result.IsFinal,
	)
	if err != nil {
		return fmt.Errorf("update immediate tracking shipment: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	if previousStatus != result.NormalizedStatus {
		if _, err := tx.Exec(ctx, `
			INSERT INTO tracking_status_history (
				shipment_id, normalized_status, status_label,
				provider_code, provider_fetched_at
			)
			VALUES ($1::uuid, $2, $3, $4, $5)
		`, shipmentID, result.NormalizedStatus, result.StatusLabel,
			result.ProviderCode, result.FetchedAt); err != nil {
			return fmt.Errorf("insert immediate tracking status history: %w", err)
		}
	}
	if eventType := changedTrackingEvent(previousStatus, previousValidation, result); eventType != "" {
		if err := enqueueTrackingWebhook(ctx, tx, shipmentID, eventType); err != nil {
			return err
		}
	}

	completedJobs, err := tx.Exec(ctx, `
		UPDATE tracking_refresh_jobs
		SET status = 'completed',
		    locked_at = NULL,
		    locked_by = NULL,
		    updated_at = now()
		WHERE shipment_id = $1::uuid
		  AND status = 'pending'
	`, shipmentID)
	if err != nil {
		return fmt.Errorf("complete pending tracking refresh: %w", err)
	}
	if completedJobs.RowsAffected() > 0 &&
		!result.IsFinal &&
		result.NextRefreshAt != nil {
		if _, err := tx.Exec(ctx, `
			INSERT INTO tracking_refresh_jobs (shipment_id, available_at)
			SELECT id, $2
			FROM tracking_shipments
			WHERE id = $1::uuid AND polling_enabled
			ON CONFLICT (shipment_id) WHERE status IN ('pending', 'running') DO NOTHING
		`, shipmentID, result.NextRefreshAt); err != nil {
			return fmt.Errorf("schedule immediate tracking refresh: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit immediate tracking completion: %w", err)
	}
	return nil
}

func (r *PostgresRepository) RecordNotFound(
	ctx context.Context,
	job Job,
	fetchedAt time.Time,
	nextRefreshAt *time.Time,
	invalid bool,
) error {
	return r.recordNotFound(ctx, job.ShipmentID, job.ID, fetchedAt, nextRefreshAt, invalid)
}

func (r *PostgresRepository) RecordNotFoundImmediate(
	ctx context.Context,
	shipmentID string,
	fetchedAt time.Time,
	nextRefreshAt *time.Time,
	invalid bool,
) error {
	return r.recordNotFound(ctx, shipmentID, "", fetchedAt, nextRefreshAt, invalid)
}

func (r *PostgresRepository) recordNotFound(
	ctx context.Context,
	shipmentID, jobID string,
	fetchedAt time.Time,
	nextRefreshAt *time.Time,
	invalid bool,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin record tracking not found: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	validationStatus := "not_found"
	if invalid {
		validationStatus = "invalid"
	}
	var previousValidation string
	if err := tx.QueryRow(ctx, `
		SELECT validation_status
		FROM tracking_shipments
		WHERE id = $1::uuid
		FOR UPDATE
	`, shipmentID).Scan(&previousValidation); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("lock not-found tracking shipment: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE tracking_shipments
		SET validation_status = $2,
		    validation_checked_at = $3,
		    not_found_count = not_found_count + 1,
		    provider_hit_count = provider_hit_count + 1,
		    provider_fetched_at = $3,
		    next_refresh_at = $4,
		    last_error_code = 'WAYBILL_NOT_FOUND',
		    updated_at = now()
		WHERE id = $1::uuid
	`, shipmentID, validationStatus, fetchedAt, nextRefreshAt); err != nil {
		return fmt.Errorf("record tracking not found: %w", err)
	}

	if jobID != "" {
		if _, err := tx.Exec(ctx, `
			UPDATE tracking_refresh_jobs
			SET status = 'completed', locked_at = NULL, locked_by = NULL,
			    last_error_code = 'WAYBILL_NOT_FOUND',
			    last_error_message = 'tracking waybill is not available',
			    updated_at = now()
			WHERE id = $1::uuid
		`, jobID); err != nil {
			return fmt.Errorf("complete not-found tracking job: %w", err)
		}
	} else {
		if _, err := tx.Exec(ctx, `
			UPDATE tracking_refresh_jobs
			SET status = 'completed', locked_at = NULL, locked_by = NULL,
			    last_error_code = 'WAYBILL_NOT_FOUND', updated_at = now()
			WHERE shipment_id = $1::uuid AND status IN ('pending', 'running')
		`, shipmentID); err != nil {
			return fmt.Errorf("complete immediate not-found tracking job: %w", err)
		}
	}
	if !invalid && nextRefreshAt != nil {
		if _, err := tx.Exec(ctx, `
			INSERT INTO tracking_refresh_jobs (shipment_id, available_at)
			SELECT id, $2
			FROM tracking_shipments
			WHERE id = $1::uuid
			  AND polling_enabled
			  AND provider_hit_count < provider_hit_limit
			ON CONFLICT (shipment_id) WHERE status IN ('pending', 'running') DO NOTHING
		`, shipmentID, nextRefreshAt); err != nil {
			return fmt.Errorf("schedule not-found recheck: %w", err)
		}
	}
	if invalid && previousValidation != "invalid" {
		if err := enqueueTrackingWebhook(ctx, tx, shipmentID, "tracking.invalid"); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tracking not found: %w", err)
	}
	return nil
}

func (r *PostgresRepository) RecordImmediateFailure(
	ctx context.Context,
	shipmentID, errorCode string,
	retryAt time.Time,
	countProviderHit bool,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin record immediate tracking failure: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	delta := boolToInt(countProviderHit)
	tag, err := tx.Exec(ctx, `
		UPDATE tracking_shipments
		SET last_error_code = $2,
		    provider_hit_count = provider_hit_count + $3,
		    next_refresh_at = CASE
		        WHEN provider_hit_count + $3 >= provider_hit_limit THEN NULL
		        ELSE $4
		    END,
		    updated_at = now()
		WHERE id = $1::uuid
	`, shipmentID, errorCode, delta, retryAt)
	if err != nil {
		return fmt.Errorf("record immediate tracking failure: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, `
		UPDATE tracking_refresh_jobs job
		SET status = CASE
		        WHEN NOT shipment.polling_enabled THEN 'completed'
		        WHEN shipment.provider_hit_count >= shipment.provider_hit_limit THEN 'dead'
		        ELSE 'pending'
		    END,
		    available_at = $2,
		    last_error_code = $3,
		    last_error_message = 'tracking provider request failed',
		    updated_at = now()
		FROM tracking_shipments shipment
		WHERE job.shipment_id = shipment.id
		  AND shipment.id = $1::uuid
		  AND job.status = 'pending'
	`, shipmentID, retryAt, errorCode); err != nil {
		return fmt.Errorf("reschedule immediate tracking failure: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit immediate tracking failure: %w", err)
	}
	return nil
}

func (r *PostgresRepository) Fail(
	ctx context.Context,
	job Job,
	errorCode, message string,
	retryAt time.Time,
) error {
	nextAttempt := job.AttemptCount + 1
	countProviderHit := errorCode != "ADAPTER_UNAVAILABLE" &&
		errorCode != "DECRYPTION_FAILED" &&
		errorCode != "PROVIDER_QUOTA_EXHAUSTED"
	nextProviderHits := job.ProviderHitCount
	if countProviderHit {
		nextProviderHits++
	}
	status := "pending"
	if nextAttempt >= job.MaxAttempts ||
		(job.ProviderHitLimit > 0 && nextProviderHits >= job.ProviderHitLimit) {
		status = "dead"
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE tracking_refresh_jobs job
		SET status = CASE WHEN shipment.polling_enabled THEN $2 ELSE 'completed' END,
		    attempt_count = $3,
		    available_at = $4,
		    locked_at = NULL,
		    locked_by = NULL,
		    last_error_code = $5,
		    last_error_message = left($6, 500),
		    updated_at = now()
		FROM tracking_shipments shipment
		WHERE job.id = $1::uuid
		  AND shipment.id = job.shipment_id
	`, job.ID, status, nextAttempt, retryAt, errorCode, message)
	if err != nil {
		return fmt.Errorf("fail tracking refresh job: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	_, _ = r.pool.Exec(ctx, `
		UPDATE tracking_shipments
		SET last_error_code = $2,
		    provider_hit_count = provider_hit_count + $3,
		    next_refresh_at = CASE
		        WHEN NOT polling_enabled OR $4 = 'dead' THEN NULL
		        ELSE $5
		    END,
		    updated_at = now()
		WHERE id = $1::uuid
	`, job.ShipmentID, errorCode, boolToInt(countProviderHit), status, retryAt)
	return nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (r *PostgresRepository) getShipment(ctx context.Context, id string) (Shipment, error) {
	var shipment Shipment
	var summaryJSON, eventsJSON []byte
	err := r.pool.QueryRow(ctx, `
		SELECT
			shipment.id::text,
			shipment.courier_code,
			coalesce(shipment.waybill, shipment.waybill_masked),
			shipment.normalized_status,
			coalesce(shipment.status_label, ''),
			shipment.summary_json,
			shipment.events_json,
			coalesce(shipment.provider_code, ''),
			shipment.provider_fetched_at,
			shipment.next_refresh_at,
			shipment.is_final,
			coalesce(shipment.last_error_code, ''),
			shipment.validation_status,
			shipment.validation_checked_at,
			shipment.not_found_count,
			shipment.provider_hit_count,
			shipment.provider_hit_limit,
			(
				NOT shipment.polling_enabled OR
				shipment.is_final OR
				shipment.validation_status = 'invalid' OR
				shipment.provider_hit_count >= shipment.provider_hit_limit
			),
			EXISTS (
				SELECT 1
				FROM tracking_refresh_jobs job
				WHERE job.shipment_id = shipment.id
				  AND job.status IN ('pending', 'running')
			)
		FROM tracking_shipments shipment
		WHERE shipment.id = $1::uuid
	`, id).Scan(
		&shipment.ID,
		&shipment.CourierCode,
		&shipment.WaybillMasked,
		&shipment.NormalizedStatus,
		&shipment.StatusLabel,
		&summaryJSON,
		&eventsJSON,
		&shipment.ProviderCode,
		&shipment.ProviderFetchedAt,
		&shipment.NextRefreshAt,
		&shipment.IsFinal,
		&shipment.LastErrorCode,
		&shipment.ValidationStatus,
		&shipment.ValidationChecked,
		&shipment.NotFoundCount,
		&shipment.ProviderHitCount,
		&shipment.ProviderHitLimit,
		&shipment.PollingStopped,
		&shipment.RefreshQueued,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Shipment{}, ErrNotFound
	}
	if err != nil {
		return Shipment{}, fmt.Errorf("get tracking shipment: %w", err)
	}
	if err := json.Unmarshal(summaryJSON, &shipment.Summary); err != nil {
		return Shipment{}, fmt.Errorf("decode tracking summary: %w", err)
	}
	if err := json.Unmarshal(eventsJSON, &shipment.Events); err != nil {
		return Shipment{}, fmt.Errorf("decode tracking events: %w", err)
	}
	return shipment, nil
}

func (r *PostgresRepository) UpsertSubscription(
	ctx context.Context,
	shipmentID, orderReference, fulfillmentReference string,
	expectedRevision int,
) (Subscription, error) {
	identity, ok := tenancy.FromContext(ctx)
	if !ok {
		return Subscription{}, ErrNotFound
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Subscription{}, fmt.Errorf("begin upsert tracking subscription: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var subscription Subscription
	var currentShipmentID string
	var legacyDomainID string
	err = tx.QueryRow(ctx, `
		SELECT id::text, order_reference, fulfillment_reference, domain_id,
		       active, revision, shipment_id::text
		FROM tracking_subscriptions
		WHERE tenant_id = $1 AND fulfillment_reference = $2
		FOR UPDATE
	`, identity.TenantID, fulfillmentReference).Scan(
		&subscription.ID, &subscription.OrderReference,
		&subscription.FulfillmentReference, &legacyDomainID,
		&subscription.Active, &subscription.Revision, &currentShipmentID,
	)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if expectedRevision > 0 {
			return Subscription{}, ErrRevisionConflict
		}
		err = tx.QueryRow(ctx, `
			INSERT INTO tracking_subscriptions (
				tenant_id, domain_id, order_reference,
				fulfillment_reference, shipment_id, revision
			)
			VALUES ($1, $2, $3, $4, $5::uuid, 1)
			RETURNING id::text, order_reference, fulfillment_reference,
			          active, revision
		`, identity.TenantID, "", orderReference,
			fulfillmentReference, shipmentID).Scan(
			&subscription.ID, &subscription.OrderReference,
			&subscription.FulfillmentReference,
			&subscription.Active, &subscription.Revision,
		)
		if err != nil {
			return Subscription{}, fmt.Errorf("insert tracking subscription: %w", err)
		}
	case err != nil:
		return Subscription{}, fmt.Errorf("lock tracking subscription: %w", err)
	case !subscription.Active:
		if expectedRevision > 0 && expectedRevision != subscription.Revision {
			return Subscription{}, ErrRevisionConflict
		}
		if currentShipmentID != shipmentID {
			if _, err := tx.Exec(ctx, `
				INSERT INTO tracking_subscription_revisions (
					subscription_id, tenant_id, domain_id, order_reference,
					fulfillment_reference, shipment_id, revision, replacement_reason
				)
				VALUES ($1::uuid, $2, $3, $4, $5, $6::uuid, $7, 'reactivated_after_removal')
			`, subscription.ID, identity.TenantID, legacyDomainID,
				subscription.OrderReference, subscription.FulfillmentReference,
				currentShipmentID, subscription.Revision); err != nil {
				return Subscription{}, fmt.Errorf("archive removed tracking subscription revision: %w", err)
			}
		}
		err = tx.QueryRow(ctx, `
			UPDATE tracking_subscriptions
			SET domain_id = '', order_reference = $2, shipment_id = $3::uuid,
			    active = true,
			    revision = revision + CASE WHEN shipment_id <> $3::uuid THEN 1 ELSE 0 END,
			    updated_at = now()
			WHERE id = $1::uuid
			RETURNING order_reference, active, revision
		`, subscription.ID, orderReference, shipmentID).Scan(
			&subscription.OrderReference, &subscription.Active, &subscription.Revision,
		)
		if err != nil {
			return Subscription{}, fmt.Errorf("reactivate tracking subscription: %w", err)
		}
	case currentShipmentID == shipmentID:
		if expectedRevision > 0 && expectedRevision != subscription.Revision {
			return Subscription{}, ErrRevisionConflict
		}
		err = tx.QueryRow(ctx, `
			UPDATE tracking_subscriptions
			SET domain_id = '', order_reference = $2, active = true, updated_at = now()
			WHERE id = $1::uuid
			RETURNING order_reference, active, revision
		`, subscription.ID, orderReference).Scan(
			&subscription.OrderReference, &subscription.Active, &subscription.Revision,
		)
		if err != nil {
			return Subscription{}, fmt.Errorf("refresh tracking subscription: %w", err)
		}
	default:
		// A changed AWB must use the verified replacement endpoint and include
		// the revision read by Emisell. POST retries cannot silently overwrite it.
		if expectedRevision <= 0 || expectedRevision != subscription.Revision {
			return Subscription{}, ErrRevisionConflict
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO tracking_subscription_revisions (
				subscription_id, tenant_id, domain_id, order_reference,
				fulfillment_reference, shipment_id, revision
			)
			VALUES ($1::uuid, $2, $3, $4, $5, $6::uuid, $7)
		`, subscription.ID, identity.TenantID, legacyDomainID,
			subscription.OrderReference, subscription.FulfillmentReference,
			currentShipmentID, subscription.Revision); err != nil {
			return Subscription{}, fmt.Errorf("archive tracking subscription revision: %w", err)
		}
		err = tx.QueryRow(ctx, `
			UPDATE tracking_subscriptions
			SET domain_id = '', order_reference = $2, shipment_id = $3::uuid,
			    active = true, revision = revision + 1, updated_at = now()
			WHERE id = $1::uuid
			RETURNING order_reference, active, revision
		`, subscription.ID, orderReference, shipmentID).Scan(
			&subscription.OrderReference, &subscription.Active, &subscription.Revision,
		)
		if err != nil {
			return Subscription{}, fmt.Errorf("replace tracking subscription: %w", err)
		}
	}
	var normalizedStatus, validationStatus string
	if err := tx.QueryRow(ctx, `
		SELECT normalized_status, validation_status
		FROM tracking_shipments
		WHERE id = $1::uuid
	`, shipmentID).Scan(&normalizedStatus, &validationStatus); err != nil {
		return Subscription{}, fmt.Errorf("read subscribed tracking shipment: %w", err)
	}
	if eventType := currentTrackingEvent(normalizedStatus, validationStatus); eventType != "" {
		if err := enqueueTrackingWebhook(ctx, tx, shipmentID, eventType); err != nil {
			return Subscription{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Subscription{}, fmt.Errorf("commit tracking subscription: %w", err)
	}
	shipment, err := r.getShipment(ctx, shipmentID)
	if err != nil {
		return Subscription{}, err
	}
	subscription.Shipment = shipment
	return subscription, nil
}

func currentTrackingEvent(normalizedStatus, validationStatus string) string {
	if validationStatus == "invalid" {
		return "tracking.invalid"
	}
	if validationStatus != "valid" {
		return ""
	}
	if normalizedStatus == "delivered" {
		return "tracking.delivered"
	}
	if normalizedStatus != "unknown" {
		return "tracking.status_changed"
	}
	return "tracking.validated"
}

func (r *PostgresRepository) GetSubscription(
	ctx context.Context,
	fulfillmentReference string,
) (Subscription, error) {
	identity, ok := tenancy.FromContext(ctx)
	if !ok {
		return Subscription{}, ErrNotFound
	}
	var subscription Subscription
	var shipmentID string
	err := r.pool.QueryRow(ctx, `
		SELECT id::text, order_reference, fulfillment_reference,
		       active, revision, shipment_id::text
		FROM tracking_subscriptions
		WHERE tenant_id = $1
		  AND fulfillment_reference = $2
		  AND active
	`, identity.TenantID, fulfillmentReference).Scan(
		&subscription.ID,
		&subscription.OrderReference,
		&subscription.FulfillmentReference,
		&subscription.Active,
		&subscription.Revision,
		&shipmentID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Subscription{}, ErrNotFound
	}
	if err != nil {
		return Subscription{}, fmt.Errorf("get tracking subscription: %w", err)
	}
	shipment, err := r.getShipment(ctx, shipmentID)
	if err != nil {
		return Subscription{}, err
	}
	subscription.Shipment = shipment
	return subscription, nil
}

func (r *PostgresRepository) DeactivateSubscription(
	ctx context.Context,
	fulfillmentReference string,
) (SubscriptionRemoval, error) {
	identity, ok := tenancy.FromContext(ctx)
	if !ok {
		return SubscriptionRemoval{}, ErrNotFound
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return SubscriptionRemoval{}, fmt.Errorf("begin deactivate tracking subscription: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var removal SubscriptionRemoval
	var shipmentID string
	err = tx.QueryRow(ctx, `
		SELECT id::text, fulfillment_reference, active, revision, shipment_id::text
		FROM tracking_subscriptions
		WHERE tenant_id = $1 AND fulfillment_reference = $2
		FOR UPDATE
	`, identity.TenantID, fulfillmentReference).Scan(
		&removal.ID,
		&removal.FulfillmentReference,
		&removal.Active,
		&removal.Revision,
		&shipmentID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return SubscriptionRemoval{}, ErrNotFound
	}
	if err != nil {
		return SubscriptionRemoval{}, fmt.Errorf("lock tracking subscription for deactivation: %w", err)
	}

	if removal.Active {
		if _, err := tx.Exec(ctx, `
			UPDATE tracking_subscriptions
			SET active = false, updated_at = now()
			WHERE id = $1::uuid
		`, removal.ID); err != nil {
			return SubscriptionRemoval{}, fmt.Errorf("deactivate tracking subscription: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE tracking_webhook_outbox
		SET status = 'dead',
		    locked_at = NULL,
		    locked_by = NULL,
		    last_error_message = 'tracking subscription removed by merchant',
		    updated_at = now()
		WHERE subscription_id = $1::uuid
		  AND status IN ('pending', 'running')
	`, removal.ID); err != nil {
		return SubscriptionRemoval{}, fmt.Errorf("cancel tracking subscription webhooks: %w", err)
	}

	var activeSubscriptions int
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM tracking_subscriptions
		WHERE shipment_id = $1::uuid AND active
	`, shipmentID).Scan(&activeSubscriptions); err != nil {
		return SubscriptionRemoval{}, fmt.Errorf("count active tracking subscriptions: %w", err)
	}
	removal.PollingStopped = activeSubscriptions == 0
	if removal.PollingStopped {
		if _, err := tx.Exec(ctx, `
			UPDATE tracking_shipments
			SET polling_enabled = false, next_refresh_at = NULL, updated_at = now()
			WHERE id = $1::uuid
		`, shipmentID); err != nil {
			return SubscriptionRemoval{}, fmt.Errorf("stop tracking shipment polling: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE tracking_refresh_jobs
			SET status = 'completed',
			    locked_at = NULL,
			    locked_by = NULL,
			    last_error_code = 'SUBSCRIPTION_REMOVED',
			    last_error_message = 'tracking subscription removed by merchant',
			    updated_at = now()
			WHERE shipment_id = $1::uuid
			  AND status IN ('pending', 'running')
		`, shipmentID); err != nil {
			return SubscriptionRemoval{}, fmt.Errorf("cancel tracking refresh jobs: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return SubscriptionRemoval{}, fmt.Errorf("commit tracking subscription deactivation: %w", err)
	}
	removal.Active = false
	removal.Status = "removed"
	removal.SnapshotRetained = true
	return removal, nil
}

func changedTrackingEvent(previousStatus, previousValidation string, result Result) string {
	if result.NormalizedStatus == "delivered" && previousStatus != "delivered" {
		return "tracking.delivered"
	}
	if previousStatus != result.NormalizedStatus {
		return "tracking.status_changed"
	}
	if previousValidation != "valid" {
		return "tracking.validated"
	}
	return ""
}

func enqueueTrackingWebhook(
	ctx context.Context,
	tx pgx.Tx,
	shipmentID, eventType string,
) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO tracking_webhook_outbox (
			subscription_id, shipment_id, tenant_id, event_type,
			deduplication_key, data_json
		)
		SELECT
			subscription.id,
			shipment.id,
			subscription.tenant_id,
			$2,
			subscription.id::text || ':' || $2 || ':' || coalesce(
				shipment.provider_fetched_at::text,
				shipment.validation_checked_at::text,
				shipment.updated_at::text
			),
			jsonb_build_object(
				'merchant_id', subscription.tenant_id,
				'order_id', subscription.order_reference,
				'fulfillment_id', subscription.fulfillment_reference,
				'tracking_revision', subscription.revision,
				'shipment', jsonb_build_object(
					'courier', shipment.courier_code,
					'waybill', coalesce(shipment.waybill, shipment.waybill_masked),
					'validation_status', shipment.validation_status,
					'status', shipment.normalized_status,
					'status_label', coalesce(shipment.status_label, ''),
					'provider', coalesce(shipment.provider_code, ''),
					'provider_fetched_at', shipment.provider_fetched_at,
					'next_refresh_at', shipment.next_refresh_at,
					'is_final', shipment.is_final
				)
			)
		FROM tracking_subscriptions subscription
		JOIN tracking_shipments shipment ON shipment.id = subscription.shipment_id
		WHERE shipment.id = $1::uuid
		  AND subscription.active
		ON CONFLICT (deduplication_key) DO NOTHING
	`, shipmentID, eventType)
	if err != nil {
		return fmt.Errorf("enqueue tracking webhook: %w", err)
	}
	return nil
}

func (r *PostgresRepository) ClaimWebhook(
	ctx context.Context,
	workerID string,
) (WebhookJob, error) {
	var job WebhookJob
	var dataJSON []byte
	err := r.pool.QueryRow(ctx, `
		WITH candidate AS (
			SELECT id
			FROM tracking_webhook_outbox
			WHERE (status = 'pending' AND available_at <= now())
			   OR (status = 'running' AND locked_at < now() - interval '5 minutes')
			ORDER BY available_at, created_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE tracking_webhook_outbox outbox
		SET status = 'running', locked_at = now(), locked_by = $1, updated_at = now()
		FROM candidate
		WHERE outbox.id = candidate.id
		RETURNING outbox.id::text, outbox.event_type, outbox.data_json,
		          outbox.attempt_count, outbox.max_attempts, outbox.created_at
	`, workerID).Scan(
		&job.ID,
		&job.EventType,
		&dataJSON,
		&job.AttemptCount,
		&job.MaxAttempts,
		&job.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return WebhookJob{}, ErrNoJobAvailable
	}
	if err != nil {
		return WebhookJob{}, fmt.Errorf("claim tracking webhook: %w", err)
	}
	if err := json.Unmarshal(dataJSON, &job.Data); err != nil {
		return WebhookJob{}, fmt.Errorf("decode tracking webhook data: %w", err)
	}
	return job, nil
}

func (r *PostgresRepository) CompleteWebhook(
	ctx context.Context,
	jobID string,
	httpStatus int,
) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE tracking_webhook_outbox
		SET status = 'delivered', attempt_count = attempt_count + 1,
		    last_http_status = $2, delivered_at = now(),
		    locked_at = NULL, locked_by = NULL, updated_at = now()
		WHERE id = $1::uuid
	`, jobID, httpStatus)
	if err != nil {
		return fmt.Errorf("complete tracking webhook: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) FailWebhook(
	ctx context.Context,
	job WebhookJob,
	httpStatus int,
	message string,
	retryAt time.Time,
	retry bool,
) error {
	nextAttempt := job.AttemptCount + 1
	status := "dead"
	if retry && nextAttempt < job.MaxAttempts {
		status = "pending"
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE tracking_webhook_outbox
		SET status = $2, attempt_count = $3, available_at = $4,
		    last_http_status = NULLIF($5, 0),
		    last_error_message = left($6, 500),
		    locked_at = NULL, locked_by = NULL, updated_at = now()
		WHERE id = $1::uuid
	`, job.ID, status, nextAttempt, retryAt, httpStatus, message)
	if err != nil {
		return fmt.Errorf("fail tracking webhook: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}
