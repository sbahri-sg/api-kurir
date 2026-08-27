ALTER TABLE shipping_integration_providers
    DROP CONSTRAINT shipping_integration_providers_credential_type_check;

ALTER TABLE shipping_integration_providers
    ADD CONSTRAINT shipping_integration_providers_credential_type_check
        CHECK (credential_type IN (
            'none',
            'api_key',
            'capability_api_keys',
            'bearer_token',
            'api_key_secret',
            'oauth2_client_credentials',
            'provider_declared'
        )),
    ADD COLUMN credential_schema jsonb NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(credential_schema) = 'array'),
    ADD COLUMN environment_schema jsonb NOT NULL DEFAULT
        '[{"code":"live","label":"Live","description":"Operasi provider production."}]'::jsonb
        CHECK (jsonb_typeof(environment_schema) = 'array'),
    ADD COLUMN capability_environment_schema jsonb NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(capability_environment_schema) = 'array');

ALTER TABLE provider_credentials
    ADD COLUMN environment_code text NOT NULL DEFAULT 'live'
        CHECK (environment_code IN ('live', 'sandbox'));

DROP INDEX provider_credentials_one_active_per_merchant_provider_idx;

CREATE UNIQUE INDEX provider_credentials_one_active_per_merchant_provider_environment_idx
    ON provider_credentials (tenant_id, provider_code, environment_code)
    WHERE tenant_id <> '' AND active;

CREATE INDEX provider_credentials_environment_active_idx
    ON provider_credentials (provider_code, environment_code, created_at)
    WHERE active AND validation_status = 'valid';

UPDATE shipping_integration_providers
SET credential_schema = '[
      {
        "code": "shipping_api_key",
        "label": "Shipping Cost API key",
        "input_type": "password",
        "secret": true,
        "required": true,
        "placeholder": "Masukkan API key Shipping Cost",
        "help": "Dipakai untuk cek ongkir dan tracking; produk ini selalu live.",
        "capabilities": ["rates:read", "tracking:read"],
        "environments": ["live"]
      },
      {
        "code": "delivery_api_key",
        "label": "Shipping Delivery API key",
        "input_type": "password",
        "secret": true,
        "required": false,
        "placeholder": "Masukkan API key Shipping Delivery",
        "help": "Dipakai untuk order, pickup, label, detail, dan pembatalan.",
        "capabilities": ["shipments:write", "shipments:read", "pickup:write", "labels:read", "shipments:cancel"],
        "environments": ["live", "sandbox"]
      }
    ]'::jsonb,
    environment_schema = '[
      {"code":"live","label":"Live","description":"Transaksi RajaOngkir production."},
      {"code":"sandbox","label":"Sandbox","description":"Pengujian Shipping Delivery; Shipping Cost tetap live read-only."}
    ]'::jsonb,
    capability_environment_schema = '[
      {"capability":"rates","environment":"live","behavior":"live","credential_environment":"live","billing":"provider_charged"},
      {"capability":"rates","environment":"sandbox","behavior":"live_read_only","credential_environment":"live","billing":"provider_charged"},
      {"capability":"tracking","environment":"live","behavior":"live","credential_environment":"live","billing":"provider_charged"},
      {"capability":"tracking","environment":"sandbox","behavior":"live_read_only","credential_environment":"live","billing":"provider_charged"},
      {"capability":"shipments","environment":"live","behavior":"live","credential_environment":"live","billing":"provider_charged"},
      {"capability":"shipments","environment":"sandbox","behavior":"provider_sandbox","credential_environment":"sandbox","billing":"provider_defined"},
      {"capability":"pickup","environment":"live","behavior":"live","credential_environment":"live","billing":"provider_charged"},
      {"capability":"pickup","environment":"sandbox","behavior":"provider_sandbox","credential_environment":"sandbox","billing":"provider_defined"}
    ]'::jsonb,
    updated_at = now()
WHERE code = 'rajaongkir';

COMMENT ON COLUMN shipping_integration_providers.credential_schema IS
    'Safe declarative merchant credential form copied from the active partner package; an empty array uses the legacy credential_type fallback.';
COMMENT ON COLUMN shipping_integration_providers.environment_schema IS
    'Provider-declared live/sandbox environments exposed to Emisell without upstream secrets or URLs.';
COMMENT ON COLUMN shipping_integration_providers.capability_environment_schema IS
    'Per-capability execution, credential environment, and billing behavior declared by the provider package.';
COMMENT ON COLUMN provider_credentials.environment_code IS
    'Credential environment. Live remains the default for backward-compatible requests.';
