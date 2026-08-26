-- RajaOngkir menjadi satu provider canonical yang dijalankan melalui connector
-- hosted API Kurir. Credential resmi tetap milik merchant (BYOK), sehingga
-- connector release dan credential merchant sama-sama diperlukan saat aktif.

ALTER TABLE shipping_integration_providers
    DROP CONSTRAINT shipping_integration_providers_credential_consistency_check;

ALTER TABLE shipping_integration_providers
    ADD CONSTRAINT shipping_integration_providers_credential_consistency_check
        CHECK (
            (integration_type = 'built_in' AND NOT requires_credential AND credential_type = 'none')
            OR
            (integration_type = 'managed_upstream' AND requires_credential AND credential_type <> 'none')
            OR
            (
                integration_type = 'partner_hosted'
                AND (
                    (requires_credential AND credential_type <> 'none')
                    OR
                    (NOT requires_credential AND credential_type = 'none')
                )
            )
        );

ALTER TABLE tenant_active_shipping_providers
    DROP CONSTRAINT tenant_active_shipping_providers_selection_check,
    ADD CONSTRAINT tenant_active_shipping_providers_selection_check CHECK (
        (provider_code IS NULL AND credential_id IS NULL AND release_id IS NULL)
        OR
        (provider_code = 'emisell' AND credential_id IS NULL AND release_id IS NULL)
        OR
        (
            provider_code IS NOT NULL
            AND provider_code <> 'emisell'
            AND (credential_id IS NOT NULL OR release_id IS NOT NULL)
        )
    );

-- Lepaskan pointer release sebelum submission UAT kandidat dipindahkan ke kode
-- canonical. Data upload, access key, credential explorer, dan bukti test tetap
-- dipertahankan; hanya provider duplikatnya yang dihilangkan.
UPDATE shipping_integration_providers
SET active_release_id = NULL,
    updated_at = now()
WHERE code IN ('rajaongkir', 'rajaongkir_hosted');

UPDATE partner_integration_submissions
SET provider_code = 'rajaongkir',
    updated_at = now()
WHERE provider_code = 'rajaongkir_hosted';

UPDATE partner_access_keys
SET provider_code = 'rajaongkir'
WHERE provider_code = 'rajaongkir_hosted';

UPDATE partner_explorer_credentials
SET provider_code = 'rajaongkir',
    updated_at = now()
WHERE provider_code = 'rajaongkir_hosted';

UPDATE partner_explorer_runs
SET provider_code = 'rajaongkir'
WHERE provider_code = 'rajaongkir_hosted';

DELETE FROM tenant_shipping_service_selections
WHERE provider_code = 'rajaongkir_hosted';

DELETE FROM tenant_shipping_preferences
WHERE provider_code = 'rajaongkir_hosted';

DELETE FROM provider_courier_services
WHERE provider_code = 'rajaongkir_hosted';

UPDATE tenant_active_shipping_providers
SET provider_code = NULL,
    credential_id = NULL,
    release_id = NULL,
    granted_scopes = '{}'::text[],
    version = version + 1,
    updated_by = 'migration:rajaongkir-canonical-hosted',
    updated_at = now()
WHERE provider_code = 'rajaongkir_hosted';

DELETE FROM shipping_integration_providers
WHERE code = 'rajaongkir_hosted';

UPDATE shipping_integration_providers
SET name = 'RajaOngkir',
    description = 'Connector RajaOngkir yang di-host API Kurir; merchant tetap memakai Shipping Cost dan Shipping Delivery API key miliknya sendiri.',
    integration_type = 'partner_hosted',
    distribution_type = 'public',
    requires_credential = true,
    credential_type = 'capability_api_keys',
    active_release_id = (
        SELECT submission.id
        FROM partner_integration_submissions submission
        WHERE submission.provider_code = 'rajaongkir'
          AND submission.status = 'published'
        ORDER BY submission.updated_at DESC
        LIMIT 1
    ),
    available = EXISTS (
        SELECT 1
        FROM partner_integration_submissions submission
        WHERE submission.provider_code = 'rajaongkir'
          AND submission.status = 'published'
    ),
    updated_at = now()
WHERE code = 'rajaongkir';

-- Pilihan lama hanya dapat dipertahankan apabila connector telah published.
UPDATE tenant_active_shipping_providers selection
SET release_id = provider.active_release_id,
    granted_scopes = COALESCE(submission.required_scopes, '{}'::text[]),
    version = selection.version + 1,
    updated_by = 'migration:rajaongkir-canonical-hosted',
    updated_at = now()
FROM shipping_integration_providers provider
LEFT JOIN partner_integration_submissions submission
  ON submission.id = provider.active_release_id
WHERE selection.provider_code = 'rajaongkir'
  AND provider.code = 'rajaongkir'
  AND provider.active_release_id IS NOT NULL;

UPDATE tenant_active_shipping_providers selection
SET provider_code = NULL,
    credential_id = NULL,
    release_id = NULL,
    granted_scopes = '{}'::text[],
    version = selection.version + 1,
    updated_by = 'migration:rajaongkir-awaiting-release',
    updated_at = now()
WHERE selection.provider_code = 'rajaongkir'
  AND NOT EXISTS (
      SELECT 1
      FROM shipping_integration_providers provider
      WHERE provider.code = 'rajaongkir'
        AND provider.active_release_id IS NOT NULL
  );

COMMENT ON TABLE shipping_integration_providers IS
    'Provider shipping built-in, managed upstream, dan partner hosted. RajaOngkir menggunakan satu kode canonical dan connector hosted.';
