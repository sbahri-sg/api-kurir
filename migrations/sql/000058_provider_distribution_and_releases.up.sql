ALTER TABLE partner_integration_submissions
    ADD COLUMN required_scopes text[] NOT NULL DEFAULT '{}';

ALTER TABLE shipping_integration_providers
    ADD COLUMN integration_type text NOT NULL DEFAULT 'partner_hosted',
    ADD COLUMN distribution_type text NOT NULL DEFAULT 'public',
    ADD COLUMN active_release_id uuid;

UPDATE shipping_integration_providers
SET integration_type = CASE code
        WHEN 'emisell' THEN 'built_in'
        WHEN 'rajaongkir' THEN 'managed_upstream'
        WHEN 'kiriminaja' THEN 'managed_upstream'
        ELSE 'partner_hosted'
    END,
    distribution_type = CASE code
        WHEN 'emisell' THEN 'built_in'
        ELSE 'public'
    END,
    requires_credential = CASE code
        WHEN 'rajaongkir' THEN true
        WHEN 'kiriminaja' THEN true
        ELSE false
    END;

UPDATE shipping_integration_providers provider
SET active_release_id = release.id
FROM partner_integration_submissions release
WHERE release.provider_code = provider.code
  AND release.status = 'published';

ALTER TABLE partner_integration_submissions
    ADD CONSTRAINT partner_integration_submissions_provider_id_unique
        UNIQUE (provider_code, id);

ALTER TABLE shipping_integration_providers
    ADD CONSTRAINT shipping_integration_providers_integration_type_check
        CHECK (integration_type IN ('built_in', 'managed_upstream', 'partner_hosted')),
    ADD CONSTRAINT shipping_integration_providers_distribution_type_check
        CHECK (distribution_type IN ('built_in', 'public', 'limited', 'private')),
    ADD CONSTRAINT shipping_integration_providers_type_consistency_check
        CHECK (
            (built_in AND integration_type = 'built_in' AND distribution_type = 'built_in')
            OR
            (NOT built_in AND integration_type <> 'built_in' AND distribution_type <> 'built_in')
        ),
    ADD CONSTRAINT shipping_integration_providers_release_type_check
        CHECK (active_release_id IS NULL OR integration_type = 'partner_hosted'),
    ADD CONSTRAINT shipping_integration_providers_active_release_fkey
        FOREIGN KEY (code, active_release_id)
        REFERENCES partner_integration_submissions(provider_code, id)
        ON DELETE RESTRICT;

ALTER TABLE tenant_active_shipping_providers
    ADD COLUMN release_id uuid,
    ADD COLUMN granted_scopes text[] NOT NULL DEFAULT '{}',
    ADD CONSTRAINT tenant_active_shipping_providers_release_fkey
        FOREIGN KEY (provider_code, release_id)
        REFERENCES partner_integration_submissions(provider_code, id)
        ON DELETE RESTRICT;

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

CREATE OR REPLACE FUNCTION fallback_disabled_shipping_provider_credential()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF OLD.active AND NOT NEW.active THEN
        UPDATE tenant_active_shipping_providers
        SET provider_code = NULL,
            credential_id = NULL,
            release_id = NULL,
            granted_scopes = '{}'::text[],
            version = version + 1,
            updated_by = 'system:credential-disabled',
            updated_at = now()
        WHERE credential_id = NEW.id;
    END IF;
    RETURN NEW;
END;
$$;

UPDATE tenant_active_shipping_providers
SET granted_scopes = ARRAY['rates:read', 'tracking:read']::text[]
WHERE provider_code IS NOT NULL;
