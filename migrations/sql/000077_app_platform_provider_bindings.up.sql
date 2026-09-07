CREATE TABLE app_platform_provider_bindings (
 merchant_id text NOT NULL, provider_code text NOT NULL, installation_id text NOT NULL,
 app_id text NOT NULL, revision bigint NOT NULL CHECK(revision>0),
 active boolean NOT NULL, revoked boolean NOT NULL DEFAULT false,
 credential_id text NOT NULL DEFAULT '',
 PRIMARY KEY(merchant_id,provider_code,installation_id),
 CHECK(NOT(active AND revoked))
);
CREATE INDEX app_platform_provider_active ON app_platform_provider_bindings(merchant_id,provider_code) WHERE active;
