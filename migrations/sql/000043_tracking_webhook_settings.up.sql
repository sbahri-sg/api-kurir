CREATE TABLE tracking_webhook_settings (
    id smallint PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    callback_url text NOT NULL DEFAULT '',
    secret_ciphertext bytea,
    secret_fingerprint bytea,
    secret_prefix text NOT NULL DEFAULT '',
    secret_last_four text NOT NULL DEFAULT '',
    enabled boolean NOT NULL DEFAULT false,
    last_test_at timestamptz,
    last_test_success boolean,
    last_test_http_status integer,
    last_test_error text NOT NULL DEFAULT '',
    created_by text NOT NULL DEFAULT 'system',
    updated_by text NOT NULL DEFAULT 'system',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (
        (secret_ciphertext IS NULL AND secret_fingerprint IS NULL) OR
        (secret_ciphertext IS NOT NULL AND secret_fingerprint IS NOT NULL)
    )
);

COMMENT ON TABLE tracking_webhook_settings IS
    'Konfigurasi singleton webhook tracking outbound menuju Emisell.';
COMMENT ON COLUMN tracking_webhook_settings.secret_ciphertext IS
    'Secret HMAC terenkripsi; plaintext hanya dikembalikan saat generate/rotate.';
