ALTER TABLE tracking_webhook_outbox
    ADD COLUMN deduplication_key text;

UPDATE tracking_webhook_outbox
SET deduplication_key = id::text
WHERE deduplication_key IS NULL;

ALTER TABLE tracking_webhook_outbox
    ALTER COLUMN deduplication_key SET NOT NULL;

ALTER TABLE tracking_webhook_outbox
    ADD CONSTRAINT tracking_webhook_outbox_deduplication_key_key
    UNIQUE (deduplication_key);
