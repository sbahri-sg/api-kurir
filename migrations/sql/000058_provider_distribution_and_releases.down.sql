CREATE OR REPLACE FUNCTION fallback_disabled_shipping_provider_credential()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF OLD.active AND NOT NEW.active THEN
        UPDATE tenant_active_shipping_providers
        SET provider_code = NULL,
            credential_id = NULL,
            version = version + 1,
            updated_by = 'system:credential-disabled',
            updated_at = now()
        WHERE credential_id = NEW.id;
    END IF;
    RETURN NEW;
END;
$$;

ALTER TABLE tenant_active_shipping_providers
    DROP CONSTRAINT tenant_active_shipping_providers_selection_check,
    DROP CONSTRAINT tenant_active_shipping_providers_release_fkey,
    DROP COLUMN granted_scopes,
    DROP COLUMN release_id,
    ADD CONSTRAINT tenant_active_shipping_providers_selection_check CHECK (
        (provider_code IS NULL AND credential_id IS NULL)
        OR
        (provider_code = 'emisell' AND credential_id IS NULL)
        OR
        (provider_code IS NOT NULL AND provider_code <> 'emisell' AND credential_id IS NOT NULL)
    );

UPDATE shipping_integration_providers
SET requires_credential = NOT built_in;

ALTER TABLE shipping_integration_providers
    DROP CONSTRAINT shipping_integration_providers_active_release_fkey,
    DROP CONSTRAINT shipping_integration_providers_release_type_check,
    DROP CONSTRAINT shipping_integration_providers_type_consistency_check,
    DROP CONSTRAINT shipping_integration_providers_distribution_type_check,
    DROP CONSTRAINT shipping_integration_providers_integration_type_check,
    DROP COLUMN active_release_id,
    DROP COLUMN distribution_type,
    DROP COLUMN integration_type;

ALTER TABLE partner_integration_submissions
    DROP CONSTRAINT partner_integration_submissions_provider_id_unique,
    DROP COLUMN required_scopes;
