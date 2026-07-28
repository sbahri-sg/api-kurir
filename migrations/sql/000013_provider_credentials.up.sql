CREATE TABLE provider_credentials (
    id uuid PRIMARY KEY,
    provider_code text NOT NULL CHECK (provider_code ~ '^[a-z0-9_-]+$'),
    credential_alias text NOT NULL,
    secret_ciphertext bytea NOT NULL,
    secret_fingerprint bytea NOT NULL CHECK (octet_length(secret_fingerprint) = 32),
    key_prefix text NOT NULL,
    key_last_four char(4) NOT NULL,
    daily_limit bigint NOT NULL DEFAULT 50000 CHECK (daily_limit > 0),
    active boolean NOT NULL DEFAULT true,
    validation_status text NOT NULL DEFAULT 'valid'
        CHECK (validation_status IN ('valid', 'invalid')),
    last_validated_at timestamptz NOT NULL DEFAULT now(),
    last_selected_at timestamptz,
    created_by text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    disabled_by text,
    disabled_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider_code, secret_fingerprint),
    UNIQUE (provider_code, credential_alias),
    CHECK (
        (active AND disabled_at IS NULL) OR
        (NOT active AND disabled_at IS NOT NULL)
    )
);

CREATE INDEX provider_credentials_active_idx
    ON provider_credentials (provider_code, created_at)
    WHERE active AND validation_status = 'valid';
