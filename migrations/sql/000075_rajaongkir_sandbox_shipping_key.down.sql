UPDATE shipping_integration_providers provider
SET credential_schema = (
        SELECT jsonb_agg(
            CASE
                WHEN field.value ->> 'code' = 'shipping_api_key'
                    THEN jsonb_set(
                        field.value,
                        '{environments}',
                        '["live"]'::jsonb,
                        true
                    )
                ELSE field.value
            END
            ORDER BY field.ordinality
        )
        FROM jsonb_array_elements(provider.credential_schema)
            WITH ORDINALITY AS field(value, ordinality)
    ),
    updated_at = now()
WHERE provider.code = 'rajaongkir'
  AND jsonb_typeof(provider.credential_schema) = 'array';

