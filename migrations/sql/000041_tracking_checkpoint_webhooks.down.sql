DROP TABLE IF EXISTS tracking_webhook_outbox;
DROP TABLE IF EXISTS tracking_subscriptions;
DROP TABLE IF EXISTS tracking_status_history;

ALTER TABLE tracking_shipments
    DROP COLUMN IF EXISTS status_changed_at,
    DROP COLUMN IF EXISTS provider_hit_limit,
    DROP COLUMN IF EXISTS provider_hit_count,
    DROP COLUMN IF EXISTS not_found_count,
    DROP COLUMN IF EXISTS validation_checked_at,
    DROP COLUMN IF EXISTS validation_status;

ALTER TABLE tracking_shipments
    ADD CONSTRAINT tracking_shipments_courier_code_waybill_hash_key
    UNIQUE (courier_code, waybill_hash);
