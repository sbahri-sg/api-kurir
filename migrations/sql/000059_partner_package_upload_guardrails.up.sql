CREATE TABLE partner_upload_rate_limits (
    key_id uuid NOT NULL
        REFERENCES partner_access_keys(id)
        ON DELETE CASCADE,
    bucket_start timestamptz NOT NULL,
    attempts integer NOT NULL DEFAULT 1,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (key_id, bucket_start),
    CONSTRAINT partner_upload_rate_limits_attempts_check
        CHECK (attempts BETWEEN 1 AND 1000)
);

CREATE INDEX partner_upload_rate_limits_updated_idx
    ON partner_upload_rate_limits (updated_at);

COMMENT ON TABLE partner_upload_rate_limits IS
    'Hourly upload-attempt ledger shared by all API replicas for Partner Portal abuse protection.';
