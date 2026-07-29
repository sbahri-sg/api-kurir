package tracking

import (
	"context"
	"encoding/json"
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

func (r *PostgresRepository) Register(
	ctx context.Context,
	courierCode string,
	waybillHash string,
	waybillMasked string,
	waybillCiphertext []byte,
	providerContextCiphertext []byte,
) (Shipment, error) {
	return r.register(
		ctx,
		courierCode,
		waybillHash,
		waybillMasked,
		waybillCiphertext,
		providerContextCiphertext,
		true,
	)
}

func (r *PostgresRepository) RegisterImmediate(
	ctx context.Context,
	courierCode string,
	waybillHash string,
	waybillMasked string,
	waybillCiphertext []byte,
	providerContextCiphertext []byte,
) (Shipment, error) {
	return r.register(
		ctx,
		courierCode,
		waybillHash,
		waybillMasked,
		waybillCiphertext,
		providerContextCiphertext,
		false,
	)
}

func (r *PostgresRepository) register(
	ctx context.Context,
	courierCode string,
	waybillHash string,
	waybillMasked string,
	waybillCiphertext []byte,
	providerContextCiphertext []byte,
	enqueue bool,
) (Shipment, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Shipment{}, fmt.Errorf("begin register tracking shipment: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var shipmentID string
	err = tx.QueryRow(ctx, `
		INSERT INTO tracking_shipments (
			courier_code,
			waybill_hash,
			waybill_masked,
			waybill_ciphertext,
			provider_context_ciphertext
		)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (courier_code, waybill_hash) DO UPDATE
		SET waybill_masked = EXCLUDED.waybill_masked,
		    waybill_ciphertext = EXCLUDED.waybill_ciphertext,
		    provider_context_ciphertext = coalesce(
		        EXCLUDED.provider_context_ciphertext,
		        tracking_shipments.provider_context_ciphertext
		    ),
		    updated_at = now()
		RETURNING id::text
	`,
		courierCode,
		waybillHash,
		waybillMasked,
		waybillCiphertext,
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
			WHERE job.status = 'pending'
			  AND job.available_at <= now()
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
			shipment.courier_code,
			shipment.waybill_ciphertext,
			shipment.provider_context_ciphertext,
			job.attempt_count,
			job.max_attempts
	`, workerID, courierCodes).Scan(
		&job.ID,
		&job.ShipmentID,
		&job.CourierCode,
		&job.WaybillCiphertext,
		&job.ProviderContextCiphertext,
		&job.AttemptCount,
		&job.MaxAttempts,
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
			VALUES ($1::uuid, $2)
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
			VALUES ($1::uuid, $2)
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

func (r *PostgresRepository) Fail(
	ctx context.Context,
	job Job,
	errorCode, message string,
	retryAt time.Time,
) error {
	nextAttempt := job.AttemptCount + 1
	status := "pending"
	if nextAttempt >= job.MaxAttempts {
		status = "dead"
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE tracking_refresh_jobs
		SET status = $2,
		    attempt_count = $3,
		    available_at = $4,
		    locked_at = NULL,
		    locked_by = NULL,
		    last_error_code = $5,
		    last_error_message = left($6, 500),
		    updated_at = now()
		WHERE id = $1::uuid
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
		    updated_at = now()
		WHERE id = $1::uuid
	`, job.ShipmentID, errorCode)
	return nil
}

func (r *PostgresRepository) getShipment(ctx context.Context, id string) (Shipment, error) {
	var shipment Shipment
	var summaryJSON, eventsJSON []byte
	err := r.pool.QueryRow(ctx, `
		SELECT
			shipment.id::text,
			shipment.courier_code,
			shipment.waybill_masked,
			shipment.normalized_status,
			coalesce(shipment.status_label, ''),
			shipment.summary_json,
			shipment.events_json,
			coalesce(shipment.provider_code, ''),
			shipment.provider_fetched_at,
			shipment.next_refresh_at,
			shipment.is_final,
			coalesce(shipment.last_error_code, ''),
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
