ALTER TABLE couriers
    ADD COLUMN rate_provider_code text,
    ADD COLUMN tracking_provider_code text,
    ADD COLUMN tracking_catalog_source text;

-- Seluruh cek ongkir aktif tetap bersumber dari RajaOngkir. Biteship tidak
-- pernah menjadi rate provider pada fase ini.
UPDATE couriers
SET rate_provider_code = CASE
        WHEN supports_domestic_cost OR supports_international_cost
            THEN 'rajaongkir'
        ELSE NULL
    END,
    catalog_source = CASE
        WHEN supports_domestic_cost OR supports_international_cost
            THEN 'https://rajaongkir.com/'
        ELSE catalog_source
    END,
    updated_at = now()
WHERE active;

UPDATE couriers
SET catalog_source = 'https://biteship.com/id/docs/api/couriers/overview',
    updated_at = now()
WHERE code = 'paxel';

-- Capability AWB resmi RajaOngkir.
UPDATE couriers
SET supports_tracking = true,
    tracking_provider_code = 'rajaongkir',
    tracking_catalog_source =
        'https://www.rajaongkir.com/docs/shipping-cost/getting_started/courier_availability',
    updated_at = now()
WHERE code IN ('jne', 'sap', 'ninja', 'jnt', 'tiki', 'wahana', 'pos', 'lion');

-- Biteship hanya menutup celah tracking yang tidak dicakup RajaOngkir.
UPDATE couriers
SET supports_tracking = true,
    tracking_provider_code = 'biteship',
    tracking_catalog_source =
        'https://biteship.com/id/docs/api/trackings/overview',
    updated_at = now()
WHERE code IN ('anteraja', 'ide', 'paxel', 'rpx', 'sentral', 'sicepat');

-- Kurir lain tetap tersedia untuk cek ongkir RajaOngkir, tetapi tidak
-- ditampilkan pada cek resi sampai ada adapter tracking yang terverifikasi.
UPDATE couriers
SET supports_tracking = false,
    tracking_provider_code = NULL,
    tracking_catalog_source = NULL,
    updated_at = now()
WHERE code NOT IN (
    'jne', 'sap', 'ninja', 'jnt', 'tiki', 'wahana', 'pos', 'lion',
    'anteraja', 'ide', 'paxel', 'rpx', 'sentral', 'sicepat'
);

-- J&T Cargo sebelumnya ditambahkan berdasarkan asumsi. Kode tersebut tidak
-- ada pada katalog Biteship yang diverifikasi, sehingga tidak boleh muncul.
DELETE FROM couriers
WHERE code = 'jntcargo';

ALTER TABLE couriers
    ADD CONSTRAINT couriers_rate_provider_code_check
    CHECK (
        rate_provider_code IS NULL OR
        rate_provider_code ~ '^[a-z0-9_-]+$'
    ),
    ADD CONSTRAINT couriers_tracking_provider_code_check
    CHECK (
        tracking_provider_code IS NULL OR
        tracking_provider_code ~ '^[a-z0-9_-]+$'
    ),
    ADD CONSTRAINT couriers_tracking_capability_check
    CHECK (
        (supports_tracking AND tracking_provider_code IS NOT NULL) OR
        (NOT supports_tracking AND tracking_provider_code IS NULL)
    );
