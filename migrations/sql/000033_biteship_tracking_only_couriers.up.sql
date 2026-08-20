INSERT INTO couriers (
    code,
    name,
    provider_code,
    supports_domestic_cost,
    supports_international_cost,
    supports_tracking,
    catalog_source,
    catalog_verified_at,
    active
)
VALUES
    ('anteraja', 'AnterAja', 'rajaongkir', true, false, true,
     'https://rajaongkir.com/; https://biteship.com/id/docs/api/trackings/status',
     DATE '2026-08-20', true),
    ('paxel', 'Paxel', 'biteship', false, false, true,
     'https://biteship.com/id/docs/api/couriers/overview',
     DATE '2026-08-20', true),
    ('rpx', 'RPX', 'rajaongkir', true, false, true,
     'https://rajaongkir.com/; https://biteship.com/id/docs/api/trackings/status',
     DATE '2026-08-20', true)
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name,
    provider_code = EXCLUDED.provider_code,
    supports_domestic_cost = EXCLUDED.supports_domestic_cost,
    supports_tracking = true,
    catalog_source = CASE
        WHEN position(
            EXCLUDED.catalog_source IN coalesce(couriers.catalog_source, '')
        ) > 0 THEN couriers.catalog_source
        ELSE concat_ws(
            '; ',
            nullif(couriers.catalog_source, ''),
            EXCLUDED.catalog_source
        )
    END,
    catalog_verified_at = EXCLUDED.catalog_verified_at,
    active = true,
    updated_at = now();
