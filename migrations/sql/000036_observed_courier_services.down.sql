UPDATE courier_service_aliases alias
SET canonical_service_id = NULL,
    classification_source = 'provider_observed',
    source_reference = NULL,
    updated_at = now()
FROM couriers courier
WHERE courier.code = 'anteraja'
  AND alias.courier_id = courier.id
  AND upper(alias.raw_service_code) IN ('DOK', 'ECO', 'MIC', 'ND', 'REG');

UPDATE rate_snapshots
SET canonical_service_code = '',
    service_group = 'unknown',
    service_type = 'unknown',
    service_variant_code = service_code
WHERE lower(courier_code) = 'anteraja'
  AND upper(service_code) IN ('DOK', 'ECO', 'MIC', 'ND', 'REG');

DELETE FROM courier_services service
USING couriers courier
WHERE service.courier_id = courier.id
  AND courier.code = 'anteraja'
  AND service.code IN ('DOK', 'ECO', 'MIC', 'ND', 'REG')
  AND NOT EXISTS (
      SELECT 1
      FROM tenant_shipping_service_selections selection
      WHERE selection.courier_service_id = service.id
  );
