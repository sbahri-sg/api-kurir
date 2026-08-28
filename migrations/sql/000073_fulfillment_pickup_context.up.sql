ALTER TABLE fulfillment_shipments
    ADD COLUMN package_weight_grams bigint NOT NULL DEFAULT 1000,
    ADD CONSTRAINT fulfillment_shipments_package_weight_check
        CHECK (package_weight_grams > 0 AND package_weight_grams <= 1000000);

COMMENT ON COLUMN fulfillment_shipments.package_weight_grams IS
    'Immutable, non-PII provider context used by pickup adapters; never accepted from pickup requests.';
