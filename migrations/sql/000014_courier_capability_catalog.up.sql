ALTER TABLE couriers
    ADD COLUMN provider_code text NOT NULL DEFAULT 'rajaongkir',
    ADD COLUMN supports_domestic_cost boolean NOT NULL DEFAULT false,
    ADD COLUMN supports_international_cost boolean NOT NULL DEFAULT false,
    ADD COLUMN supports_tracking boolean NOT NULL DEFAULT false,
    ADD COLUMN catalog_source text,
    ADD COLUMN catalog_verified_at date;

ALTER TABLE couriers
    ADD CONSTRAINT couriers_provider_code_check
    CHECK (provider_code ~ '^[a-z0-9_-]+$');

INSERT INTO couriers (
    code,
    name,
    provider_code,
    supports_domestic_cost,
    supports_international_cost,
    supports_tracking,
    catalog_source,
    catalog_verified_at,
    active
)
VALUES
    ('jne', 'JNE', 'rajaongkir', true, true, true,
     'https://rajaongkir.com/docs/shipping-cost/getting_started/courier_availability',
     DATE '2026-07-28', true),
    ('sicepat', 'SiCepat', 'rajaongkir', true, false, false,
     'https://rajaongkir.com/docs/shipping-cost/getting_started/courier_availability',
     DATE '2026-07-28', true),
    ('ide', 'IDExpress', 'rajaongkir', true, false, false,
     'https://rajaongkir.com/docs/shipping-cost/getting_started/courier_availability',
     DATE '2026-07-28', true),
    ('sap', 'SAP Express', 'rajaongkir', true, false, true,
     'https://rajaongkir.com/docs/shipping-cost/getting_started/courier_availability',
     DATE '2026-07-28', true),
    ('ninja', 'Ninja Xpress', 'rajaongkir', true, false, true,
     'https://rajaongkir.com/docs/shipping-cost/getting_started/courier_availability',
     DATE '2026-07-28', true),
    ('jnt', 'J&T Express', 'rajaongkir', true, false, true,
     'https://rajaongkir.com/docs/shipping-cost/getting_started/courier_availability',
     DATE '2026-07-28', true),
    ('tiki', 'TIKI', 'rajaongkir', true, true, true,
     'https://rajaongkir.com/docs/shipping-cost/getting_started/courier_availability',
     DATE '2026-07-28', true),
    ('wahana', 'Wahana Express', 'rajaongkir', true, false, true,
     'https://rajaongkir.com/docs/shipping-cost/getting_started/courier_availability',
     DATE '2026-07-28', true),
    ('pos', 'POS Indonesia', 'rajaongkir', true, true, true,
     'https://rajaongkir.com/docs/shipping-cost/getting_started/courier_availability',
     DATE '2026-07-28', true),
    ('sentral', 'Sentral Cargo', 'rajaongkir', true, false, false,
     'https://rajaongkir.com/docs/shipping-cost/getting_started/courier_availability',
     DATE '2026-07-28', true),
    ('lion', 'Lion Parcel', 'rajaongkir', true, false, true,
     'https://rajaongkir.com/docs/shipping-cost/getting_started/courier_availability',
     DATE '2026-07-28', true),
    ('rex', 'Royal Express Asia', 'rajaongkir', true, false, false,
     'https://rajaongkir.com/docs/shipping-cost/getting_started/courier_availability',
     DATE '2026-07-28', true)
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name,
    provider_code = EXCLUDED.provider_code,
    supports_domestic_cost = EXCLUDED.supports_domestic_cost,
    supports_international_cost = EXCLUDED.supports_international_cost,
    supports_tracking = EXCLUDED.supports_tracking,
    catalog_source = EXCLUDED.catalog_source,
    catalog_verified_at = EXCLUDED.catalog_verified_at,
    active = true,
    updated_at = now();
