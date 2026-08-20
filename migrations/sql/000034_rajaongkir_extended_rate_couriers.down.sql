DELETE FROM couriers
WHERE code IN ('ncs', 'star', 'dse');

UPDATE couriers
SET provider_code = 'biteship',
    supports_domestic_cost = false,
    catalog_source = 'https://biteship.com/id/docs/api/trackings/status',
    updated_at = now()
WHERE code IN ('anteraja', 'rpx');
