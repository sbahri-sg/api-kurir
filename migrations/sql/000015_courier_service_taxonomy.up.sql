ALTER TABLE courier_services
    ADD COLUMN service_group text NOT NULL DEFAULT 'unknown',
    ADD COLUMN classification_source text NOT NULL DEFAULT 'unknown',
    ADD COLUMN source_reference text,
    ADD COLUMN catalog_verified_at date;

ALTER TABLE courier_services
    ADD CONSTRAINT courier_services_group_check
    CHECK (
        service_group IN (
            'economy',
            'regular',
            'next_day',
            'express',
            'same_day',
            'instant',
            'cargo',
            'international',
            'special',
            'unknown'
        )
    ),
    ADD CONSTRAINT courier_services_classification_source_check
    CHECK (
        classification_source IN (
            'official_public',
            'official_contract',
            'provider_observed',
            'inferred_rule',
            'unknown'
        )
    );

CREATE TABLE courier_service_aliases (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_code text NOT NULL,
    courier_id uuid NOT NULL REFERENCES couriers(id),
    raw_service_code text NOT NULL,
    raw_service_name text NOT NULL DEFAULT '',
    canonical_service_id uuid REFERENCES courier_services(id),
    classification_source text NOT NULL DEFAULT 'unknown' CHECK (
        classification_source IN (
            'official_public',
            'official_contract',
            'provider_observed',
            'inferred_rule',
            'unknown'
        )
    ),
    source_reference text,
    first_seen_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (provider_code ~ '^[a-z0-9_-]+$'),
    CHECK (btrim(raw_service_code) <> ''),
    UNIQUE (provider_code, courier_id, raw_service_code)
);

CREATE INDEX courier_service_aliases_canonical_idx
    ON courier_service_aliases (canonical_service_id)
    WHERE active;

CREATE INDEX courier_service_aliases_observed_idx
    ON courier_service_aliases (courier_id, last_seen_at DESC);

ALTER TABLE rate_snapshots
    ADD COLUMN canonical_service_code text NOT NULL DEFAULT '',
    ADD COLUMN service_group text NOT NULL DEFAULT 'unknown',
    ADD COLUMN service_type text NOT NULL DEFAULT 'unknown',
    ADD COLUMN service_variant_code text NOT NULL DEFAULT '';

ALTER TABLE rate_snapshots
    ADD CONSTRAINT rate_snapshots_service_group_check
    CHECK (
        service_group IN (
            'economy',
            'regular',
            'next_day',
            'express',
            'same_day',
            'instant',
            'cargo',
            'international',
            'special',
            'unknown'
        )
    ),
    ADD CONSTRAINT rate_snapshots_service_type_check
    CHECK (
        service_type IN (
            'parcel',
            'cargo',
            'same_day',
            'instant',
            'international',
            'unknown'
        )
    );

