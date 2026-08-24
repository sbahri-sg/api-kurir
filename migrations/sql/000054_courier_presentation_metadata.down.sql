ALTER TABLE couriers
    DROP CONSTRAINT IF EXISTS couriers_logo_source_url_check,
    DROP CONSTRAINT IF EXISTS couriers_logo_url_check,
    DROP COLUMN IF EXISTS logo_verified_at,
    DROP COLUMN IF EXISTS logo_source_url,
    DROP COLUMN IF EXISTS logo_url;
