CREATE TABLE postal_codes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code varchar(5) NOT NULL UNIQUE CHECK (
        code ~ '^[0-9]{5}$' AND code <> '00000'
    ),
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE location_postal_codes (
    location_id uuid NOT NULL REFERENCES locations(id),
    postal_code_id uuid NOT NULL REFERENCES postal_codes(id),
    provider_code text NOT NULL,
    provider_location_id text,
    source_type text NOT NULL CHECK (
        source_type IN (
            'courier_direct',
            'aggregator_direct',
            'official_contract',
            'official_registry',
            'legacy_import'
        )
    ),
    source_endpoint text,
    raw_response_hash text,
    retrieved_at timestamptz NOT NULL DEFAULT now(),
    verified_at timestamptz,
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (location_id, postal_code_id, provider_code)
);

CREATE INDEX location_postal_codes_code_lookup_idx
    ON location_postal_codes (postal_code_id, location_id)
    WHERE active;

CREATE INDEX location_postal_codes_provider_lookup_idx
    ON location_postal_codes (provider_code, provider_location_id)
    WHERE active;

ALTER TABLE provider_location_mappings
    ADD COLUMN source_type text NOT NULL DEFAULT 'legacy_import' CHECK (
        source_type IN (
            'courier_direct',
            'aggregator_direct',
            'official_contract',
            'official_registry',
            'legacy_import'
        )
    ),
    ADD COLUMN source_endpoint text,
    ADD COLUMN raw_response_hash text,
    ADD COLUMN retrieved_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN confidence numeric(5,4) NOT NULL DEFAULT 1 CHECK (
        confidence >= 0 AND confidence <= 1
    );

CREATE TABLE provider_location_sync_checkpoints (
    provider_code text NOT NULL,
    credential_alias text NOT NULL,
    endpoint_level text NOT NULL CHECK (
        endpoint_level IN ('province', 'city', 'district', 'subdistrict')
    ),
    parent_provider_location_id text NOT NULL DEFAULT '',
    item_count integer NOT NULL DEFAULT 0 CHECK (item_count >= 0),
    response_hash text,
    completed_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (
        provider_code,
        credential_alias,
        endpoint_level,
        parent_provider_location_id
    )
);

INSERT INTO postal_codes (code)
SELECT DISTINCT postal_code
FROM locations
WHERE postal_code ~ '^[0-9]{5}$'
  AND postal_code <> '00000'
ON CONFLICT (code) DO NOTHING;

INSERT INTO location_postal_codes (
    location_id,
    postal_code_id,
    provider_code,
    provider_location_id,
    source_type,
    source_endpoint,
    retrieved_at,
    verified_at
)
SELECT
    location.id,
    postal.id,
    coalesce(mapping.provider_code, 'legacy'),
    mapping.provider_location_id,
    'legacy_import',
    mapping.source_endpoint,
    coalesce(mapping.retrieved_at, location.updated_at),
    mapping.verified_at
FROM locations location
JOIN postal_codes postal ON postal.code = location.postal_code
LEFT JOIN LATERAL (
    SELECT
        provider_code,
        provider_location_id,
        source_endpoint,
        retrieved_at,
        verified_at
    FROM provider_location_mappings
    WHERE location_id = location.id
      AND active
    ORDER BY verified_at DESC NULLS LAST, updated_at DESC
    LIMIT 1
) mapping ON true
ON CONFLICT (location_id, postal_code_id, provider_code) DO NOTHING;
