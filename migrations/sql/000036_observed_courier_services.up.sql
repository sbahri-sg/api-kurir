WITH service_seed (
    code,
    name,
    service_type,
    service_group
) AS (
    VALUES
        ('DOK', 'Anteraja Document', 'parcel', 'regular'),
        ('ECO', 'Anteraja Economy', 'parcel', 'economy'),
        ('MIC', 'Anteraja Mini Cargo', 'cargo', 'cargo'),
        ('ND', 'Anteraja Next Day', 'parcel', 'next_day'),
        ('REG', 'Anteraja Regular', 'parcel', 'regular')
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
    c.id,
    seed.code,
    seed.name,
    seed.service_type,
    CASE WHEN seed.service_type = 'cargo' THEN 'road' ELSE 'road_air' END,
    true,
    seed.service_group,
    'official_public',
    'https://anteraja.id/id/services',
    DATE '2026-08-20'
FROM service_seed seed
JOIN couriers c ON c.code = 'anteraja'
ON CONFLICT (courier_id, code) DO UPDATE
SET name = EXCLUDED.name,
    service_type = EXCLUDED.service_type,
    service_group = EXCLUDED.service_group,
    classification_source = EXCLUDED.classification_source,
    source_reference = EXCLUDED.source_reference,
    catalog_verified_at = EXCLUDED.catalog_verified_at,
    active = true,
    updated_at = now();

UPDATE courier_service_aliases alias
SET canonical_service_id = service.id,
    classification_source = 'official_public',
    source_reference = 'https://anteraja.id/id/services',
    active = true,
    updated_at = now()
FROM couriers courier
JOIN courier_services service
  ON service.courier_id = courier.id
WHERE courier.code = 'anteraja'
  AND alias.courier_id = courier.id
  AND upper(alias.raw_service_code) = service.code;

UPDATE rate_snapshots snapshot
SET canonical_service_code = service.code,
    service_group = service.service_group,
    service_type = service.service_type,
    service_variant_code = CASE
        WHEN upper(snapshot.service_code) = service.code THEN ''
        ELSE snapshot.service_code
    END
FROM couriers courier
JOIN courier_services service
  ON service.courier_id = courier.id
WHERE courier.code = 'anteraja'
  AND lower(snapshot.courier_code) = courier.code
  AND upper(snapshot.service_code) = service.code;
