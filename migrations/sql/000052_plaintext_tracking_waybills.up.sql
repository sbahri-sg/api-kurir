ALTER TABLE tracking_shipments
    ADD COLUMN waybill text;

ALTER TABLE tracking_shipments
    ALTER COLUMN waybill_ciphertext DROP NOT NULL;

ALTER TABLE tracking_shipments
    ADD CONSTRAINT tracking_shipments_waybill_format_check CHECK (
        waybill IS NULL OR waybill ~ '^[A-Z0-9-]{6,40}$'
    );

COMMENT ON COLUMN tracking_shipments.waybill IS
    'Normalized plaintext AWB used by the Emisell backend and tracking webhooks.';

COMMENT ON COLUMN tracking_shipments.waybill_ciphertext IS
    'Legacy encrypted AWB; cleared after application-level migration.';
