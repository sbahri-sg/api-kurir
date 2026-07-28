ALTER TABLE couriers
    DROP CONSTRAINT IF EXISTS couriers_provider_code_check,
    DROP COLUMN IF EXISTS catalog_verified_at,
    DROP COLUMN IF EXISTS catalog_source,
    DROP COLUMN IF EXISTS supports_tracking,
    DROP COLUMN IF EXISTS supports_international_cost,
    DROP COLUMN IF EXISTS supports_domestic_cost,
    DROP COLUMN IF EXISTS provider_code;
