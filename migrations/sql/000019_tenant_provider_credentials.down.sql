DROP INDEX IF EXISTS tracking_shipments_tenant_idx;
ALTER TABLE tracking_shipments
    DROP CONSTRAINT IF EXISTS tracking_shipments_tenant_courier_waybill_key;
ALTER TABLE tracking_shipments
    DROP COLUMN IF EXISTS provider_credential_id,
    DROP COLUMN IF EXISTS tenant_id;

DROP INDEX IF EXISTS rate_snapshots_tenant_fingerprint_fresh_idx;
ALTER TABLE rate_snapshots
    DROP COLUMN IF EXISTS integration_id,
    DROP COLUMN IF EXISTS tenant_id;

DROP INDEX IF EXISTS provider_api_calls_tenant_usage_idx;
ALTER TABLE provider_api_calls DROP COLUMN IF EXISTS tenant_id;

ALTER TABLE provider_quota_ledger
    DROP CONSTRAINT IF EXISTS provider_quota_ledger_tenant_provider_credential_date_key;
ALTER TABLE provider_quota_ledger DROP COLUMN IF EXISTS tenant_id;

DROP INDEX IF EXISTS provider_credentials_tenant_active_idx;
ALTER TABLE provider_credentials DROP COLUMN IF EXISTS tenant_id;
