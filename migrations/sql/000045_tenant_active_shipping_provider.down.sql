DROP TRIGGER IF EXISTS provider_credentials_shipping_fallback ON provider_credentials;
DROP FUNCTION IF EXISTS fallback_disabled_shipping_provider_credential();
DROP TABLE IF EXISTS tenant_active_shipping_providers;
DROP TABLE IF EXISTS shipping_integration_providers;
