INSERT INTO locations (
    public_id,
    level,
    province,
    city,
    district,
    official_region_code
)
VALUES
    (
        'loc_test_jakarta',
        'district',
        'DKI Jakarta',
        'Jakarta Selatan',
        'Setiabudi',
        'TEST-ORIGIN'
    ),
    (
        'loc_test_bandung',
        'district',
        'Jawa Barat',
        'Bandung',
        'Coblong',
        'TEST-DESTINATION'
    )
ON CONFLICT (public_id) DO UPDATE
SET province = EXCLUDED.province,
    city = EXCLUDED.city,
    district = EXCLUDED.district,
    updated_at = now();

INSERT INTO provider_location_mappings (
    location_id,
    provider_code,
    provider_location_id,
    provider_location_name,
    granularity,
    verified_at,
    active
)
SELECT
    id,
    'rajaongkir',
    CASE public_id
        WHEN 'loc_test_jakarta' THEN '100'
        WHEN 'loc_test_bandung' THEN '200'
    END,
    city || ', ' || province,
    'district',
    now(),
    true
FROM locations
WHERE public_id IN ('loc_test_jakarta', 'loc_test_bandung')
ON CONFLICT (provider_code, provider_location_id) DO UPDATE
SET location_id = EXCLUDED.location_id,
    provider_location_name = EXCLUDED.provider_location_name,
    verified_at = now(),
    active = true,
    updated_at = now();

DELETE FROM rate_cards
WHERE source_reference = 'e2e-fixture';

INSERT INTO rate_cards (
    origin_location_id,
    destination_location_id,
    courier_service_id,
    pricing_model,
    base_price,
    rate_per_increment,
    minimum_weight_grams,
    weight_increment_grams,
    volumetric_divisor,
    rounding_profile_id,
    etd_min_days,
    etd_max_days,
    verification_status,
    source_provider,
    source_reference,
    effective_from,
    fetched_at,
    expires_at
)
SELECT
    origin.id,
    destination.id,
    service.id,
    'per_kg',
    0,
    4000,
    10000,
    1000,
    5000,
    profile.id,
    3,
    7,
    'observed',
    'test_fixture',
    'e2e-fixture',
    now() - interval '1 day',
    now(),
    now() + interval '30 days'
FROM locations origin
CROSS JOIN locations destination
CROSS JOIN courier_services service
JOIN couriers courier ON courier.id = service.courier_id
CROSS JOIN rounding_profiles profile
WHERE origin.public_id = 'loc_test_jakarta'
  AND destination.public_id = 'loc_test_bandung'
  AND courier.code = 'jne'
  AND service.code = 'JTR'
  AND profile.code = 'jne-jtr-public-2026';