WITH service_seed (
    courier_code,
    code,
    name,
    service_type,
    transport_mode,
    service_group,
    source_reference
) AS (
    VALUES
        ('jne', 'REG', 'JNE Regular', 'parcel', 'road_air', 'regular',
         'https://www.jne.co.id/produk-dan-layanan'),
        ('jne', 'OKE', 'JNE OKE', 'parcel', 'road', 'economy',
         'https://www.jne.co.id/kiriman-domestik-id'),
        ('jne', 'YES', 'JNE YES', 'parcel', 'road_air', 'next_day',
         'https://www.jne.co.id/yes'),
        ('jne', 'SPS', 'JNE Super Speed', 'parcel', 'road_air', 'express',
         'https://www.jne.co.id/super-speed-id'),
        ('jne', 'CTC', 'JNE City Courier', 'parcel', 'road', 'regular',
         'https://www.jne.co.id/produk-dan-layanan'),
        ('jne', 'JTR', 'JNE Trucking', 'cargo', 'road_sea', 'cargo',
         'https://www.jne.co.id/jtr-id'),
        ('jne', 'INT', 'JNE International Express', 'international', 'air', 'international',
         'https://www.jne.co.id/produk-dan-layanan'),
        ('jne', 'DIPLOMAT', 'JNE Diplomat', 'parcel', 'hand_carry', 'special',
         'https://www.jne.co.id/produk-dan-layanan'),
        ('jne', 'JESIKA', 'JNE Jesika', 'parcel', 'road', 'special',
         'https://www.jne.co.id/produk-dan-layanan'),

        ('tiki', 'SDS', 'TIKI Same Day Service', 'same_day', 'road_air', 'same_day',
         'https://www.tiki.id/id/produk'),
        ('tiki', 'ONS', 'TIKI Over Night Service', 'parcel', 'road_air', 'next_day',
         'https://www.tiki.id/id/produk'),
        ('tiki', 'REG', 'TIKI Regular Service', 'parcel', 'road_air', 'regular',
         'https://www.tiki.id/id/produk'),
        ('tiki', 'ECO', 'TIKI Economy Service', 'parcel', 'road', 'economy',
         'https://www.tiki.id/id/produk'),
        ('tiki', 'TRC', 'TIKI Trucking Service', 'cargo', 'road', 'cargo',
         'https://www.tiki.id/id/produk'),
        ('tiki', 'INT', 'TIKI International Service', 'international', 'air', 'international',
         'https://www.tiki.id/id/produk'),
        ('tiki', 'FROOZY', 'TIKI Froozy', 'same_day', 'cold_chain', 'special',
         'https://www.tiki.id/id/produk'),
        ('tiki', 'SRP', 'TIKI Solusi Pengiriman Ikan Prioritas', 'parcel', 'road_air', 'special',
         'https://www.tiki.id/id/produk'),
        ('tiki', 'DAT', 'TIKI Distribusi Aman Tanaman dan Buah', 'parcel', 'road_air', 'special',
         'https://www.tiki.id/id/produk'),
        ('tiki', 'TRX', 'TIKI Kirim Reptil Express', 'parcel', 'road_air', 'special',
         'https://www.tiki.id/id/produk'),

        ('sicepat', 'REGULER', 'SiCepat REGULER', 'parcel', 'road_air', 'regular',
         'https://ekspres.sicepat.com/services/reguler'),
        ('sicepat', 'BEST', 'SiCepat BEST', 'parcel', 'road_air', 'next_day',
         'https://ekspres.sicepat.com/services/reguler'),
        ('sicepat', 'HALU', 'SiCepat HALU', 'parcel', 'road', 'economy',
         'https://ekspres.sicepat.com/services/reguler'),
        ('sicepat', 'H3LO', 'SiCepat H3LO', 'parcel', 'road', 'economy',
         'https://ekspres.sicepat.com/services/reguler'),
        ('sicepat', 'GOKIL', 'SiCepat GOKIL', 'cargo', 'road_sea', 'cargo',
         'https://ekspres.sicepat.com/services/reguler'),
        ('sicepat', 'COD', 'SiCepat COD', 'parcel', 'road_air', 'special',
         'https://ekspres.sicepat.com/services/reguler'),

        ('jnt', 'EZ', 'J&T EZ', 'parcel', 'road_air', 'regular',
         'https://www.jet.co.id/information/terms/conditions'),
        ('jnt', 'ECO', 'J&T ECO', 'parcel', 'road', 'economy',
         'https://www.jet.co.id/information/terms/conditions'),
        ('jnt', 'SUPER', 'J&T SUPER', 'parcel', 'road_air', 'express',
         'https://www.jet.co.id/information/terms/conditions'),
        ('jnt', 'HBO', 'J&T HEBOH', 'cargo', 'road', 'cargo',
         'https://www.jet.co.id/information/terms/conditions'),
        ('jnt', 'DOC', 'J&T DOC', 'parcel', 'road_air', 'special',
         'https://www.jet.co.id/information/terms/conditions'),

        ('ide', 'LITE', 'IDExpress Lite', 'parcel', 'road', 'economy',
         'https://idexpress.com/'),
        ('ide', 'REG', 'IDExpress Regular', 'parcel', 'road_air', 'regular',
         'https://idexpress.com/'),
        ('ide', 'SAME_DAY', 'IDExpress Same Day', 'same_day', 'road', 'same_day',
         'https://idexpress.com/'),
        ('ide', 'CARGO', 'IDExpress Cargo', 'cargo', 'road', 'cargo',
         'https://idexpress.com/'),

        ('ninja', 'REG', 'Ninja Regular', 'parcel', 'road_air', 'regular',
         'https://www.ninjaxpress.co/id-id'),
        ('ninja', 'SAME_DAY', 'Ninja Same Day', 'same_day', 'road', 'same_day',
         'https://www.ninjaxpress.co/id-id'),
        ('ninja', 'CARGO', 'Ninja Cargo', 'cargo', 'road', 'cargo',
         'https://www.ninjaxpress.co/id-id'),
        ('ninja', 'B2BR', 'Ninja B2BR', 'cargo', 'road', 'cargo',
         'https://www.ninjaxpress.co/id-id/b2b-restock'),
        ('ninja', 'COLD', 'Ninja Cold', 'parcel', 'cold_chain', 'special',
         'https://www.ninjaxpress.co/id-id'),
        ('ninja', 'CROSS_BORDER', 'Ninja Cross Border', 'international', 'road_air', 'international',
         'https://www.ninjaxpress.co/id-id'),
        ('ninja', 'FREIGHT', 'Ninja Freight Forwarding', 'international', 'road_air_sea', 'international',
         'https://www.ninjaxpress.co/id-id'),

        ('lion', 'BOSSPACK', 'Lion Parcel BOSSPACK', 'parcel', 'road_air', 'express',
         'https://erp.lionparcel.com/'),
        ('lion', 'REGPACK', 'Lion Parcel REGPACK', 'parcel', 'road_air', 'regular',
         'https://erp.lionparcel.com/'),
        ('lion', 'JAGOPACK', 'Lion Parcel JAGOPACK', 'parcel', 'road', 'economy',
         'https://erp.lionparcel.com/'),
        ('lion', 'INTERPACK', 'Lion Parcel INTERPACK', 'international', 'air', 'international',
         'https://erp.lionparcel.com/'),
        ('lion', 'BIGPACK', 'Lion Parcel BIGPACK', 'cargo', 'road_air', 'cargo',
         'https://erp.lionparcel.com/'),

        ('pos', 'SAME_DAY', 'Pos Sameday', 'same_day', 'road', 'same_day',
         'https://www.posindonesia.co.id/id/pages/pos-reguler'),
        ('pos', 'NEXT_DAY', 'Pos Nextday', 'parcel', 'road_air', 'next_day',
         'https://www.posindonesia.co.id/id/pages/pos-reguler'),
        ('pos', 'REGULER', 'Pos Reguler', 'parcel', 'road_air', 'regular',
         'https://www.posindonesia.co.id/id/pages/pos-reguler'),
        ('pos', 'EKONOMI', 'Pos Ekonomi', 'parcel', 'road_sea', 'economy',
         'https://www.posindonesia.co.id/id/pages/pos-reguler'),
        ('pos', 'KARGO', 'Pos Kargo', 'cargo', 'road_sea', 'cargo',
         'https://www.posindonesia.co.id/id/pages/pos-reguler'),
        ('pos', 'DANGEROUS_GOODS', 'Paketpos Dangerous Goods', 'parcel', 'road', 'special',
         'https://www.posindonesia.co.id/id/pages/syarat-dan-ketentuan-kiriman-domestik'),
        ('pos', 'VALUABLE_GOODS', 'Paketpos Valuable Goods', 'parcel', 'road_air', 'special',
         'https://www.posindonesia.co.id/id/pages/syarat-dan-ketentuan-kiriman-domestik'),
        ('pos', 'EMS', 'Pos EMS', 'international', 'air', 'international',
         'https://www.posindonesia.co.id/id/pages/pos-reguler'),
        ('pos', 'POS_EKSPOR', 'Pos Ekspor', 'international', 'air', 'international',
         'https://www.posindonesia.co.id/id/pages/pos-reguler'),
        ('pos', 'E_PACKET', 'Pos e-Packet', 'international', 'air', 'international',
         'https://www.posindonesia.co.id/id/pages/pos-reguler'),

        ('wahana', 'EXPRESS', 'Wahana Express', 'parcel', 'road_air', 'regular',
         'https://wahana.com/syarat-ketentuan'),
        ('wahana', 'NEXT_DAY', 'Wahana NextDay', 'parcel', 'road_air', 'next_day',
         'https://wahana.com/syarat-ketentuan'),
        ('wahana', 'EKONOMIS', 'Wahana Ekonomis', 'parcel', 'road', 'economy',
         'https://wahana.com/syarat-ketentuan'),
        ('wahana', 'KARGO', 'Wahana Kargo', 'cargo', 'road_sea', 'cargo',
         'https://wahana.com/syarat-ketentuan'),
        ('wahana', 'INTERNASIONAL', 'Wahana Internasional', 'international', 'air', 'international',
         'https://wahana.com/syarat-ketentuan'),
        ('wahana', 'COD', 'Wahana COD', 'parcel', 'road', 'special',
         'https://wahana.com/syarat-ketentuan'),

        ('sentral', 'DARAT', 'Sentral Cargo Darat', 'cargo', 'road', 'cargo',
         'https://sentralcargo.co.id/syarat-dan-ketentuan'),
        ('sentral', 'LAUT', 'Sentral Cargo Laut', 'cargo', 'sea', 'cargo',
         'https://sentralcargo.co.id/syarat-dan-ketentuan'),
        ('sentral', 'UDARA', 'Sentral Cargo Udara', 'cargo', 'air', 'cargo',
         'https://sentralcargo.co.id/syarat-dan-ketentuan'),

        ('sap', 'REG', 'SAPX Regular', 'parcel', 'road_air', 'regular',
         'https://www.sapx.id/id'),
        ('sap', 'SDS', 'SAPX Same Day Service', 'same_day', 'road', 'same_day',
         'https://www.sapx.id/id'),
        ('sap', 'ODS', 'SAPX One Day Service', 'parcel', 'road_air', 'next_day',
         'https://www.sapx.id/id'),
        ('sap', 'CARGO', 'SAPX Cargo', 'cargo', 'road_air_sea', 'cargo',
         'https://www.sapx.id/id'),
        ('sap', 'INT', 'SAPX International', 'international', 'air', 'international',
         'https://www.sapx.id/id'),
        ('sap', 'DEDICATED', 'SAPX Dedicated Courier', 'parcel', 'dedicated', 'special',
         'https://www.sapx.id/id'),

        ('rex', 'REX0', 'REX-0 Same Day Service', 'same_day', 'road_air', 'same_day',
         'https://www.rex.co.id/en/busines-opportunities/2/to-be-our-cash-sales-counter'),
        ('rex', 'REX1', 'REX-1 Over Night Service', 'parcel', 'road_air', 'next_day',
         'https://www.rex.co.id/en/busines-opportunities/2/to-be-our-cash-sales-counter'),
        ('rex', 'EXP', 'REX Express Service', 'parcel', 'road_air', 'express',
         'https://www.rex.co.id/en/busines-opportunities/2/to-be-our-cash-sales-counter'),
        ('rex', 'REG', 'REX Regular', 'parcel', 'road_sea', 'regular',
         'https://www.rex.co.id/public/files/file/Brosur_REX.pdf'),
        ('rex', 'INT', 'REX International', 'international', 'air', 'international',
         'https://www.rex.co.id/en/busines-opportunities/2/to-be-our-cash-sales-counter'),
        ('rex', 'OTH', 'REX Other', 'cargo', 'road_sea', 'special',
         'https://www.rex.co.id/en/busines-opportunities/2/to-be-our-cash-sales-counter')
)
INSERT INTO courier_services (
    courier_id,
    code,
    name,
    service_type,
    transport_mode,
    service_group,
    classification_source,
    source_reference,
    catalog_verified_at,
    active
)
SELECT
    c.id,
    seed.code,
    seed.name,
    seed.service_type,
    seed.transport_mode,
    seed.service_group,
    'official_public',
    seed.source_reference,
    DATE '2026-07-29',
    true
