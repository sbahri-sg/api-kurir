CREATE TABLE courier_service_shipping_policies (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    courier_service_id uuid REFERENCES courier_services(id) ON DELETE CASCADE,
    service_group text,
    weight_basis text NOT NULL DEFAULT 'provided' CHECK (
        weight_basis = 'provided'
    ),
    minimum_accepted_weight_grams bigint NOT NULL DEFAULT 1 CHECK (
        minimum_accepted_weight_grams > 0
    ),
    minimum_billable_weight_grams bigint CHECK (
        minimum_billable_weight_grams IS NULL OR
        minimum_billable_weight_grams > 0
    ),
    maximum_accepted_weight_grams bigint CHECK (
        maximum_accepted_weight_grams IS NULL OR
        maximum_accepted_weight_grams > 0
    ),
    source_type text NOT NULL CHECK (
        source_type IN (
            'official_public',
            'official_contract',
            'provider_api',
            'marketplace_reference',
            'emisell_business_policy'
        )
    ),
    source_reference text,
    verification_status text NOT NULL CHECK (
        verification_status IN (
            'official_public',
            'official_contract',
            'observed',
            'needs_contract_confirmation'
        )
    ),
    verified_at date NOT NULL,
    effective_from timestamptz NOT NULL,
    effective_until timestamptz,
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((courier_service_id IS NULL) <> (service_group IS NULL)),
    CHECK (
        service_group IS NULL OR service_group IN (
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
    CHECK (
        minimum_billable_weight_grams IS NULL OR
        minimum_billable_weight_grams >= minimum_accepted_weight_grams
    ),
    CHECK (
        maximum_accepted_weight_grams IS NULL OR
        maximum_accepted_weight_grams >= minimum_accepted_weight_grams
    ),
    CHECK (effective_until IS NULL OR effective_until > effective_from)
);

CREATE UNIQUE INDEX courier_service_shipping_policies_service_active_idx
    ON courier_service_shipping_policies (courier_service_id)
    WHERE active AND courier_service_id IS NOT NULL;

CREATE UNIQUE INDEX courier_service_shipping_policies_group_active_idx
    ON courier_service_shipping_policies (service_group)
    WHERE active AND service_group IS NOT NULL;

CREATE INDEX courier_service_shipping_policies_lookup_idx
    ON courier_service_shipping_policies (
        courier_service_id,
        service_group,
        effective_from DESC
    )
    WHERE active;

-- Pengaman Emisell: layanan cargo tidak ditawarkan untuk paket ringan ketika
-- belum ada aturan service yang lebih spesifik. Harga tetap berasal dari
-- provider karena minimum tagihan sengaja dibiarkan null pada fallback ini.
INSERT INTO courier_service_shipping_policies (
    service_group,
    minimum_accepted_weight_grams,
    source_type,
    source_reference,
    verification_status,
    verified_at,
    effective_from
) VALUES (
    'cargo',
    3000,
    'emisell_business_policy',
    'Emisell checkout cargo eligibility default',
    'official_contract',
    DATE '2026-08-20',
    TIMESTAMPTZ '2026-08-20 00:00:00+07'
);

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
        ('jne', 'JTR', 10000, 10000, NULL, 'official_public',
         'https://www.jne.co.id/jtr-indonesia', 'official_public'),
        ('tiki', 'TRC', 10000, 10000, NULL, 'official_public',
         'https://www.tiki.id/id/produk', 'official_public'),
        ('sicepat', 'GOKIL', 10000, 10000, NULL, 'official_public',
         'https://ekspres.sicepat.com/services/GOKIL', 'official_public'),
        ('wahana', 'KARGO', 10000, 10000, NULL, 'official_public',
         'https://wahana.com/syarat-ketentuan', 'official_public'),
        ('rpx', 'HWP', 20000, 20000, NULL, 'official_public',
         'https://www.rpx.co.id/service/domestic-express-en/heavy-weight-package-hwp-en-en', 'official_public'),
        ('rex', 'REX10', 10000, 10000, NULL, 'provider_api',
         'RajaOngkir provider-observed REX-10 catalog', 'observed'),
        ('lion', 'BIGPACK', 10000, 10000, NULL, 'official_public',
         'https://lionparcel.com/product/', 'official_public'),
        ('ide', 'CARGO', 10000, 10000, NULL, 'official_public',
         'https://idexpress.com/bantuan/syarat-dan-ketentuan', 'needs_contract_confirmation'),
        ('anteraja', 'MIC', 5000, 5000, NULL, 'official_public',
         'https://anteraja.id/id/services', 'needs_contract_confirmation'),
        ('anteraja', 'BIG', 3000, 5000, 100000, 'marketplace_reference',
         'https://seller.shopee.co.id/portal/all-settings/shipping/shipping-channel', 'needs_contract_confirmation'),
        ('jnt', 'HBO', 2310, 3000, 10300, 'official_public',
         'https://www.jet.co.id/information/terms/conditions', 'needs_contract_confirmation')
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
    DATE '2026-08-20',
    TIMESTAMPTZ '2026-08-20 00:00:00+07'
FROM policy_seed seed
JOIN couriers courier ON courier.code = seed.courier_code
JOIN courier_services service
  ON service.courier_id = courier.id
 AND service.code = seed.service_code;
