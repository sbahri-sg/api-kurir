UPDATE tracking_webhook_outbox
SET data_json = data_json
        #- '{shipment,shipped_at}'
        #- '{shipment,delivered_at}',
    updated_at = now()
WHERE status IN ('pending', 'running');
