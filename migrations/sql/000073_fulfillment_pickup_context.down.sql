ALTER TABLE fulfillment_shipments
    DROP CONSTRAINT IF EXISTS fulfillment_shipments_package_weight_check,
    DROP COLUMN IF EXISTS package_weight_grams;
