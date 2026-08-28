ALTER TABLE provider_credentials
    DROP CONSTRAINT IF EXISTS provider_credentials_provider_code_secret_fingerprint_key,
    DROP CONSTRAINT IF EXISTS provider_credentials_provider_code_credential_alias_key;

ALTER TABLE provider_credentials
    ADD CONSTRAINT provider_credentials_tenant_provider_environment_fingerprint_key
        UNIQUE (tenant_id, provider_code, environment_code, secret_fingerprint),
    ADD CONSTRAINT provider_credentials_tenant_provider_environment_alias_key
        UNIQUE (tenant_id, provider_code, environment_code, credential_alias);

COMMENT ON CONSTRAINT provider_credentials_tenant_provider_environment_fingerprint_key
    ON provider_credentials IS
    'Credential yang sama boleh dipasang merchant berbeda; duplikasi hanya dicegah di dalam merchant, provider, dan environment yang sama.';
