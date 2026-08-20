UPDATE couriers
SET supports_tracking = true,
    tracking_provider_code = 'rajaongkir',
    tracking_catalog_source = 'https://rajaongkir.com/lacak-resi',
    updated_at = now()
WHERE code = 'anteraja';