FROM service_seed seed
JOIN couriers c ON c.code = seed.courier_code
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

INSERT INTO courier_services (
    courier_id,
    code,
    name,
    service_type,
    transport_mode,
    service_group,
    classification_source,
    source_reference,
    catalog_verified_at,
    active
)
SELECT
    c.id,
    'REX10',
    'REX-10',
    'cargo',
    'road_sea',
    'cargo',
    'provider_observed',
    'https://rajaongkir.com/docs/shipping-cost/endpoint-rajaongkir-for-search-base/calculate-domestic-cost',
    DATE '2026-07-29',
    true
FROM couriers c
WHERE c.code = 'rex'
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

INSERT INTO courier_service_aliases (
    provider_code,
    courier_id,
    raw_service_code,
    raw_service_name,
    canonical_service_id,
    classification_source,
    source_reference
)
SELECT
    c.provider_code,
    c.id,
    cs.code,
    cs.name,
    cs.id,
    cs.classification_source,
    cs.source_reference
FROM courier_services cs
JOIN couriers c ON c.id = cs.courier_id
WHERE cs.catalog_verified_at = DATE '2026-07-29'
ON CONFLICT (provider_code, courier_id, raw_service_code) DO UPDATE
SET raw_service_name = EXCLUDED.raw_service_name,
    canonical_service_id = EXCLUDED.canonical_service_id,
    classification_source = EXCLUDED.classification_source,
    source_reference = EXCLUDED.source_reference,
    active = true,
    updated_at = now();

