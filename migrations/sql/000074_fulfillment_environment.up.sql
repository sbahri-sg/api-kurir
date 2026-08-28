ALTER TABLE fulfillment_shipments
    ADD COLUMN environment_code text NOT NULL DEFAULT 'live'
        CHECK (environment_code IN ('live', 'sandbox'));

CREATE INDEX fulfillment_shipments_environment_idx
    ON fulfillment_shipments (tenant_id, provider_code, environment_code, updated_at DESC);

COMMENT ON COLUMN fulfillment_shipments.environment_code IS
    'Provider execution environment selected from the merchant credential and locked for the shipment lifecycle.';
