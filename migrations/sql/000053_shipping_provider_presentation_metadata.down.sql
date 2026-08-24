ALTER TABLE shipping_integration_providers
    DROP CONSTRAINT IF EXISTS shipping_integration_providers_description_check,
    DROP CONSTRAINT IF EXISTS shipping_integration_providers_logo_url_check,
    DROP COLUMN IF EXISTS description,
    DROP COLUMN IF EXISTS logo_url;
