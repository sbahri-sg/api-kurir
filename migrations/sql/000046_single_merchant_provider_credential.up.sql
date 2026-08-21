WITH ranked AS (
    SELECT
        credential.id,
        row_number() OVER (
            PARTITION BY credential.tenant_id, credential.provider_code
            ORDER BY
                (selection.credential_id = credential.id) DESC NULLS LAST,
                credential.created_at DESC,
                credential.id DESC
        ) AS position
    FROM provider_credentials credential
    LEFT JOIN tenant_active_shipping_providers selection
      ON selection.tenant_id = credential.tenant_id
     AND selection.provider_code = credential.provider_code
    WHERE credential.tenant_id <> ''
      AND credential.active
)
UPDATE provider_credentials credential
SET active = false,
    disabled_at = coalesce(credential.disabled_at, now()),
    updated_at = now()
FROM ranked
WHERE credential.id = ranked.id
  AND ranked.position > 1;

CREATE UNIQUE INDEX provider_credentials_one_active_per_merchant_provider_idx
    ON provider_credentials (tenant_id, provider_code)
    WHERE tenant_id <> '' AND active;