WITH alias_seed (courier_code, raw_code, canonical_code, raw_name) AS (
    VALUES
        ('jne', 'SS', 'SPS', 'JNE Super Speed'),
        ('jne', 'CTCYES', 'YES', 'JNE City Courier YES'),
        ('jne', 'CTCSPS', 'SPS', 'JNE City Courier Super Speed'),
        ('jne', 'JTR<130', 'JTR', 'JNE Trucking'),
        ('jne', 'JTR>130', 'JTR', 'JNE Trucking'),
        ('jne', 'JTR>200', 'JTR', 'JNE Trucking'),
        ('tiki', 'REGULER', 'REG', 'TIKI Regular Service'),
        ('tiki', 'TRUCKING', 'TRC', 'TIKI Trucking Service'),
        ('tiki', 'T15', 'TRC', 'Motor Di Bawah 150cc/1500watt'),
        ('tiki', 'T25', 'TRC', 'Motor Di Bawah 250cc/Di Atas 1500watt'),
        ('sicepat', 'REG', 'REGULER', 'SiCepat REGULER'),
        ('jnt', 'HEBOH', 'HBO', 'J&T HEBOH'),
        ('ide', 'STD', 'REG', 'IDExpress Regular'),
        ('ide', 'REGULAR', 'REG', 'IDExpress Regular'),
        ('ide', 'Idtruck', 'CARGO', 'Idtruck'),
        ('ninja', 'STANDARD', 'REG', 'Ninja Regular'),
        ('ninja', 'SAMEDAY', 'SAME_DAY', 'Ninja Same Day'),
        ('pos', 'POS SAME DAY', 'SAME_DAY', 'Pos Sameday'),
        ('pos', 'EXPRESS NEXT DAY', 'NEXT_DAY', 'Pos Nextday'),
        ('pos', 'POS REGULER', 'REGULER', 'Pos Reguler'),
        ('pos', 'Pos Reguler', 'REGULER', 'Pos Reguler'),
        ('pos', 'POS EKONOMI', 'EKONOMI', 'Pos Ekonomi'),
        ('pos', 'PAKETPOS DANGEROUS GOODS', 'DANGEROUS_GOODS', 'Paketpos Dangerous Goods'),
        ('pos', 'PAKETPOS VALUABLE GOODS', 'VALUABLE_GOODS', 'Paketpos Valuable Goods'),
        ('wahana', 'NORMAL', 'EXPRESS', 'Wahana Express'),
        ('rex', 'REX-0', 'REX0', 'REX-0 Same Day Service'),
        ('rex', 'REX-1', 'REX1', 'REX-1 Over Night Service'),
        ('rex', 'SDS', 'REX0', 'REX-0 Same Day Service'),
        ('rex', 'ONS', 'REX1', 'REX-1 Over Night Service')
)
INSERT INTO courier_service_aliases (
    provider_code,
    courier_id,
    raw_service_code,
    raw_service_name,
    canonical_service_id,
    classification_source,
    source_reference
)
SELECT
    c.provider_code,
    c.id,
    seed.raw_code,
    seed.raw_name,
    cs.id,
    'official_public',
    cs.source_reference
