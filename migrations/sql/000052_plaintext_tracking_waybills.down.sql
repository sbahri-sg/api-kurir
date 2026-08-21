ALTER TABLE tracking_shipments
    DROP CONSTRAINT IF EXISTS tracking_shipments_waybill_format_check;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM tracking_shipments WHERE waybill IS NOT NULL) THEN
        RAISE EXCEPTION
            'migration 000052 cannot be rolled back while plaintext waybills exist';
    END IF;
END
$$;

ALTER TABLE tracking_shipments
    ALTER COLUMN waybill_ciphertext SET NOT NULL;

ALTER TABLE tracking_shipments
    DROP COLUMN waybill;
