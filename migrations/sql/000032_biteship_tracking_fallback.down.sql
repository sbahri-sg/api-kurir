UPDATE couriers
SET supports_tracking = false,
    catalog_source = 'https://rajaongkir.com/docs/shipping-cost/getting_started/courier_availability',
    catalog_verified_at = DATE '2026-07-28',
    updated_at = now()
WHERE code IN ('sicepat', 'ide', 'sentral');