FROM alias_seed seed
JOIN couriers c ON c.code = seed.courier_code
JOIN courier_services cs
  ON cs.courier_id = c.id
 AND cs.code = seed.canonical_code
ON CONFLICT (provider_code, courier_id, raw_service_code) DO UPDATE
SET raw_service_name = EXCLUDED.raw_service_name,
    canonical_service_id = EXCLUDED.canonical_service_id,
    classification_source = EXCLUDED.classification_source,
    source_reference = EXCLUDED.source_reference,
    active = true,
    updated_at = now();

WITH observed_alias_seed (
    courier_code,
    raw_code,
    canonical_code,
    raw_name
) AS (
    VALUES
        ('tiki', 'T60', 'TRC', 'Motor Di Bawah 600cc/Non Standar/Roda 3'),
        ('sap', 'UDRREG', 'REG', 'Reguler'),
        ('sap', 'UDRONS', 'ODS', 'Nextday'),
        ('sap', 'DRGREG', 'CARGO', 'Cargo'),
        ('rex', 'REX-10', 'REX10', 'Rex-10 ( Harga Ekonomis Mulai 10 Kg )')
)
INSERT INTO courier_service_aliases (
    provider_code,
    courier_id,
    raw_service_code,
    raw_service_name,
    canonical_service_id,
    classification_source,
    source_reference
)
SELECT
    c.provider_code,
    c.id,
    seed.raw_code,
    seed.raw_name,
    cs.id,
    'inferred_rule',
    'https://rajaongkir.com/docs/shipping-cost/endpoint-rajaongkir-for-search-base/calculate-domestic-cost'
