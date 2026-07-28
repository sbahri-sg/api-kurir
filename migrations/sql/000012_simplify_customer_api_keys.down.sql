DROP INDEX IF EXISTS customer_api_keys_active_idx;

ALTER TABLE customer_api_keys
    ADD COLUMN name text NOT NULL DEFAULT 'Customer API Key',
    ADD COLUMN expires_at timestamptz;

ALTER TABLE customer_api_keys
    ALTER COLUMN name DROP DEFAULT,
    ADD CONSTRAINT customer_api_keys_name_length_check
        CHECK (char_length(name) BETWEEN 3 AND 80);

CREATE INDEX customer_api_keys_active_expiry_idx
    ON customer_api_keys (active, expires_at);
