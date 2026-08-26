CREATE TABLE provider_courier_services (
    provider_code text NOT NULL REFERENCES shipping_integration_providers(code)
        ON DELETE CASCADE,
    courier_service_id uuid NOT NULL REFERENCES courier_services(id)
        ON DELETE RESTRICT,
    active boolean NOT NULL DEFAULT true,
    source text NOT NULL DEFAULT 'internal' CHECK (
        source IN ('internal', 'provider_sync', 'certified_package', 'legacy_backfill')
    ),
    certified_at timestamptz,
    last_synced_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (provider_code, courier_service_id)
);

CREATE INDEX provider_courier_services_service_idx
    ON provider_courier_services (courier_service_id, provider_code)
    WHERE active;

-- Emisell Kurir mempertahankan seluruh katalog canonical yang telah berjalan.
INSERT INTO provider_courier_services (
    provider_code,
    courier_service_id,
    source,
    certified_at
)
SELECT
    'emisell',
    service.id,
    'internal',
    now()
FROM courier_services service
JOIN couriers courier ON courier.id = service.courier_id
WHERE courier.active
  AND service.active
ON CONFLICT DO NOTHING;

-- RajaOngkir BYOK memulai dari katalog existing agar integrasi merchant yang
-- sudah stabil tidak berubah saat migrasi. Sink provider dapat mempersempit
-- katalog ini setelah rollout provider-aware selesai.
INSERT INTO provider_courier_services (
    provider_code,
    courier_service_id,
    source,
    certified_at
)
SELECT
    'rajaongkir',
    service.id,
    'legacy_backfill',
    now()
FROM courier_services service
JOIN couriers courier ON courier.id = service.courier_id
WHERE courier.active
  AND service.active
ON CONFLICT DO NOTHING;

ALTER TABLE tenant_shipping_preferences
    ADD COLUMN provider_code text NOT NULL DEFAULT 'emisell';

ALTER TABLE tenant_shipping_preferences
    ADD CONSTRAINT tenant_shipping_preferences_provider_fkey
    FOREIGN KEY (provider_code) REFERENCES shipping_integration_providers(code)
    ON DELETE RESTRICT;

ALTER TABLE tenant_shipping_service_selections
    ADD COLUMN provider_code text NOT NULL DEFAULT 'emisell';

ALTER TABLE tenant_shipping_service_selections
    DROP CONSTRAINT tenant_shipping_service_selections_tenant_id_fkey,
    DROP CONSTRAINT tenant_shipping_service_selections_pkey;

ALTER TABLE tenant_shipping_preferences
    DROP CONSTRAINT tenant_shipping_preferences_pkey,
    ADD CONSTRAINT tenant_shipping_preferences_pkey
        PRIMARY KEY (tenant_id, provider_code);

ALTER TABLE tenant_shipping_service_selections
    ADD CONSTRAINT tenant_shipping_service_selections_pkey
        PRIMARY KEY (tenant_id, provider_code, courier_service_id),
    ADD CONSTRAINT tenant_shipping_service_selections_preference_fkey
        FOREIGN KEY (tenant_id, provider_code)
        REFERENCES tenant_shipping_preferences(tenant_id, provider_code)
        ON DELETE CASCADE;

-- Pertahankan pilihan existing untuk provider non-Emisell yang sedang aktif.
INSERT INTO tenant_shipping_preferences (
    tenant_id,
    provider_code,
    selection_mode,
    enabled_groups,
    version,
    updated_by,
    created_at,
    updated_at
)
SELECT
    preference.tenant_id,
    active.provider_code,
    preference.selection_mode,
    preference.enabled_groups,
    preference.version,
    'migration:provider-scope',
    preference.created_at,
    preference.updated_at
FROM tenant_shipping_preferences preference
JOIN tenant_active_shipping_providers active
  ON active.tenant_id = preference.tenant_id
WHERE preference.provider_code = 'emisell'
  AND active.provider_code IS NOT NULL
  AND active.provider_code <> 'emisell'
ON CONFLICT (tenant_id, provider_code) DO NOTHING;

INSERT INTO tenant_shipping_service_selections (
    tenant_id,
    provider_code,
    courier_service_id,
    created_at
)
SELECT
    selection.tenant_id,
    active.provider_code,
    selection.courier_service_id,
    selection.created_at
FROM tenant_shipping_service_selections selection
JOIN tenant_active_shipping_providers active
  ON active.tenant_id = selection.tenant_id
WHERE selection.provider_code = 'emisell'
  AND active.provider_code IS NOT NULL
  AND active.provider_code <> 'emisell'
ON CONFLICT (tenant_id, provider_code, courier_service_id) DO NOTHING;

-- Provider aktif custom tetap memperoleh service yang sebelumnya dipakai,
-- sehingga tidak ada pilihan checkout yang hilang pada saat cutover.
INSERT INTO provider_courier_services (
    provider_code,
    courier_service_id,
    source,
    certified_at
)
SELECT DISTINCT
    selection.provider_code,
    selection.courier_service_id,
    'legacy_backfill',
    now()
FROM tenant_shipping_service_selections selection
WHERE selection.provider_code <> 'emisell'
ON CONFLICT (provider_code, courier_service_id) DO NOTHING;

ALTER TABLE tenant_shipping_preferences
    ALTER COLUMN provider_code DROP DEFAULT;

ALTER TABLE tenant_shipping_service_selections
    ALTER COLUMN provider_code DROP DEFAULT;
