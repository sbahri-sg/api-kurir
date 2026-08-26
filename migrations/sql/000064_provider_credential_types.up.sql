ALTER TABLE shipping_integration_providers
    ADD COLUMN credential_type text NOT NULL DEFAULT 'none';

UPDATE shipping_integration_providers
SET credential_type = CASE
    WHEN integration_type IN ('built_in', 'partner_hosted') THEN 'none'
    WHEN code = 'kiriminaja' THEN 'bearer_token'
    ELSE 'api_key'
END;

ALTER TABLE shipping_integration_providers
    ADD CONSTRAINT shipping_integration_providers_credential_type_check
        CHECK (credential_type IN (
            'none',
            'api_key',
            'bearer_token',
            'api_key_secret',
            'oauth2_client_credentials'
        )),
    ADD CONSTRAINT shipping_integration_providers_credential_consistency_check
        CHECK (
            (integration_type = 'managed_upstream' AND requires_credential AND credential_type <> 'none')
            OR
            (integration_type <> 'managed_upstream' AND NOT requires_credential AND credential_type = 'none')
        );

COMMENT ON COLUMN shipping_integration_providers.credential_type IS
    'Stable merchant credential form model rendered by Emisell, including OAuth client credentials.';
