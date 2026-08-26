DROP TABLE IF EXISTS fulfillment_webhook_outbox;
DROP TABLE IF EXISTS fulfillment_lifecycle_jobs;

DROP INDEX IF EXISTS fulfillment_shipments_reconcile_idx;

ALTER TABLE fulfillment_shipments
    DROP CONSTRAINT IF EXISTS fulfillment_tracking_registration_status_check,
    DROP COLUMN IF EXISTS reconcile_error,
    DROP COLUMN IF EXISTS reconcile_attempt_count,
    DROP COLUMN IF EXISTS next_reconcile_at,
    DROP COLUMN IF EXISTS last_reconciled_at,
    DROP COLUMN IF EXISTS live_tracking_url,
    DROP COLUMN IF EXISTS tracking_shipment_id,
    DROP COLUMN IF EXISTS tracking_registration_status;
