-- Versi pertama SOP kelayakan berat checkout Emisell.
--
-- Policy marketplace_reference mengatur layanan yang boleh ditampilkan pada
-- kanal Emisell. Nilai tersebut bukan pengganti kontrak komersial provider;
-- harga final tetap berasal dari exact quote provider.

-- Tutup policy grup cargo lama agar seluruh fallback grup mempunyai versi dan
-- masa berlaku yang konsisten.
UPDATE courier_service_shipping_policies
SET
    active = false,
    effective_until = TIMESTAMPTZ '2026-08-21 00:00:00+07',
    updated_at = now()
WHERE active
  AND service_group = 'cargo';

-- Paket parcel yang melebihi 50 kg tidak ditampilkan secara default. Policy
-- service yang lebih spesifik tetap dapat mengalahkan fallback grup ini.
INSERT INTO courier_service_shipping_policies (
    service_group,
    minimum_accepted_weight_grams,
    minimum_billable_weight_grams,
    maximum_accepted_weight_grams,
    source_type,
    source_reference,
    verification_status,
    verified_at,
    effective_from
) VALUES
    (
        'regular', 1, NULL, 50000,
        'emisell_business_policy',
        'API Kurir SOP kelayakan berat checkout v1',
        'observed', DATE '2026-08-21',
        TIMESTAMPTZ '2026-08-21 00:00:00+07'
    ),
    (
        'economy', 1, NULL, 50000,
        'emisell_business_policy',
        'API Kurir SOP kelayakan berat checkout v1',
        'observed', DATE '2026-08-21',
        TIMESTAMPTZ '2026-08-21 00:00:00+07'
    ),
    (
        'next_day', 1, NULL, 50000,
        'emisell_business_policy',
        'API Kurir SOP kelayakan berat checkout v1',
        'observed', DATE '2026-08-21',
        TIMESTAMPTZ '2026-08-21 00:00:00+07'
    ),
    (
        'cargo', 3000, NULL, NULL,
        'emisell_business_policy',
        'API Kurir SOP kelayakan berat checkout v1',
        'observed', DATE '2026-08-21',
        TIMESTAMPTZ '2026-08-21 00:00:00+07'
    );

-- BIG sudah dikenali classifier sebagai Anteraja Cargo, tetapi belum selalu
-- muncul pada respons RajaOngkir. Simpan sebagai canonical placeholder nonaktif
-- agar policy tersedia saat code tersebut pertama kali diobservasi. Katalog
-- admin tetap menyembunyikannya sampai provider benar-benar mengembalikannya.
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
    'BIG',
    'Anteraja Cargo',
    'cargo',
    'road',
    false,
    'cargo',
    'inferred_rule',
    'Shopee Hemat Kargo: Anteraja Cargo; checked 2026-08-21',
    DATE '2026-08-21'
FROM couriers courier
WHERE courier.code = 'anteraja'
ON CONFLICT (courier_id, code) DO NOTHING;

-- Policy berikut berubah dari seed sebelumnya atau mendapat batas yang lebih
-- lengkap. Simpan versi lama sebagai histori, jangan overwrite nilainya.
WITH target (courier_code, service_code) AS (
    VALUES
        ('anteraja', 'BIG'),
        ('jne', 'JTR'),
        ('sicepat', 'GOKIL'),
        ('wahana', 'KARGO'),
        ('rpx', 'HWP')
)
UPDATE courier_service_shipping_policies policy
SET
    active = false,
    effective_until = TIMESTAMPTZ '2026-08-21 00:00:00+07',
    updated_at = now()
FROM courier_services service
JOIN couriers courier ON courier.id = service.courier_id
JOIN target ON target.courier_code = courier.code
           AND target.service_code = service.code
WHERE policy.courier_service_id = service.id
  AND policy.active;

WITH policy_seed (
    courier_code,
    service_code,
    minimum_accepted_weight_grams,
    minimum_billable_weight_grams,
    maximum_accepted_weight_grams,
    source_type,
    source_reference,
    verification_status
) AS (
    VALUES
        -- Aturan kanal Shopee dipakai sebagai referensi eligibility Emisell.
        -- Exact quote provider tetap menentukan harga yang dibayar.
        ('anteraja', 'BIG', 3000, 5000, 100000,
         'marketplace_reference',
         'Shopee Hemat Kargo: Anteraja Cargo; checked 2026-08-21',
         'needs_contract_confirmation'),
        ('jne', 'JTR', 3000, 5000, 600000,
         'marketplace_reference',
         'Shopee Hemat Kargo: JNE Trucking (JTR); checked 2026-08-21',
         'needs_contract_confirmation'),
        ('sicepat', 'GOKIL', 3000, 5000, 50000,
         'marketplace_reference',
         'Shopee Hemat Kargo: SiCepat GOKIL; checked 2026-08-21',
         'needs_contract_confirmation'),

        -- Batas resmi yang melengkapi policy lama.
        ('wahana', 'KARGO', 10000, 10000, 50000,
         'official_public',
         'https://wahana.com/syarat-ketentuan',
         'official_public'),
        ('rpx', 'HWP', 20000, 20000, 50000,
         'official_public',
         'https://www.rpx.co.id/service/domestic-express-en/heavy-weight-package-hwp-en-en',
         'official_public'),

        -- Service yang belum mempunyai policy khusus pada seed awal.
        ('sentral', 'DARAT', 5000, NULL, NULL,
         'marketplace_reference',
         'Shopee Hemat Kargo: Sentral Cargo; checked 2026-08-21',
         'needs_contract_confirmation'),
        ('sentral', 'LAUT', 5000, NULL, NULL,
         'marketplace_reference',
         'Shopee Hemat Kargo: Sentral Cargo; checked 2026-08-21',
         'needs_contract_confirmation'),
        ('sentral', 'UDARA', 5000, NULL, NULL,
         'marketplace_reference',
         'Shopee Hemat Kargo: Sentral Cargo; checked 2026-08-21',
         'needs_contract_confirmation'),
        ('sap', 'CARGO', 5000, NULL, NULL,
         'official_public',
         'SAPX Cargo public service information; checked 2026-08-21',
         'needs_contract_confirmation'),
        ('ninja', 'REG', 1, NULL, 30000,
         'official_public',
         'Ninja Xpress Indonesia standard parcel terms; checked 2026-08-21',
         'needs_contract_confirmation')
)
INSERT INTO courier_service_shipping_policies (
    courier_service_id,
    minimum_accepted_weight_grams,
    minimum_billable_weight_grams,
    maximum_accepted_weight_grams,
    source_type,
    source_reference,
    verification_status,
    verified_at,
    effective_from
)
SELECT
    service.id,
    seed.minimum_accepted_weight_grams,
    seed.minimum_billable_weight_grams,
    seed.maximum_accepted_weight_grams,
    seed.source_type,
    seed.source_reference,
    seed.verification_status,
    DATE '2026-08-21',
    TIMESTAMPTZ '2026-08-21 00:00:00+07'
FROM policy_seed seed
JOIN couriers courier ON courier.code = seed.courier_code
JOIN courier_services service
  ON service.courier_id = courier.id
 AND service.code = seed.service_code;
