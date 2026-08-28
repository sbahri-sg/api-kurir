package fulfillment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/emisell/api-kurir/internal/providercredentials"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool   *pgxpool.Pool
	cipher *providercredentials.Cipher
}

func NewPostgresRepository(pool *pgxpool.Pool, cipher *providercredentials.Cipher) *PostgresRepository {
	return &PostgresRepository{pool: pool, cipher: cipher}
}

func (r *PostgresRepository) SaveQuotes(ctx context.Context, tenantID string, quotes []Quote) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin fulfillment quote storage: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, quote := range quotes {
		_, err = tx.Exec(ctx, `
			INSERT INTO fulfillment_quotes (
				id, tenant_id, provider_code, environment_code, credential_alias,
				provider_quote_id, courier_code, courier_name, service_code,
				native_service_code, service_name, service_group, delivery_mode,
				shipping_cost, shipping_cashback, service_fee, additional_cost,
				grand_total, cod_value, insurance_value, currency, etd,
				binding_hash, expires_at
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
				$13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24
			)
		`, quote.ID, tenantID, quote.ProviderCode, quote.Environment,
			quote.CredentialAlias, quote.ProviderQuoteID, quote.CourierCode,
			quote.CourierName, quote.ServiceCode, quote.NativeServiceCode,
			quote.ServiceName, quote.ServiceGroup, quote.DeliveryMode,
			quote.ShippingCost, quote.ShippingCashback, quote.ServiceFee,
			quote.AdditionalCost, quote.GrandTotal, quote.CODValue,
			quote.InsuranceValue, quote.Currency, quote.ETD, quote.BindingHash,
			quote.ExpiresAt)
		if err != nil {
			return fmt.Errorf("store fulfillment quote: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) GetQuote(ctx context.Context, tenantID, quoteID string) (Quote, error) {
	var quote Quote
	err := r.pool.QueryRow(ctx, `
		SELECT id, provider_code, environment_code, credential_alias,
		       provider_quote_id, courier_code, courier_name, service_code,
		       native_service_code, service_name, service_group, delivery_mode,
		       shipping_cost, shipping_cashback, service_fee, additional_cost,
		       grand_total, cod_value, insurance_value, currency, etd,
		       binding_hash, expires_at
		FROM fulfillment_quotes
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, quoteID).Scan(
		&quote.ID, &quote.ProviderCode, &quote.Environment, &quote.CredentialAlias,
		&quote.ProviderQuoteID, &quote.CourierCode, &quote.CourierName,
		&quote.ServiceCode, &quote.NativeServiceCode, &quote.ServiceName,
		&quote.ServiceGroup, &quote.DeliveryMode, &quote.ShippingCost,
		&quote.ShippingCashback, &quote.ServiceFee, &quote.AdditionalCost,
		&quote.GrandTotal, &quote.CODValue, &quote.InsuranceValue,
		&quote.Currency, &quote.ETD, &quote.BindingHash, &quote.ExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Quote{}, ErrQuoteNotFound
	}
	if err != nil {
		return Quote{}, fmt.Errorf("get fulfillment quote: %w", err)
	}
	return quote, nil
}

func (r *PostgresRepository) ReserveCreate(
	ctx context.Context,
	input ReserveCreateInput,
) (Shipment, bool, error) {
	var existingHash []byte
	var existingKey string
	existing, err := scanShipment(r.pool.QueryRow(ctx, shipmentSelect+`
		WHERE tenant_id = $1
		  AND (create_idempotency_key = $2 OR merchant_reference = $3)
		ORDER BY (create_idempotency_key = $2) DESC
		LIMIT 1
	`, input.TenantID, input.IdempotencyKey, input.MerchantReference), &existingHash, &existingKey)
	if err == nil {
		if existingKey != input.IdempotencyKey || !bytes.Equal(existingHash, input.RequestHash) {
			return Shipment{}, false, ErrIdempotencyConflict
		}
		return existing, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Shipment{}, false, err
	}
	requestCiphertext, err := r.cipher.Encrypt(
		input.RequestCiphertext,
		[]byte("fulfillment-request:"+input.TenantID+":"+input.ID),
	)
	if err != nil {
		return Shipment{}, false, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Shipment{}, false, fmt.Errorf("begin fulfillment reservation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `
		INSERT INTO fulfillment_shipments (
			id, tenant_id, provider_code, merchant_reference, quote_id,
			courier_code, service_code, delivery_mode, fulfillment_mode,
			shipping_cost, currency, package_weight_grams, create_idempotency_key,
			create_request_hash, request_ciphertext
		) VALUES (
			$1::uuid, $2, $3, $4, $5, $6, $7, $8, $9,
			$10, $11, $12, $13, $14, $15
		)
	`, input.ID, input.TenantID, input.ProviderCode, input.MerchantReference,
		input.QuoteID, input.CourierCode, input.ServiceCode, input.DeliveryMode,
		input.Fulfillment, input.ShippingCost, input.Currency,
		input.PackageWeightGrams, input.IdempotencyKey, input.RequestHash, requestCiphertext)
	if err != nil {
		if isUniqueViolation(err) {
			return Shipment{}, false, ErrIdempotencyConflict
		}
		return Shipment{}, false, fmt.Errorf("reserve fulfillment shipment: %w", err)
	}
	if strings.HasPrefix(input.QuoteID, "fq_") {
		tag, consumeErr := tx.Exec(ctx, `
			UPDATE fulfillment_quotes
			SET consumed_by_shipment_id = $3::uuid, consumed_at = now()
			WHERE tenant_id = $1 AND id = $2
			  AND consumed_by_shipment_id IS NULL
			  AND expires_at > now()
		`, input.TenantID, input.QuoteID, input.ID)
		if consumeErr != nil {
			return Shipment{}, false, fmt.Errorf("consume fulfillment quote: %w", consumeErr)
		}
		if tag.RowsAffected() != 1 {
			var consumed bool
			var expired bool
			stateErr := tx.QueryRow(ctx, `
				SELECT consumed_by_shipment_id IS NOT NULL, expires_at <= now()
				FROM fulfillment_quotes
				WHERE tenant_id = $1 AND id = $2
			`, input.TenantID, input.QuoteID).Scan(&consumed, &expired)
			switch {
			case errors.Is(stateErr, pgx.ErrNoRows):
				return Shipment{}, false, ErrQuoteNotFound
			case stateErr != nil:
				return Shipment{}, false, fmt.Errorf("inspect fulfillment quote state: %w", stateErr)
			case expired:
				return Shipment{}, false, ErrQuoteExpired
			case consumed:
				return Shipment{}, false, ErrQuoteConsumed
			default:
				return Shipment{}, false, ErrQuoteMismatch
			}
		}
	}
	if err := insertHistory(ctx, tx, input.TenantID, input.ID, StatusBookingPending, "", "Shipment reserved; booking provider is in progress."); err != nil {
		return Shipment{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Shipment{}, false, fmt.Errorf("commit fulfillment reservation: %w", err)
	}
	shipment, err := r.Get(ctx, input.TenantID, input.ID)
	return shipment, true, err
}

func (r *PostgresRepository) CompleteCreate(
	ctx context.Context,
	tenantID, shipmentID string,
	result ProviderCreateResult,
) (Shipment, error) {
	status := result.Status
	if status == "" {
		status = StatusBooked
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Shipment{}, fmt.Errorf("begin complete fulfillment booking: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `
		UPDATE fulfillment_shipments
		SET provider_shipment_id = $3,
		    awb = nullif($4, ''),
		    normalized_status = $5,
		    provider_status = $6,
		    label_available = ($4 <> ''),
		    tracking_registration_status = CASE WHEN $4 <> '' THEN 'pending' ELSE 'not_ready' END,
		    updated_at = now()
		WHERE tenant_id = $1 AND id = $2::uuid
	`, tenantID, shipmentID, result.ProviderShipmentID, result.AWB, status, result.ProviderStatus)
	if err != nil {
		return Shipment{}, fmt.Errorf("complete fulfillment booking: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return Shipment{}, ErrShipmentNotFound
	}
	if err := insertHistory(ctx, tx, tenantID, shipmentID, status, result.ProviderStatus, "Shipment accepted by provider."); err != nil {
		return Shipment{}, err
	}
	if err := enqueueFulfillmentWebhook(ctx, tx, tenantID, shipmentID, "shipment.booked"); err != nil {
		return Shipment{}, err
	}
	if result.AWB != "" {
		if err := enqueueLifecycleJob(ctx, tx, tenantID, shipmentID, "register_tracking", time.Now().UTC()); err != nil {
			return Shipment{}, err
		}
		if err := enqueueFulfillmentWebhook(ctx, tx, tenantID, shipmentID, "shipment.awb_created"); err != nil {
			return Shipment{}, err
		}
	} else if err := enqueueLifecycleJob(ctx, tx, tenantID, shipmentID, "reconcile", time.Now().UTC().Add(15*time.Minute)); err != nil {
		return Shipment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Shipment{}, fmt.Errorf("commit fulfillment booking: %w", err)
	}
	return r.Get(ctx, tenantID, shipmentID)
}

func (r *PostgresRepository) FailCreate(
	ctx context.Context,
	tenantID, shipmentID, providerStatus string,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `
		UPDATE fulfillment_shipments
		SET normalized_status = 'booking_failed', provider_status = $3, updated_at = now()
		WHERE tenant_id = $1 AND id = $2::uuid
	`, tenantID, shipmentID, providerStatus)
	if err != nil {
		return err
	}
	if err := insertHistory(ctx, tx, tenantID, shipmentID, StatusBookingFailed, providerStatus, "Provider booking failed; use a new idempotency key after correcting the request."); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) Get(
	ctx context.Context,
	tenantID, shipmentID string,
) (Shipment, error) {
	shipment, err := scanShipment(r.pool.QueryRow(ctx, shipmentSelect+`
		WHERE tenant_id = $1 AND id = $2::uuid
	`, tenantID, shipmentID), nil, nil)
	if errors.Is(err, pgx.ErrNoRows) {
		return Shipment{}, ErrShipmentNotFound
	}
	return shipment, err
}

func (r *PostgresRepository) ReserveOperation(
	ctx context.Context,
	input ReserveOperationInput,
) (bool, error) {
	var requestHash []byte
	err := r.pool.QueryRow(ctx, `
		SELECT request_hash
		FROM fulfillment_operations
		WHERE tenant_id = $1 AND operation_type = $2 AND idempotency_key = $3
	`, input.TenantID, input.OperationType, input.IdempotencyKey).Scan(&requestHash)
	if err == nil {
		if !bytes.Equal(requestHash, input.RequestHash) {
			return false, ErrIdempotencyConflict
		}
		return false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("get fulfillment operation: %w", err)
	}
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO fulfillment_operations (
			id, tenant_id, shipment_id, operation_type, idempotency_key, request_hash
		)
		SELECT $1::uuid, $2, shipment.id, $4, $5, $6
		FROM fulfillment_shipments shipment
		WHERE shipment.id = $3::uuid AND shipment.tenant_id = $2
	`, input.ID, input.TenantID, input.ShipmentID, input.OperationType,
		input.IdempotencyKey, input.RequestHash)
	if err != nil {
		if isUniqueViolation(err) {
			return false, ErrIdempotencyConflict
		}
		return false, fmt.Errorf("reserve fulfillment operation: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return false, ErrShipmentNotFound
	}
	return true, nil
}

func (r *PostgresRepository) CompletePickup(
	ctx context.Context,
	tenantID, shipmentID, operationKey string,
	result ProviderPickupResult,
) (Shipment, error) {
	status := result.Status
	if status == "" {
		status = StatusPickupRequested
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Shipment{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := completeOperation(ctx, tx, tenantID, shipmentID, "pickup", operationKey, result.ProviderOperationID, result.ProviderStatus); err != nil {
		return Shipment{}, err
	}
	_, err = tx.Exec(ctx, `
		UPDATE fulfillment_shipments
		SET awb = coalesce(nullif($3, ''), awb),
		    normalized_status = $4,
		    provider_status = $5,
		    label_available = label_available OR ($3 <> ''),
		    updated_at = now()
		WHERE tenant_id = $1 AND id = $2::uuid
	`, tenantID, shipmentID, result.AWB, status, result.ProviderStatus)
	if err != nil {
		return Shipment{}, err
	}
	if err := insertHistory(ctx, tx, tenantID, shipmentID, status, result.ProviderStatus, "Pickup requested from provider."); err != nil {
		return Shipment{}, err
	}
	if err := enqueueFulfillmentWebhook(ctx, tx, tenantID, shipmentID, "shipment.pickup_requested"); err != nil {
		return Shipment{}, err
	}
	if result.AWB != "" {
		if _, err := tx.Exec(ctx, `
			UPDATE fulfillment_shipments
			SET tracking_registration_status = 'pending'
			WHERE tenant_id = $1 AND id = $2::uuid
		`, tenantID, shipmentID); err != nil {
			return Shipment{}, err
		}
		if err := enqueueLifecycleJob(ctx, tx, tenantID, shipmentID, "register_tracking", time.Now().UTC()); err != nil {
			return Shipment{}, err
		}
		if err := enqueueFulfillmentWebhook(ctx, tx, tenantID, shipmentID, "shipment.awb_created"); err != nil {
			return Shipment{}, err
		}
	} else if err := enqueueLifecycleJob(ctx, tx, tenantID, shipmentID, "reconcile", time.Now().UTC().Add(time.Hour)); err != nil {
		return Shipment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Shipment{}, err
	}
	return r.Get(ctx, tenantID, shipmentID)
}

func (r *PostgresRepository) CompleteCancel(
	ctx context.Context,
	tenantID, shipmentID, operationKey string,
	result ProviderCancelResult,
) (Shipment, error) {
	status := result.Status
	if status == "" {
		status = StatusCancellationPending
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Shipment{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := completeOperation(ctx, tx, tenantID, shipmentID, "cancel", operationKey, "", result.ProviderStatus); err != nil {
		return Shipment{}, err
	}
	_, err = tx.Exec(ctx, `
		UPDATE fulfillment_shipments
		SET normalized_status = $3,
		    provider_status = $4,
		    cancelled_at = CASE WHEN $3 = 'cancelled' THEN now() ELSE cancelled_at END,
		    updated_at = now()
		WHERE tenant_id = $1 AND id = $2::uuid
	`, tenantID, shipmentID, status, result.ProviderStatus)
	if err != nil {
		return Shipment{}, err
	}
	if err := insertHistory(ctx, tx, tenantID, shipmentID, status, result.ProviderStatus, "Cancellation requested from provider."); err != nil {
		return Shipment{}, err
	}
	if err := enqueueFulfillmentWebhook(ctx, tx, tenantID, shipmentID, "shipment.cancelled"); err != nil {
		return Shipment{}, err
	}
	if status == StatusCancelled {
		if _, err := tx.Exec(ctx, `
			UPDATE fulfillment_lifecycle_jobs
			SET status = 'completed', locked_at = NULL, locked_by = NULL, updated_at = now()
			WHERE shipment_id = $1::uuid AND status IN ('pending', 'running')
		`, shipmentID); err != nil {
			return Shipment{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Shipment{}, err
	}
	return r.Get(ctx, tenantID, shipmentID)
}

func (r *PostgresRepository) FailOperation(
	ctx context.Context,
	tenantID, shipmentID, operationType, operationKey, providerStatus string,
) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE fulfillment_operations
		SET status = 'failed', provider_status = $5, updated_at = now()
		WHERE tenant_id = $1 AND shipment_id = $2::uuid
		  AND operation_type = $3 AND idempotency_key = $4
	`, tenantID, shipmentID, operationType, operationKey, providerStatus)
	return err
}

func (r *PostgresRepository) History(
	ctx context.Context,
	tenantID, shipmentID string,
) ([]HistoryEvent, error) {
	if _, err := r.Get(ctx, tenantID, shipmentID); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, normalized_status, provider_status, description, occurred_at
		FROM fulfillment_history
		WHERE tenant_id = $1 AND shipment_id = $2::uuid
		ORDER BY occurred_at, id
	`, tenantID, shipmentID)
	if err != nil {
		return nil, fmt.Errorf("list fulfillment history: %w", err)
	}
	defer rows.Close()
	result := make([]HistoryEvent, 0)
	for rows.Next() {
		var event HistoryEvent
		if err := rows.Scan(&event.ID, &event.Status, &event.ProviderStatus, &event.Description, &event.OccurredAt); err != nil {
			return nil, fmt.Errorf("scan fulfillment history: %w", err)
		}
		result = append(result, event)
	}
	return result, rows.Err()
}

func (r *PostgresRepository) GetLabel(
	ctx context.Context,
	tenantID, shipmentID, format string,
) (Label, error) {
	var contentType string
	var ciphertext []byte
	var expiresAt *time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT content_type, label_ciphertext, expires_at
		FROM fulfillment_shipment_labels
		WHERE tenant_id = $1 AND shipment_id = $2::uuid AND format = $3
		  AND (expires_at IS NULL OR expires_at > now())
	`, tenantID, shipmentID, format).Scan(&contentType, &ciphertext, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Label{}, ErrLabelUnavailable
	}
	if err != nil {
		return Label{}, fmt.Errorf("get fulfillment label: %w", err)
	}
	plaintext, err := r.cipher.Decrypt(ciphertext, []byte("fulfillment-label:"+tenantID+":"+shipmentID+":"+format))
	if err != nil {
		return Label{}, err
	}
	var label Label
	if err := json.Unmarshal(plaintext, &label); err != nil {
		return Label{}, fmt.Errorf("decode fulfillment label: %w", err)
	}
	label.ContentType = contentType
	label.ExpiresAt = expiresAt
	return label, nil
}

func (r *PostgresRepository) SaveLabel(
	ctx context.Context,
	tenantID, shipmentID string,
	label Label,
) error {
	payload, err := json.Marshal(label)
	if err != nil {
		return err
	}
	ciphertext, err := r.cipher.Encrypt(payload, []byte("fulfillment-label:"+tenantID+":"+shipmentID+":"+label.Format))
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO fulfillment_shipment_labels (
			shipment_id, tenant_id, format, content_type, label_ciphertext, expires_at
		) VALUES ($1::uuid, $2, $3, $4, $5, $6)
		ON CONFLICT (shipment_id, format) DO UPDATE
		SET content_type = EXCLUDED.content_type,
		    label_ciphertext = EXCLUDED.label_ciphertext,
		    expires_at = EXCLUDED.expires_at,
		    updated_at = now()
	`, shipmentID, tenantID, label.Format, label.ContentType, ciphertext, label.ExpiresAt)
	return err
}

func (r *PostgresRepository) ClaimLifecycleJob(
	ctx context.Context,
	workerID string,
) (LifecycleJob, error) {
	var job LifecycleJob
	err := r.pool.QueryRow(ctx, `
		WITH candidate AS (
			SELECT id
			FROM fulfillment_lifecycle_jobs
			WHERE (status = 'pending' AND available_at <= now())
			   OR (status = 'running' AND locked_at < now() - interval '5 minutes')
			ORDER BY available_at, created_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE fulfillment_lifecycle_jobs job
		SET status = 'running', locked_at = now(), locked_by = $1, updated_at = now()
		FROM candidate
		WHERE job.id = candidate.id
		RETURNING job.id::text, job.tenant_id, job.shipment_id::text,
		          job.job_type, job.attempt_count, job.max_attempts
	`, workerID).Scan(
		&job.ID, &job.TenantID, &job.ShipmentID, &job.JobType,
		&job.AttemptCount, &job.MaxAttempts,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return LifecycleJob{}, ErrNoLifecycleJob
	}
	if err != nil {
		return LifecycleJob{}, fmt.Errorf("claim fulfillment lifecycle job: %w", err)
	}
	return job, nil
}

func (r *PostgresRepository) GetLifecycleShipment(
	ctx context.Context,
	tenantID, shipmentID string,
) (Shipment, error) {
	return r.Get(ctx, tenantID, shipmentID)
}

func (r *PostgresRepository) CompleteTrackingRegistration(
	ctx context.Context,
	job LifecycleJob,
	trackingShipmentID string,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tracking registration completion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var shipmentStatus string
	if err := tx.QueryRow(ctx, `
		UPDATE fulfillment_shipments
		SET tracking_registration_status = 'registered',
		    tracking_shipment_id = $3::uuid,
		    reconcile_error = '',
		    updated_at = now()
		WHERE tenant_id = $1 AND id = $2::uuid
		RETURNING normalized_status
	`, job.TenantID, job.ShipmentID, trackingShipmentID).Scan(&shipmentStatus); err != nil {
		return fmt.Errorf("mark fulfillment tracking registered: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE fulfillment_lifecycle_jobs
		SET status = 'completed', locked_at = NULL, locked_by = NULL, updated_at = now()
		WHERE id = $1::uuid
	`, job.ID); err != nil {
		return fmt.Errorf("complete tracking registration job: %w", err)
	}
	if err := insertHistory(
		ctx, tx, job.TenantID, job.ShipmentID,
		shipmentStatus, "tracking_registered",
		"AWB registered in the durable tracking pipeline.",
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) CompleteReconciliation(
	ctx context.Context,
	job LifecycleJob,
	result ProviderDetailResult,
	nextRefreshAt *time.Time,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin fulfillment reconciliation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var previousStatus, previousAWB string
	if err := tx.QueryRow(ctx, `
		SELECT normalized_status, coalesce(awb, '')
		FROM fulfillment_shipments
		WHERE tenant_id = $1 AND id = $2::uuid
		FOR UPDATE
	`, job.TenantID, job.ShipmentID).Scan(&previousStatus, &previousAWB); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrShipmentNotFound
		}
		return fmt.Errorf("lock fulfillment reconciliation shipment: %w", err)
	}
	status := result.Status
	if status == "" {
		status = previousStatus
	}
	if status == StatusDelivered || status == StatusCancelled || result.AWB != "" {
		nextRefreshAt = nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE fulfillment_shipments
		SET awb = coalesce(nullif($3, ''), awb),
		    normalized_status = $4,
		    provider_status = $5,
		    live_tracking_url = $6,
		    last_reconciled_at = now(),
		    next_reconcile_at = $7,
		    reconcile_attempt_count = reconcile_attempt_count + 1,
		    reconcile_error = '',
		    cancelled_at = CASE WHEN $4 = 'cancelled' THEN coalesce(cancelled_at, now()) ELSE cancelled_at END,
		    updated_at = now()
		WHERE tenant_id = $1 AND id = $2::uuid
	`, job.TenantID, job.ShipmentID, result.AWB, status,
		result.ProviderStatus, result.LiveTrackingURL, nextRefreshAt); err != nil {
		return fmt.Errorf("apply fulfillment reconciliation: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE fulfillment_lifecycle_jobs
		SET status = 'completed', locked_at = NULL, locked_by = NULL, updated_at = now()
		WHERE id = $1::uuid
	`, job.ID); err != nil {
		return fmt.Errorf("complete fulfillment reconciliation job: %w", err)
	}
	if previousStatus != status {
		if err := insertHistory(ctx, tx, job.TenantID, job.ShipmentID, status,
			result.ProviderStatus, "Shipment status reconciled from provider."); err != nil {
			return err
		}
		eventType := "shipment.status_changed"
		if status == StatusDelivered {
			eventType = "shipment.delivered"
		} else if status == StatusCancelled {
			eventType = "shipment.cancelled"
		}
		if err := enqueueFulfillmentWebhook(ctx, tx, job.TenantID, job.ShipmentID, eventType); err != nil {
			return err
		}
	}
	awb := previousAWB
	if result.AWB != "" {
		awb = result.AWB
	}
	if previousAWB == "" && awb != "" {
		if _, err := tx.Exec(ctx, `
			UPDATE fulfillment_shipments
			SET tracking_registration_status = 'pending'
			WHERE tenant_id = $1 AND id = $2::uuid
		`, job.TenantID, job.ShipmentID); err != nil {
			return err
		}
		if err := enqueueLifecycleJob(ctx, tx, job.TenantID, job.ShipmentID, "register_tracking", time.Now().UTC()); err != nil {
			return err
		}
		if err := enqueueFulfillmentWebhook(ctx, tx, job.TenantID, job.ShipmentID, "shipment.awb_created"); err != nil {
			return err
		}
	}
	if nextRefreshAt != nil {
		if err := enqueueLifecycleJob(ctx, tx, job.TenantID, job.ShipmentID, "reconcile", *nextRefreshAt); err != nil {
			return err
		}
	}
	if status == StatusDelivered || status == StatusCancelled {
		if _, err := tx.Exec(ctx, `
			UPDATE fulfillment_lifecycle_jobs
			SET status = 'completed', locked_at = NULL, locked_by = NULL, updated_at = now()
			WHERE shipment_id = $1::uuid AND status IN ('pending', 'running')
		`, job.ShipmentID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) FailLifecycleJob(
	ctx context.Context,
	job LifecycleJob,
	errorCode, message string,
	retryAt time.Time,
) error {
	nextAttempt := job.AttemptCount + 1
	status := "pending"
	if nextAttempt >= job.MaxAttempts {
		status = "dead"
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		UPDATE fulfillment_lifecycle_jobs
		SET status = $2, attempt_count = $3, available_at = $4,
		    locked_at = NULL, locked_by = NULL,
		    last_error_code = $5, last_error_message = left($6, 500),
		    updated_at = now()
		WHERE id = $1::uuid
	`, job.ID, status, nextAttempt, retryAt, errorCode, message); err != nil {
		return fmt.Errorf("fail fulfillment lifecycle job: %w", err)
	}
	registrationStatus := "pending"
	if status == "dead" {
		registrationStatus = "failed"
	}
	if _, err := tx.Exec(ctx, `
		UPDATE fulfillment_shipments
		SET reconcile_error = left($3, 500),
		    next_reconcile_at = CASE WHEN $4 = 'dead' THEN NULL ELSE $5::timestamptz END,
		    tracking_registration_status = CASE
		        WHEN $6 = 'register_tracking' THEN $7
		        ELSE tracking_registration_status
		    END,
		    updated_at = now()
		WHERE tenant_id = $1 AND id = $2::uuid
	`, job.TenantID, job.ShipmentID, message, status, retryAt,
		job.JobType, registrationStatus); err != nil {
		return fmt.Errorf("record fulfillment lifecycle failure: %w", err)
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) EnqueueReconciliation(
	ctx context.Context,
	shipmentID string,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var tenantID, status, providerShipmentID string
	if err := tx.QueryRow(ctx, `
		SELECT tenant_id, normalized_status, coalesce(provider_shipment_id, '')
		FROM fulfillment_shipments
		WHERE id = $1::uuid
		FOR UPDATE
	`, shipmentID).Scan(&tenantID, &status, &providerShipmentID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrShipmentNotFound
		}
		return err
	}
	if providerShipmentID == "" || status == StatusDelivered || status == StatusCancelled {
		return ErrShipmentFinal
	}
	if err := enqueueLifecycleJob(ctx, tx, tenantID, shipmentID, "reconcile", time.Now().UTC()); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) ClaimFulfillmentWebhook(
	ctx context.Context,
	workerID string,
) (WebhookJob, error) {
	var job WebhookJob
	var dataJSON []byte
	err := r.pool.QueryRow(ctx, `
		WITH candidate AS (
			SELECT id
			FROM fulfillment_webhook_outbox
			WHERE (status = 'pending' AND available_at <= now())
			   OR (status = 'running' AND locked_at < now() - interval '5 minutes')
			ORDER BY available_at, created_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE fulfillment_webhook_outbox outbox
		SET status = 'running', locked_at = now(), locked_by = $1, updated_at = now()
		FROM candidate
		WHERE outbox.id = candidate.id
		RETURNING outbox.id::text, outbox.event_type, outbox.data_json,
		          outbox.attempt_count, outbox.max_attempts, outbox.created_at
	`, workerID).Scan(&job.ID, &job.EventType, &dataJSON,
		&job.AttemptCount, &job.MaxAttempts, &job.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return WebhookJob{}, ErrNoWebhookJob
	}
	if err != nil {
		return WebhookJob{}, fmt.Errorf("claim fulfillment webhook: %w", err)
	}
	if err := json.Unmarshal(dataJSON, &job.Data); err != nil {
		return WebhookJob{}, fmt.Errorf("decode fulfillment webhook: %w", err)
	}
	return job, nil
}

func (r *PostgresRepository) CompleteFulfillmentWebhook(
	ctx context.Context,
	jobID string,
	httpStatus int,
) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE fulfillment_webhook_outbox
		SET status = 'delivered', attempt_count = attempt_count + 1,
		    last_http_status = $2, delivered_at = now(),
		    locked_at = NULL, locked_by = NULL, updated_at = now()
		WHERE id = $1::uuid
	`, jobID, httpStatus)
	return err
}

func (r *PostgresRepository) FailFulfillmentWebhook(
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
	_, err := r.pool.Exec(ctx, `
		UPDATE fulfillment_webhook_outbox
		SET status = $2, attempt_count = $3, available_at = $4,
		    last_http_status = NULLIF($5, 0),
		    last_error_message = left($6, 500),
		    locked_at = NULL, locked_by = NULL, updated_at = now()
		WHERE id = $1::uuid
	`, job.ID, status, nextAttempt, retryAt, httpStatus, message)
	return err
}

const shipmentSelect = `
	SELECT
		id::text, merchant_reference, provider_code,
		coalesce(provider_shipment_id, ''), quote_id, courier_code,
		service_code, delivery_mode, fulfillment_mode, coalesce(awb, ''),
		normalized_status, provider_status, shipping_cost, currency,
		package_weight_grams,
		label_available, created_at, updated_at, cancelled_at,
		tracking_registration_status, coalesce(tracking_shipment_id::text, ''),
		live_tracking_url, last_reconciled_at, next_reconcile_at,
		reconcile_attempt_count, reconcile_error,
		create_request_hash, create_idempotency_key
	FROM fulfillment_shipments
`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanShipment(row rowScanner, requestHash *[]byte, idempotencyKey *string) (Shipment, error) {
	var shipment Shipment
	var storedHash []byte
	var storedKey string
	err := row.Scan(
		&shipment.ID, &shipment.MerchantReference, &shipment.ProviderCode,
		&shipment.ProviderShipmentID, &shipment.QuoteID, &shipment.CourierCode,
		&shipment.ServiceCode, &shipment.DeliveryMode, &shipment.Fulfillment,
		&shipment.AWB, &shipment.Status, &shipment.ProviderStatus,
		&shipment.ShippingCost, &shipment.Currency, &shipment.PackageWeightGrams,
		&shipment.LabelAvailable,
		&shipment.CreatedAt, &shipment.UpdatedAt, &shipment.CancelledAt,
		&shipment.TrackingRegistrationStatus, &shipment.TrackingShipmentID,
		&shipment.LiveTrackingURL, &shipment.LastReconciledAt,
		&shipment.NextReconcileAt, &shipment.ReconcileAttemptCount,
		&shipment.ReconcileError,
		&storedHash, &storedKey,
	)
	if err != nil {
		return Shipment{}, err
	}
	if requestHash != nil {
		*requestHash = storedHash
	}
	if idempotencyKey != nil {
		*idempotencyKey = storedKey
	}
	return shipment, nil
}

func insertHistory(
	ctx context.Context,
	tx pgx.Tx,
	tenantID, shipmentID, status, providerStatus, description string,
) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO fulfillment_history (
			tenant_id, shipment_id, normalized_status, provider_status, description
		) VALUES ($1, $2::uuid, $3, $4, $5)
	`, tenantID, shipmentID, status, providerStatus, description)
	if err != nil {
		return fmt.Errorf("append fulfillment history: %w", err)
	}
	return nil
}

func enqueueLifecycleJob(
	ctx context.Context,
	tx pgx.Tx,
	tenantID, shipmentID, jobType string,
	availableAt time.Time,
) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO fulfillment_lifecycle_jobs (
			tenant_id, shipment_id, job_type, available_at
		) VALUES ($1, $2::uuid, $3, $4)
		ON CONFLICT (shipment_id, job_type)
			WHERE status IN ('pending', 'running')
		DO UPDATE SET
			available_at = least(fulfillment_lifecycle_jobs.available_at, EXCLUDED.available_at),
			updated_at = now()
	`, tenantID, shipmentID, jobType, availableAt)
	if err != nil {
		return fmt.Errorf("enqueue fulfillment lifecycle job: %w", err)
	}
	return nil
}

func enqueueFulfillmentWebhook(
	ctx context.Context,
	tx pgx.Tx,
	tenantID, shipmentID, eventType string,
) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO fulfillment_webhook_outbox (
			tenant_id, shipment_id, event_type, deduplication_key, data_json
		)
		SELECT
			shipment.tenant_id,
			shipment.id,
			$3,
			shipment.id::text || ':' || $3 || ':' || shipment.updated_at::text,
			jsonb_build_object(
				'merchant_id', shipment.tenant_id,
				'order_id', shipment.merchant_reference,
				'fulfillment_id', shipment.id::text,
				'shipment', jsonb_build_object(
					'shipment_id', shipment.id::text,
					'provider', shipment.provider_code,
					'provider_shipment_id', coalesce(shipment.provider_shipment_id, ''),
					'courier', shipment.courier_code,
					'service', shipment.service_code,
					'delivery_mode', shipment.delivery_mode,
					'fulfillment_mode', shipment.fulfillment_mode,
					'waybill', coalesce(shipment.awb, ''),
					'status', shipment.normalized_status,
					'provider_status', shipment.provider_status,
					'live_tracking_url', shipment.live_tracking_url,
					'updated_at', shipment.updated_at
				)
			)
		FROM fulfillment_shipments shipment
		WHERE shipment.tenant_id = $1 AND shipment.id = $2::uuid
		ON CONFLICT (deduplication_key) DO NOTHING
	`, tenantID, shipmentID, eventType)
	if err != nil {
		return fmt.Errorf("enqueue fulfillment webhook: %w", err)
	}
	return nil
}

func completeOperation(
	ctx context.Context,
	tx pgx.Tx,
	tenantID, shipmentID, operationType, operationKey,
	providerOperationID, providerStatus string,
) error {
	tag, err := tx.Exec(ctx, `
		UPDATE fulfillment_operations
		SET status = 'completed', provider_operation_id = $5,
		    provider_status = $6, updated_at = now()
		WHERE tenant_id = $1 AND shipment_id = $2::uuid
		  AND operation_type = $3 AND idempotency_key = $4
	`, tenantID, shipmentID, operationType, operationKey, providerOperationID, providerStatus)
	if err != nil {
		return fmt.Errorf("complete fulfillment operation: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrOperationInProgress
	}
	return nil
}

func isUniqueViolation(err error) bool {
	type sqlState interface{ SQLState() string }
	var state sqlState
	return errors.As(err, &state) && state.SQLState() == "23505"
}
