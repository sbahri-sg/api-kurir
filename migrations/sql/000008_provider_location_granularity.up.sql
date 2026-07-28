ALTER TABLE provider_location_mappings
    DROP CONSTRAINT IF EXISTS
        provider_location_mappings_provider_code_provider_location_id_key;

ALTER TABLE provider_location_mappings
    ADD CONSTRAINT provider_location_mappings_provider_level_id_key
    UNIQUE (provider_code, granularity, provider_location_id);

ALTER TABLE location_postal_codes
    ADD COLUMN provider_granularity text;

UPDATE location_postal_codes link
SET provider_granularity = location.level
FROM locations location
WHERE location.id = link.location_id;

ALTER TABLE location_postal_codes
    ALTER COLUMN provider_granularity SET NOT NULL,
    ADD CONSTRAINT location_postal_codes_provider_granularity_check CHECK (
        provider_granularity IN (
            'province',
            'city',
            'district',
            'subdistrict'
        )
    );

DROP INDEX IF EXISTS location_postal_codes_provider_lookup_idx;

CREATE INDEX location_postal_codes_provider_lookup_idx
    ON location_postal_codes (
        provider_code,
        provider_granularity,
        provider_location_id
    )
    WHERE active;

-- Provider IDs are only unique inside a hierarchy level. Checkpoints created
-- with the former global-ID assumption must be fetched again.
DELETE FROM provider_location_sync_checkpoints;
