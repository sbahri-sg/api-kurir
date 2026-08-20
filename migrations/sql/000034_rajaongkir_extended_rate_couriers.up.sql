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
    ('rpx', 'RPX', 'rajaongkir', true, false, true,
     'https://rajaongkir.com/; https://biteship.com/id/docs/api/trackings/status',
     DATE '2026-08-20', true),
    ('ncs', 'NCS', 'rajaongkir', true, false, false,
     'https://rajaongkir.com/', DATE '2026-08-20', true),
    ('star', 'STAR Cargo', 'rajaongkir', true, false, false,
     'https://rajaongkir.com/', DATE '2026-08-20', true),
    ('dse', 'DSE', 'rajaongkir', true, false, false,
     'https://rajaongkir.com/', DATE '2026-08-20', true)
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name,
    provider_code = EXCLUDED.provider_code,
    supports_domestic_cost = true,
    supports_tracking = EXCLUDED.supports_tracking,
    catalog_source = EXCLUDED.catalog_source,
    catalog_verified_at = EXCLUDED.catalog_verified_at,
    active = true,
    updated_at = now();
