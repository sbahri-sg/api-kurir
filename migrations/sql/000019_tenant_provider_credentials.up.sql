ALTER TABLE provider_credentials
    ADD COLUMN tenant_id text NOT NULL DEFAULT '' CHECK (
        tenant_id = '' OR tenant_id ~ '^[A-Za-z0-9._:-]{1,128}$'
    );

CREATE INDEX provider_credentials_tenant_active_idx
    ON provider_credentials (tenant_id, provider_code, created_at)
    WHERE active AND validation_status = 'valid';

ALTER TABLE provider_quota_ledger
    ADD COLUMN tenant_id text NOT NULL DEFAULT '' CHECK (
        tenant_id = '' OR tenant_id ~ '^[A-Za-z0-9._:-]{1,128}$'
    );

ALTER TABLE provider_quota_ledger
    DROP CONSTRAINT IF EXISTS provider_quota_ledger_provider_code_credential_alias_quota__key;

ALTER TABLE provider_quota_ledger
    ADD CONSTRAINT provider_quota_ledger_tenant_provider_credential_date_key
    UNIQUE (tenant_id, provider_code, credential_alias, quota_date);

ALTER TABLE provider_api_calls
    ADD COLUMN tenant_id text NOT NULL DEFAULT '' CHECK (
        tenant_id = '' OR tenant_id ~ '^[A-Za-z0-9._:-]{1,128}$'
    );

CREATE INDEX provider_api_calls_tenant_usage_idx ON provider_api_calls (
    tenant_id,
    provider_code,
    credential_alias,
    created_at DESC
);

ALTER TABLE rate_snapshots
    ADD COLUMN tenant_id text NOT NULL DEFAULT '' CHECK (
        tenant_id = '' OR tenant_id ~ '^[A-Za-z0-9._:-]{1,128}$'
    ),
    ADD COLUMN integration_id text NOT NULL DEFAULT '' CHECK (
        integration_id = '' OR integration_id ~ '^[A-Za-z0-9._:-]{1,128}$'
    );

CREATE INDEX rate_snapshots_tenant_fingerprint_fresh_idx ON rate_snapshots (
    tenant_id,
    request_fingerprint,
    provider_code,
    expires_at DESC
);

ALTER TABLE tracking_shipments
    ADD COLUMN tenant_id text NOT NULL DEFAULT '' CHECK (
        tenant_id = '' OR tenant_id ~ '^[A-Za-z0-9._:-]{1,128}$'
    ),
    ADD COLUMN provider_credential_id text NOT NULL DEFAULT '' CHECK (
        provider_credential_id = '' OR
        provider_credential_id ~ '^[A-Za-z0-9._:-]{1,128}$'
    );

ALTER TABLE tracking_shipments
    ADD CONSTRAINT tracking_shipments_tenant_courier_waybill_key
    UNIQUE (tenant_id, courier_code, waybill_hash);

CREATE INDEX tracking_shipments_tenant_idx
    ON tracking_shipments (tenant_id, updated_at DESC);
