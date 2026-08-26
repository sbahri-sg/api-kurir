UPDATE tracking_webhook_outbox outbox
SET data_json = jsonb_set(
        jsonb_set(
            outbox.data_json,
            '{shipment,shipped_at}',
            coalesce(to_jsonb(shipment.shipped_at), 'null'::jsonb),
            true
        ),
        '{shipment,delivered_at}',
        coalesce(to_jsonb(shipment.delivered_at), 'null'::jsonb),
        true
    ),
    updated_at = now()
FROM tracking_shipments shipment
WHERE shipment.id = outbox.shipment_id
  AND outbox.status IN ('pending', 'running');
