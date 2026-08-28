DROP INDEX IF EXISTS fulfillment_shipments_environment_idx;

ALTER TABLE fulfillment_shipments
    DROP COLUMN environment_code;
