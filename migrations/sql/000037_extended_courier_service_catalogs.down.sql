DELETE FROM courier_services service
USING couriers courier
WHERE service.courier_id = courier.id
  AND (
      (courier.code = 'dse' AND service.source_reference = 'https://www.21express.co.id/layanan-kami') OR
      (courier.code = 'ncs' AND service.source_reference = 'https://ncskurir.com/ncskurir/product-service') OR
      (courier.code = 'rpx' AND service.source_reference = 'https://www.rpx.co.id/service/domestic-express-id') OR
      (courier.code = 'star' AND service.source_reference = 'https://starcargo.co.id/pages/index/tentang-kami')
  );

UPDATE couriers
SET catalog_source = 'https://rajaongkir.com/',
    updated_at = now()
WHERE code IN ('dse', 'ncs', 'rpx', 'star');
