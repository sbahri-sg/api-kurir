-- Satukan pilihan provider aktif kembali ke scope legacy sebelum kolom
-- provider dihapus agar rollback tidak mengosongkan konfigurasi merchant.
INSERT INTO tenant_shipping_service_selections (
    tenant_id,
    provider_code,
    courier_service_id,
    created_at
)
SELECT
    selection.tenant_id,
    'emisell',
    selection.courier_service_id,
    selection.created_at
FROM tenant_shipping_service_selections selection
WHERE selection.provider_code <> 'emisell'
ON CONFLICT (tenant_id, provider_code, courier_service_id) DO NOTHING;

DELETE FROM tenant_shipping_service_selections
WHERE provider_code <> 'emisell';

DELETE FROM tenant_shipping_preferences
WHERE provider_code <> 'emisell';

ALTER TABLE tenant_shipping_service_selections
    DROP CONSTRAINT tenant_shipping_service_selections_preference_fkey,
    DROP CONSTRAINT tenant_shipping_service_selections_pkey;

ALTER TABLE tenant_shipping_preferences
    DROP CONSTRAINT tenant_shipping_preferences_pkey,
    DROP CONSTRAINT tenant_shipping_preferences_provider_fkey,
    ADD CONSTRAINT tenant_shipping_preferences_pkey PRIMARY KEY (tenant_id);

ALTER TABLE tenant_shipping_service_selections
    ADD CONSTRAINT tenant_shipping_service_selections_pkey
        PRIMARY KEY (tenant_id, courier_service_id),
    ADD CONSTRAINT tenant_shipping_service_selections_tenant_id_fkey
        FOREIGN KEY (tenant_id) REFERENCES tenant_shipping_preferences(tenant_id)
        ON DELETE CASCADE;

ALTER TABLE tenant_shipping_service_selections
    DROP COLUMN provider_code;

ALTER TABLE tenant_shipping_preferences
    DROP COLUMN provider_code;

DROP TABLE provider_courier_services;
