CREATE TABLE shipping_integration_providers (
    code text PRIMARY KEY CHECK (code ~ '^[a-z0-9_-]{2,48}$'),
    name text NOT NULL CHECK (length(name) BETWEEN 2 AND 100),
    built_in boolean NOT NULL DEFAULT false,
    requires_credential boolean NOT NULL DEFAULT true,
    available boolean NOT NULL DEFAULT true,
    display_order integer NOT NULL DEFAULT 100,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (NOT built_in OR NOT requires_credential)
);

INSERT INTO shipping_integration_providers (
    code,
    name,
    built_in,
    requires_credential,
    available,
    display_order
) VALUES
    ('emisell', 'Emisell Kurir', true, false, true, 10),
    ('rajaongkir', 'RajaOngkir', false, true, true, 20),
    ('kiriminaja', 'KiriminAja', false, true, false, 30);

CREATE TABLE tenant_active_shipping_providers (
    tenant_id text PRIMARY KEY CHECK (
        tenant_id ~ '^[A-Za-z0-9._:-]{1,128}$'
    ),
    provider_code text NOT NULL REFERENCES shipping_integration_providers(code)
        ON DELETE RESTRICT,
    credential_id uuid REFERENCES provider_credentials(id) ON DELETE RESTRICT,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    updated_by text NOT NULL DEFAULT '' CHECK (length(updated_by) <= 160),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (
        (provider_code = 'emisell' AND credential_id IS NULL)
        OR
        (provider_code <> 'emisell' AND credential_id IS NOT NULL)
    )
);

CREATE INDEX tenant_active_shipping_providers_credential_idx
    ON tenant_active_shipping_providers (credential_id)
    WHERE credential_id IS NOT NULL;

CREATE OR REPLACE FUNCTION fallback_disabled_shipping_provider_credential()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF OLD.active AND NOT NEW.active THEN
        UPDATE tenant_active_shipping_providers
        SET provider_code = 'emisell',
            credential_id = NULL,
            version = version + 1,
            updated_by = 'system:credential-disabled',
            updated_at = now()
        WHERE credential_id = NEW.id;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER provider_credentials_shipping_fallback
AFTER UPDATE OF active ON provider_credentials
FOR EACH ROW
EXECUTE FUNCTION fallback_disabled_shipping_provider_credential();
