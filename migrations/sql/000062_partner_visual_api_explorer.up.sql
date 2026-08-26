CREATE TABLE partner_explorer_credentials (
    provider_code text PRIMARY KEY
        REFERENCES shipping_integration_providers(code)
        ON UPDATE CASCADE ON DELETE CASCADE,
    display_key text NOT NULL,
    auth_header text NOT NULL,
    auth_prefix text NOT NULL DEFAULT '',
    secret_ciphertext bytea NOT NULL,
    created_by text NOT NULL,
    updated_by text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT partner_explorer_credentials_display_key_check
        CHECK (length(display_key) BETWEEN 8 AND 64),
    CONSTRAINT partner_explorer_credentials_auth_header_check
        CHECK (
            length(auth_header) BETWEEN 2 AND 64
            AND auth_header ~ '^[A-Za-z][A-Za-z0-9-]+$'
        ),
    CONSTRAINT partner_explorer_credentials_auth_prefix_check
        CHECK (length(auth_prefix) <= 32),
    CONSTRAINT partner_explorer_credentials_secret_check
        CHECK (octet_length(secret_ciphertext) BETWEEN 36 AND 4096)
);

COMMENT ON TABLE partner_explorer_credentials IS
    'Encrypted official provider credentials used only by the controlled Partner Portal API Explorer.';

CREATE TABLE partner_explorer_rate_limits (
    key_id uuid NOT NULL
        REFERENCES partner_access_keys(id)
        ON DELETE CASCADE,
    bucket_start timestamptz NOT NULL,
    attempts integer NOT NULL DEFAULT 1,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (key_id, bucket_start),
    CONSTRAINT partner_explorer_rate_limits_attempts_check
        CHECK (attempts BETWEEN 1 AND 10000)
);

CREATE INDEX partner_explorer_rate_limits_updated_idx
    ON partner_explorer_rate_limits (updated_at);

CREATE TABLE partner_explorer_runs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    submission_id uuid NOT NULL
        REFERENCES partner_integration_submissions(id)
        ON DELETE CASCADE,
    provider_code text NOT NULL
        REFERENCES shipping_integration_providers(code)
        ON UPDATE CASCADE ON DELETE CASCADE,
    operation_id text NOT NULL,
    method text NOT NULL,
    path text NOT NULL,
    environment text NOT NULL DEFAULT 'official',
    response_status integer NOT NULL DEFAULT 0,
    duration_ms bigint NOT NULL DEFAULT 0,
    outcome text NOT NULL,
    error_code text NOT NULL DEFAULT '',
    response_preview text NOT NULL DEFAULT '',
    created_by text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT partner_explorer_runs_method_check
        CHECK (method IN ('GET', 'HEAD', 'OPTIONS', 'POST', 'PUT', 'PATCH', 'DELETE')),
    CONSTRAINT partner_explorer_runs_path_check
        CHECK (left(path, 1) = '/' AND length(path) <= 512),
    CONSTRAINT partner_explorer_runs_environment_check
        CHECK (environment = 'official'),
    CONSTRAINT partner_explorer_runs_response_status_check
        CHECK (response_status BETWEEN 0 AND 599),
    CONSTRAINT partner_explorer_runs_duration_check
        CHECK (duration_ms BETWEEN 0 AND 60000),
    CONSTRAINT partner_explorer_runs_outcome_check
        CHECK (outcome IN ('passed', 'failed')),
    CONSTRAINT partner_explorer_runs_preview_check
        CHECK (length(response_preview) <= 4096)
);

CREATE INDEX partner_explorer_runs_submission_created_idx
    ON partner_explorer_runs (submission_id, created_at DESC);

CREATE INDEX partner_explorer_runs_provider_created_idx
    ON partner_explorer_runs (provider_code, created_at DESC);

COMMENT ON TABLE partner_explorer_runs IS
    'Redacted evidence for official read-only Partner Portal API Explorer requests.';
