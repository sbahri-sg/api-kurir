ALTER TABLE partner_explorer_credentials
    DROP CONSTRAINT partner_explorer_credentials_pkey;

ALTER TABLE partner_explorer_credentials
    ADD COLUMN credential_code text NOT NULL DEFAULT 'default';

ALTER TABLE partner_explorer_credentials
    ADD CONSTRAINT partner_explorer_credentials_code_check
        CHECK (credential_code ~ '^[a-z0-9][a-z0-9_-]{1,47}$'),
    ADD PRIMARY KEY (provider_code, credential_code);

COMMENT ON COLUMN partner_explorer_credentials.credential_code IS
    'Stable OpenAPI credential profile, for example shipping_cost or shipping_delivery.';
