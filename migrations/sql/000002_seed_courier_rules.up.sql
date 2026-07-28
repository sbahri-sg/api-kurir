INSERT INTO couriers (code, name)
VALUES
    ('jne', 'JNE'),
    ('tiki', 'TIKI'),
    ('sicepat', 'SiCepat')
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name,
    updated_at = now();

INSERT INTO courier_services (courier_id, code, name, service_type, transport_mode)
SELECT id, 'REG', 'Reguler', 'parcel', 'road'
FROM couriers
WHERE code = 'jne'
ON CONFLICT (courier_id, code) DO UPDATE
SET name = EXCLUDED.name,
    service_type = EXCLUDED.service_type,
    transport_mode = EXCLUDED.transport_mode,
    updated_at = now();

INSERT INTO courier_services (courier_id, code, name, service_type, transport_mode)
SELECT id, 'JTR', 'JNE Trucking', 'cargo', 'road'
FROM couriers
WHERE code = 'jne'
ON CONFLICT (courier_id, code) DO UPDATE
SET name = EXCLUDED.name,
    service_type = EXCLUDED.service_type,
    transport_mode = EXCLUDED.transport_mode,
    updated_at = now();

INSERT INTO rounding_profiles (
    code,
    rounding_mode,
    increment_grams,
    threshold_grams,
    verification_status,
    source_reference,
    effective_from
)
VALUES (
    'jne-jtr-public-2026',
    'threshold',
    1000,
    300,
    'official_public',
    'https://www.jne.co.id/jtr-indonesia',
    DATE '2026-01-01'
)
ON CONFLICT (code) DO UPDATE
SET rounding_mode = EXCLUDED.rounding_mode,
    increment_grams = EXCLUDED.increment_grams,
    threshold_grams = EXCLUDED.threshold_grams,
    verification_status = EXCLUDED.verification_status,
    source_reference = EXCLUDED.source_reference,
    updated_at = now();
