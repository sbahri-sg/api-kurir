ALTER TABLE rate_snapshots
    ADD COLUMN request_fingerprint text,
    ADD COLUMN courier_name text,
    ADD COLUMN service_name text,
    ADD COLUMN description text,
    ADD COLUMN verification_status text NOT NULL DEFAULT 'observed',
    ADD COLUMN source_type text NOT NULL DEFAULT 'provider_quote';

UPDATE rate_snapshots
SET request_fingerprint = encode(digest(id::text, 'sha256'), 'hex')
WHERE request_fingerprint IS NULL;

ALTER TABLE rate_snapshots
    ALTER COLUMN request_fingerprint SET NOT NULL,
    ADD CONSTRAINT rate_snapshots_verification_status_check CHECK (
        verification_status IN (
            'official_public',
            'official_contract',
            'observed',
            'needs_contract_confirmation',
            'deprecated'
        )
    ),
    ADD CONSTRAINT rate_snapshots_source_type_check CHECK (
        source_type IN ('provider_quote', 'local_rate_card')
    );

CREATE INDEX rate_snapshots_fingerprint_fresh_idx ON rate_snapshots (
    request_fingerprint,
    provider_code,
    expires_at DESC
);

CREATE TABLE provider_api_calls (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_code text NOT NULL,
    credential_alias text NOT NULL,
    endpoint text NOT NULL,
    request_fingerprint text,
    http_status integer,
    outcome text NOT NULL CHECK (
        outcome IN ('success', 'client_error', 'provider_error', 'timeout', 'network_error')
    ),
    duration_ms bigint NOT NULL CHECK (duration_ms >= 0),
    quota_cost integer NOT NULL DEFAULT 1 CHECK (quota_cost >= 0),
    response_hash text,
    error_code text,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX provider_api_calls_usage_idx ON provider_api_calls (
    provider_code,
    credential_alias,
    created_at DESC
);
