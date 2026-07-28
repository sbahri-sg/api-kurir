CREATE TABLE customer_api_keys (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (char_length(name) BETWEEN 3 AND 80),
    key_prefix text NOT NULL,
    key_last_four char(4) NOT NULL,
    key_hash bytea NOT NULL UNIQUE CHECK (octet_length(key_hash) = 32),
    scopes text[] NOT NULL DEFAULT ARRAY['shipping:read', 'tracking:read'],
    active boolean NOT NULL DEFAULT true,
    expires_at timestamptz,
    last_used_at timestamptz,
    created_by text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_by text,
    revoked_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (
        (active AND revoked_at IS NULL) OR
        (NOT active AND revoked_at IS NOT NULL)
    )
);

CREATE INDEX customer_api_keys_active_expiry_idx
    ON customer_api_keys (active, expires_at);

CREATE INDEX customer_api_keys_created_idx
    ON customer_api_keys (created_at DESC);
