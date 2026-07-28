DROP INDEX IF EXISTS customer_api_keys_active_expiry_idx;

ALTER TABLE customer_api_keys
    DROP COLUMN name,
    DROP COLUMN expires_at;

CREATE INDEX customer_api_keys_active_idx
    ON customer_api_keys (active)
    WHERE active;
