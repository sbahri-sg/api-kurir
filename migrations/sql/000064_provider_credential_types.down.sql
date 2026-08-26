ALTER TABLE shipping_integration_providers
    DROP CONSTRAINT IF EXISTS shipping_integration_providers_credential_consistency_check,
    DROP CONSTRAINT IF EXISTS shipping_integration_providers_credential_type_check,
    DROP COLUMN IF EXISTS credential_type;
