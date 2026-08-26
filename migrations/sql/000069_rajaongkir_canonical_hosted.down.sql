-- Rollback metadata ke model managed upstream lama. Submission hosted yang
-- sudah dipindahkan dikembalikan ke provider kandidat agar audit tetap utuh.

INSERT INTO shipping_integration_providers (
    code, name, logo_url, description, built_in, integration_type,
    distribution_type, requires_credential, credential_type, available,
    display_order
)
SELECT
    'rajaongkir_hosted',
    'RajaOngkir Hosted',
    logo_url,
    'Kandidat connector RajaOngkir hosted.',
    false,
    'partner_hosted',
    'private',
    false,
    'none',
    false,
    display_order + 1
FROM shipping_integration_providers
WHERE code = 'rajaongkir'
ON CONFLICT (code) DO NOTHING;

UPDATE tenant_active_shipping_providers
SET provider_code = NULL,
    credential_id = NULL,
    release_id = NULL,
    granted_scopes = '{}'::text[],
    version = version + 1,
    updated_by = 'migration:rollback-rajaongkir-hosted',
    updated_at = now()
WHERE provider_code = 'rajaongkir';

UPDATE shipping_integration_providers
SET active_release_id = NULL,
    integration_type = 'managed_upstream',
    distribution_type = 'public',
    requires_credential = true,
    credential_type = 'capability_api_keys',
    available = true,
    updated_at = now()
WHERE code = 'rajaongkir';

UPDATE partner_integration_submissions
SET provider_code = 'rajaongkir_hosted',
    updated_at = now()
WHERE provider_code = 'rajaongkir';

UPDATE partner_access_keys
SET provider_code = 'rajaongkir_hosted'
WHERE provider_code = 'rajaongkir';

UPDATE partner_explorer_credentials
SET provider_code = 'rajaongkir_hosted',
    updated_at = now()
WHERE provider_code = 'rajaongkir';

UPDATE partner_explorer_runs
SET provider_code = 'rajaongkir_hosted'
WHERE provider_code = 'rajaongkir';

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
            AND (
                (credential_id IS NOT NULL AND release_id IS NULL)
                OR
                (credential_id IS NULL AND release_id IS NOT NULL)
            )
        )
    );

ALTER TABLE shipping_integration_providers
    DROP CONSTRAINT shipping_integration_providers_credential_consistency_check,
    ADD CONSTRAINT shipping_integration_providers_credential_consistency_check
        CHECK (
            (integration_type = 'managed_upstream' AND requires_credential AND credential_type <> 'none')
            OR
            (integration_type <> 'managed_upstream' AND NOT requires_credential AND credential_type = 'none')
        );
