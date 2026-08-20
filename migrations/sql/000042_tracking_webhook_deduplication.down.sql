ALTER TABLE tracking_webhook_outbox
    DROP CONSTRAINT IF EXISTS tracking_webhook_outbox_deduplication_key_key,
    DROP COLUMN IF EXISTS deduplication_key;
