CREATE TABLE partner_integration_submissions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_code text NOT NULL
        REFERENCES shipping_integration_providers(code)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    version text NOT NULL,
    status text NOT NULL,
    file_name text NOT NULL,
    content_type text NOT NULL DEFAULT 'application/zip',
    artifact_size bigint NOT NULL,
    artifact_sha256 char(64) NOT NULL,
    artifact bytea NOT NULL,
    scan_report jsonb NOT NULL DEFAULT '{}'::jsonb,
    review_note text NOT NULL DEFAULT '',
    submitted_by text NOT NULL,
    reviewed_by text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT partner_integration_submissions_version_check
        CHECK (version ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$'),
    CONSTRAINT partner_integration_submissions_status_check
        CHECK (status IN (
            'technical_review', 'sandbox_testing', 'security_review', 'uat',
            'approved', 'published', 'changes_requested', 'rejected',
            'suspended', 'superseded'
        )),
    CONSTRAINT partner_integration_submissions_file_name_check
        CHECK (length(file_name) BETWEEN 5 AND 255),
    CONSTRAINT partner_integration_submissions_artifact_size_check
        CHECK (artifact_size BETWEEN 1 AND 26214400),
    CONSTRAINT partner_integration_submissions_review_note_check
        CHECK (length(review_note) <= 4000),
    UNIQUE (provider_code, version)
);

CREATE INDEX partner_integration_submissions_status_created_idx
    ON partner_integration_submissions (status, created_at DESC);

CREATE INDEX partner_integration_submissions_provider_created_idx
    ON partner_integration_submissions (provider_code, created_at DESC);

CREATE UNIQUE INDEX partner_integration_submissions_one_published_idx
    ON partner_integration_submissions (provider_code)
    WHERE status = 'published';

CREATE TABLE partner_access_keys (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_code text NOT NULL
        REFERENCES shipping_integration_providers(code)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    display_key text NOT NULL,
    secret_hash char(64) NOT NULL UNIQUE,
    active boolean NOT NULL DEFAULT true,
    created_by text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz,
    revoked_by text NOT NULL DEFAULT '',
    revoked_at timestamptz,
    CONSTRAINT partner_access_keys_display_key_check
        CHECK (
            left(display_key, 17) = 'epk_live_********'
            AND length(display_key) = 21
            AND right(display_key, 4) ~ '^[A-Za-z0-9_-]{4}$'
        ),
    CONSTRAINT partner_access_keys_lifecycle_check
        CHECK (
            (active = true AND revoked_at IS NULL AND revoked_by = '')
            OR
            (active = false AND revoked_at IS NOT NULL AND revoked_by <> '')
        )
);

CREATE INDEX partner_access_keys_provider_created_idx
    ON partner_access_keys (provider_code, created_at DESC);

CREATE INDEX partner_access_keys_active_hash_idx
    ON partner_access_keys (secret_hash)
    WHERE active = true;