FROM observed_alias_seed seed
JOIN couriers c ON c.code = seed.courier_code
JOIN courier_services cs
  ON cs.courier_id = c.id
 AND cs.code = seed.canonical_code
ON CONFLICT (provider_code, courier_id, raw_service_code) DO UPDATE
SET raw_service_name = EXCLUDED.raw_service_name,
    canonical_service_id = EXCLUDED.canonical_service_id,
    classification_source = EXCLUDED.classification_source,
    source_reference = EXCLUDED.source_reference,
    active = true,
    updated_at = now();

UPDATE rate_snapshots
SET canonical_service_code = CASE
        WHEN upper(service_code) = 'CTCYES' THEN 'YES'
        WHEN upper(service_code) = 'CTCSPS' THEN 'SPS'
        WHEN upper(service_code) = 'CTC' THEN 'CTC'
        WHEN upper(service_code) LIKE 'JTR%' THEN 'JTR'
        WHEN upper(service_code) LIKE 'REG%' THEN 'REG'
        WHEN upper(service_code) LIKE 'YES%' THEN 'YES'
        WHEN upper(service_code) IN ('SPS', 'SS') THEN 'SPS'
        WHEN upper(service_code) LIKE 'OKE%' THEN 'OKE'
        ELSE ''
    END,
    service_group = CASE
        WHEN upper(service_code) = 'CTCYES' OR upper(service_code) LIKE 'YES%' THEN 'next_day'
        WHEN upper(service_code) = 'CTCSPS' OR upper(service_code) IN ('SPS', 'SS') THEN 'express'
        WHEN upper(service_code) = 'CTC' OR upper(service_code) LIKE 'REG%' THEN 'regular'
        WHEN upper(service_code) LIKE 'JTR%' THEN 'cargo'
        WHEN upper(service_code) LIKE 'OKE%' THEN 'economy'
        ELSE 'unknown'
    END,
    service_type = CASE
        WHEN upper(service_code) LIKE 'JTR%' THEN 'cargo'
        WHEN upper(service_code) IN ('CTCYES', 'CTCSPS', 'CTC')
          OR upper(service_code) LIKE 'REG%'
          OR upper(service_code) LIKE 'YES%'
          OR upper(service_code) IN ('SPS', 'SS')
          OR upper(service_code) LIKE 'OKE%'
        THEN 'parcel'
        ELSE 'unknown'
    END,
    service_variant_code = CASE
        WHEN upper(service_code) IN ('REG', 'YES', 'SPS', 'OKE', 'CTC', 'JTR') THEN ''
        ELSE service_code
    END
WHERE lower(courier_code) = 'jne';

UPDATE rate_snapshots rs
SET canonical_service_code = cs.code,
    service_group = cs.service_group,
    service_type = cs.service_type,
    service_variant_code = CASE
        WHEN upper(regexp_replace(rs.service_code, '[^A-Za-z0-9]', '', 'g'))
           = upper(regexp_replace(cs.code, '[^A-Za-z0-9]', '', 'g'))
        THEN ''
        ELSE rs.service_code
    END
FROM couriers c
JOIN courier_service_aliases alias
  ON alias.courier_id = c.id
 AND alias.active
JOIN courier_services cs
  ON cs.id = alias.canonical_service_id
WHERE lower(rs.courier_code) = c.code
  AND rs.provider_code = alias.provider_code
  AND upper(rs.service_code) = upper(alias.raw_service_code);
