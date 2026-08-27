UPDATE provider_credentials
SET active = false,
    disabled_at = coalesce(disabled_at, now()),
    updated_at = now()
WHERE environment_code = 'sandbox'
  AND active;

DROP INDEX IF EXISTS provider_credentials_environment_active_idx;
DROP INDEX IF EXISTS provider_credentials_one_active_per_merchant_provider_environment_idx;

CREATE UNIQUE INDEX provider_credentials_one_active_per_merchant_provider_idx
    ON provider_credentials (tenant_id, provider_code)
    WHERE tenant_id <> '' AND active;

ALTER TABLE provider_credentials
    DROP COLUMN environment_code;

ALTER TABLE shipping_integration_providers
    DROP COLUMN capability_environment_schema,
    DROP COLUMN environment_schema,
    DROP COLUMN credential_schema,
    DROP CONSTRAINT shipping_integration_providers_credential_type_check;

UPDATE shipping_integration_providers
SET credential_type = CASE
    WHEN credential_type = 'provider_declared' THEN 'api_key'
    ELSE credential_type
END;

ALTER TABLE shipping_integration_providers
    ADD CONSTRAINT shipping_integration_providers_credential_type_check
        CHECK (credential_type IN (
            'none',
            'api_key',
            'capability_api_keys',
            'bearer_token',
            'api_key_secret',
            'oauth2_client_credentials'
        ));
