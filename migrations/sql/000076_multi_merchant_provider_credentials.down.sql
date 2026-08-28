ALTER TABLE provider_credentials
    DROP CONSTRAINT IF EXISTS provider_credentials_tenant_provider_environment_fingerprint_key,
    DROP CONSTRAINT IF EXISTS provider_credentials_tenant_provider_environment_alias_key;

ALTER TABLE provider_credentials
    ADD CONSTRAINT provider_credentials_provider_code_secret_fingerprint_key
        UNIQUE (provider_code, secret_fingerprint),
    ADD CONSTRAINT provider_credentials_provider_code_credential_alias_key
        UNIQUE (provider_code, credential_alias);
