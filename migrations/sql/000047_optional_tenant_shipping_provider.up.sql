ALTER TABLE tenant_active_shipping_providers
    DROP CONSTRAINT tenant_active_shipping_providers_check;

ALTER TABLE tenant_active_shipping_providers
    ALTER COLUMN provider_code DROP NOT NULL;

ALTER TABLE tenant_active_shipping_providers
    ADD CONSTRAINT tenant_active_shipping_providers_selection_check CHECK (
        (provider_code IS NULL AND credential_id IS NULL)
        OR
        (provider_code = 'emisell' AND credential_id IS NULL)
        OR
        (provider_code IS NOT NULL AND provider_code <> 'emisell' AND credential_id IS NOT NULL)
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
            version = version + 1,
            updated_by = 'system:credential-disabled',
            updated_at = now()
        WHERE credential_id = NEW.id;
    END IF;
    RETURN NEW;
END;
$$;
