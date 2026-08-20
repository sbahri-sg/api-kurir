WITH service_seed (
    courier_code,
    code,
    name,
    service_type,
    service_group,
    transport_mode,
    source_reference
) AS (
    VALUES
        ('dse', 'REG', '21 Express Regular', 'parcel', 'regular', 'road_air', 'https://www.21express.co.id/layanan-kami'),
        ('dse', 'ONS', '21 Express Over Night Service', 'parcel', 'next_day', 'air', 'https://www.21express.co.id/layanan-kami'),
        ('dse', 'SDS', '21 Express Same Day Service', 'same_day', 'same_day', 'air', 'https://www.21express.co.id/layanan-kami'),
        ('dse', 'INT', '21 Express International', 'international', 'international', 'air', 'https://www.21express.co.id/layanan-kami'),
        ('dse', 'CITY', '21 Express City Courier', 'parcel', 'regular', 'road', 'https://www.21express.co.id/layanan-kami'),

        ('ncs', 'REG', 'NCS Regular', 'parcel', 'regular', 'road_air', 'https://ncskurir.com/ncskurir/product-service'),
        ('ncs', 'ONS', 'NCS Overnight', 'parcel', 'next_day', 'road_air', 'https://ncskurir.com/ncskurir/product-service'),
        ('ncs', 'SDS', 'NCS Same Day', 'same_day', 'same_day', 'road', 'https://ncskurir.com/ncskurir/product-service'),
        ('ncs', 'DARAT', 'NCS Regular Darat', 'cargo', 'cargo', 'road', 'https://ncskurir.com/ncskurir/product-service'),
        ('ncs', 'NFD', 'Nusantara Food Delivery', 'parcel', 'special', 'road', 'https://ncskurir.com/ncskurir/product-service'),
        ('ncs', 'INT', 'NCS International Express', 'international', 'international', 'air', 'https://ncskurir.com/ncskurir/product-service'),

        ('rpx', 'SDP', 'RPX Same Day Package', 'same_day', 'same_day', 'road_air', 'https://www.rpx.co.id/service/domestic-express-id'),
        ('rpx', 'MDP', 'RPX Midday Package', 'parcel', 'next_day', 'road_air', 'https://www.rpx.co.id/service/domestic-express-id'),
        ('rpx', 'NDP', 'RPX Next Day Package', 'parcel', 'next_day', 'road_air', 'https://www.rpx.co.id/service/domestic-express-id'),
        ('rpx', 'RGP', 'RPX Regular Package', 'parcel', 'regular', 'road_air', 'https://www.rpx.co.id/service/domestic-express-id'),
        ('rpx', 'HWP', 'RPX Heavy Weight Package', 'cargo', 'cargo', 'road', 'https://www.rpx.co.id/service/domestic-express-id'),
        ('rpx', 'ECP', 'RPX Economy Package', 'parcel', 'economy', 'road_air', 'https://www.rpx.co.id/service/domestic-express-id'),
        ('rpx', 'HCP', 'RPX Hand Carry Package', 'parcel', 'next_day', 'road_air', 'https://www.rpx.co.id/service/domestic-express-id'),

        ('star', 'CARGO', 'STAR Cargo', 'cargo', 'cargo', 'multimodal', 'https://starcargo.co.id/pages/index/tentang-kami'),
        ('star', 'UDARA', 'STAR Cargo Udara', 'cargo', 'cargo', 'air', 'https://starcargo.co.id/pages/index/tentang-kami'),
        ('star', 'DARAT', 'STAR Cargo Darat', 'cargo', 'cargo', 'road', 'https://starcargo.co.id/pages/index/tentang-kami'),
        ('star', 'LAUT', 'STAR Cargo Laut', 'cargo', 'cargo', 'sea', 'https://starcargo.co.id/pages/index/tentang-kami')
)
INSERT INTO courier_services (
    courier_id,
    code,
    name,
    service_type,
    transport_mode,
    active,
    service_group,
    classification_source,
    source_reference,
    catalog_verified_at
)
SELECT
    courier.id,
    seed.code,
    seed.name,
    seed.service_type,
    seed.transport_mode,
    true,
    seed.service_group,
    'official_public',
    seed.source_reference,
    DATE '2026-08-20'
FROM service_seed seed
JOIN couriers courier ON courier.code = seed.courier_code
ON CONFLICT (courier_id, code) DO UPDATE
SET name = EXCLUDED.name,
    service_type = EXCLUDED.service_type,
    transport_mode = EXCLUDED.transport_mode,
    service_group = EXCLUDED.service_group,
    classification_source = EXCLUDED.classification_source,
    source_reference = EXCLUDED.source_reference,
    catalog_verified_at = EXCLUDED.catalog_verified_at,
    active = true,
    updated_at = now();

UPDATE couriers
SET catalog_source = CASE code
        WHEN 'dse' THEN 'https://www.21express.co.id/layanan-kami'
        WHEN 'ncs' THEN 'https://ncskurir.com/ncskurir/product-service'
        WHEN 'rpx' THEN 'https://www.rpx.co.id/service/domestic-express-id'
        WHEN 'star' THEN 'https://starcargo.co.id/pages/index/tentang-kami'
        ELSE catalog_source
    END,
    catalog_verified_at = DATE '2026-08-20',
    updated_at = now()
WHERE code IN ('dse', 'ncs', 'rpx', 'star');
