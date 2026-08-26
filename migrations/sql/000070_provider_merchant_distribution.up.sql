ALTER TABLE shipping_integration_providers
    DROP CONSTRAINT shipping_integration_providers_distribution_type_check;

UPDATE shipping_integration_providers
SET distribution_type = CASE
    WHEN built_in THEN 'built_in'
    ELSE 'merchant'
END,
updated_at = now();

ALTER TABLE shipping_integration_providers
    ADD CONSTRAINT shipping_integration_providers_distribution_type_check
        CHECK (distribution_type IN ('built_in', 'merchant')),
    ADD CONSTRAINT shipping_integration_providers_classification_check_v2
        CHECK (
            (built_in AND integration_type = 'built_in' AND distribution_type = 'built_in')
            OR
            (NOT built_in AND integration_type <> 'built_in' AND distribution_type = 'merchant')
        );
