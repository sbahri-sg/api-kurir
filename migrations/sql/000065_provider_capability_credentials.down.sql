UPDATE shipping_integration_providers
SET credential_type = 'api_key',
    updated_at = now()
WHERE code = 'rajaongkir'
  AND credential_type = 'capability_api_keys';

ALTER TABLE shipping_integration_providers
    DROP CONSTRAINT shipping_integration_providers_credential_type_check;

ALTER TABLE shipping_integration_providers
    ADD CONSTRAINT shipping_integration_providers_credential_type_check
        CHECK (credential_type IN (
            'none',
            'api_key',
            'bearer_token',
            'api_key_secret',
            'oauth2_client_credentials'
        ));

COMMENT ON COLUMN shipping_integration_providers.credential_type IS
    'Stable merchant credential form model rendered by Emisell, including OAuth client credentials.';
