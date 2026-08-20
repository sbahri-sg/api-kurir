ALTER TABLE couriers
    DROP CONSTRAINT IF EXISTS couriers_tracking_capability_check,
    DROP CONSTRAINT IF EXISTS couriers_tracking_provider_code_check,
    DROP CONSTRAINT IF EXISTS couriers_rate_provider_code_check,
    DROP COLUMN IF EXISTS tracking_catalog_source,
    DROP COLUMN IF EXISTS tracking_provider_code,
    DROP COLUMN IF EXISTS rate_provider_code;
