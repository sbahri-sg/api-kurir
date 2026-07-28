-- PostgreSQL truncated the original generated constraint name to 63 bytes.
ALTER TABLE provider_location_mappings
    DROP CONSTRAINT IF EXISTS
        provider_location_mappings_provider_code_provider_location__key;

ALTER TABLE provider_location_mappings
    DROP CONSTRAINT IF EXISTS
        provider_location_mappings_provider_code_provider_location_id_key;
