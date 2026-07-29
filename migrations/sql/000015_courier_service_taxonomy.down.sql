DROP TABLE IF EXISTS courier_service_aliases;

ALTER TABLE rate_snapshots
    DROP CONSTRAINT IF EXISTS rate_snapshots_service_type_check,
    DROP CONSTRAINT IF EXISTS rate_snapshots_service_group_check,
    DROP COLUMN IF EXISTS service_variant_code,
    DROP COLUMN IF EXISTS service_type,
    DROP COLUMN IF EXISTS service_group,
    DROP COLUMN IF EXISTS canonical_service_code;

ALTER TABLE courier_services
    DROP CONSTRAINT IF EXISTS courier_services_classification_source_check,
    DROP CONSTRAINT IF EXISTS courier_services_group_check,
    DROP COLUMN IF EXISTS catalog_verified_at,
    DROP COLUMN IF EXISTS source_reference,
    DROP COLUMN IF EXISTS classification_source,
    DROP COLUMN IF EXISTS service_group;
