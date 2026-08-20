UPDATE couriers
SET supports_tracking = true,
    tracking_provider_code = 'biteship',
    tracking_catalog_source = 'https://biteship.com/id/docs/api/trackings/overview',
    updated_at = now()
WHERE code = 'anteraja';
