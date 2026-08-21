DELETE FROM courier_service_shipping_policies
WHERE effective_from = TIMESTAMPTZ '2026-08-21 00:00:00+07'
  AND verified_at = DATE '2026-08-21';

-- Aktifkan kembali hanya versi yang ditutup oleh migration up ini.
UPDATE courier_service_shipping_policies
SET
    active = true,
    effective_until = NULL,
    updated_at = now()
WHERE effective_until = TIMESTAMPTZ '2026-08-21 00:00:00+07'
  AND (
      service_group = 'cargo'
      OR courier_service_id IN (
          SELECT service.id
          FROM courier_services service
          JOIN couriers courier ON courier.id = service.courier_id
          WHERE (courier.code, service.code) IN (
              ('anteraja', 'BIG'),
              ('jne', 'JTR'),
              ('sicepat', 'GOKIL'),
              ('wahana', 'KARGO'),
              ('rpx', 'HWP')
          )
      )
  );

-- Hapus placeholder hanya bila belum pernah diaktifkan/diobservasi provider.
DELETE FROM courier_services service
USING couriers courier
WHERE service.courier_id = courier.id
  AND courier.code = 'anteraja'
  AND service.code = 'BIG'
  AND NOT service.active
  AND service.classification_source = 'inferred_rule'
  AND service.source_reference =
      'Shopee Hemat Kargo: Anteraja Cargo; checked 2026-08-21'
  AND NOT EXISTS (
      SELECT 1 FROM rate_cards card
      WHERE card.courier_service_id = service.id
  )
  AND NOT EXISTS (
      SELECT 1 FROM courier_service_aliases alias
      WHERE alias.canonical_service_id = service.id
  );
