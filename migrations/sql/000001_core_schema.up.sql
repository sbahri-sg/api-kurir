CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE locations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    public_id text NOT NULL UNIQUE,
    parent_id uuid REFERENCES locations(id),
    level text NOT NULL CHECK (level IN ('province', 'city', 'district', 'subdistrict')),
    province text,
    city text,
    district text,
    subdistrict text,
    postal_code text,
    official_region_code text,
    active boolean NOT NULL DEFAULT true,
    search_text text GENERATED ALWAYS AS (
        lower(
            coalesce(subdistrict, '') || ' ' ||
            coalesce(district, '') || ' ' ||
            coalesce(city, '') || ' ' ||
            coalesce(province, '') || ' ' ||
            coalesce(postal_code, '')
        )
    ) STORED,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX locations_search_trgm_idx ON locations USING gin (search_text gin_trgm_ops);
CREATE INDEX locations_active_level_idx ON locations (active, level);
CREATE INDEX locations_official_region_idx ON locations (official_region_code);

CREATE TABLE provider_location_mappings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    location_id uuid NOT NULL REFERENCES locations(id),
    provider_code text NOT NULL,
    provider_location_id text NOT NULL,
    provider_location_name text,
    granularity text NOT NULL,
    verified_at timestamptz,
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider_code, provider_location_id),
    UNIQUE (location_id, provider_code, granularity)
);

CREATE INDEX provider_location_mapping_lookup_idx
    ON provider_location_mappings (location_id, provider_code)
    WHERE active;

CREATE TABLE couriers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code text NOT NULL UNIQUE,
    name text NOT NULL,
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE courier_services (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    courier_id uuid NOT NULL REFERENCES couriers(id),
    code text NOT NULL,
    name text NOT NULL,
    service_type text NOT NULL CHECK (
        service_type IN ('parcel', 'cargo', 'same_day', 'instant', 'international')
    ),
    transport_mode text,
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (courier_id, code)
);

CREATE TABLE rounding_profiles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code text NOT NULL UNIQUE,
    rounding_mode text NOT NULL CHECK (
        rounding_mode IN ('none', 'ceil', 'floor', 'threshold')
    ),
    increment_grams bigint NOT NULL DEFAULT 1000 CHECK (increment_grams > 0),
    threshold_grams bigint CHECK (
        threshold_grams IS NULL OR
        (threshold_grams >= 0 AND threshold_grams <= increment_grams)
    ),
    verification_status text NOT NULL CHECK (
        verification_status IN (
            'official_public',
            'official_contract',
            'observed',
            'needs_contract_confirmation',
            'deprecated'
        )
    ),
    source_reference text,
    effective_from date NOT NULL,
    effective_until date,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (effective_until IS NULL OR effective_until >= effective_from)
);

CREATE TABLE rate_cards (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    origin_location_id uuid NOT NULL REFERENCES locations(id),
    destination_location_id uuid NOT NULL REFERENCES locations(id),
    courier_service_id uuid NOT NULL REFERENCES courier_services(id),
    pricing_model text NOT NULL CHECK (
        pricing_model IN (
            'flat',
            'per_kg',
            'base_plus_increment',
            'minimum_then_per_kg',
            'tiered',
            'distance_based',
            'provider_quote'
        )
    ),
    currency char(3) NOT NULL DEFAULT 'IDR',
    base_weight_grams bigint NOT NULL DEFAULT 0 CHECK (base_weight_grams >= 0),
    base_price bigint NOT NULL DEFAULT 0 CHECK (base_price >= 0),
    rate_per_increment bigint NOT NULL DEFAULT 0 CHECK (rate_per_increment >= 0),
    minimum_weight_grams bigint NOT NULL DEFAULT 0 CHECK (minimum_weight_grams >= 0),
    maximum_weight_grams bigint CHECK (
        maximum_weight_grams IS NULL OR maximum_weight_grams > 0
    ),
    weight_increment_grams bigint NOT NULL DEFAULT 1000 CHECK (weight_increment_grams > 0),
    volumetric_divisor bigint CHECK (volumetric_divisor IS NULL OR volumetric_divisor > 0),
    rounding_profile_id uuid NOT NULL REFERENCES rounding_profiles(id),
    etd_min_days integer CHECK (etd_min_days IS NULL OR etd_min_days >= 0),
    etd_max_days integer CHECK (etd_max_days IS NULL OR etd_max_days >= 0),
    verification_status text NOT NULL CHECK (
        verification_status IN (
            'official_public',
            'official_contract',
            'observed',
            'needs_contract_confirmation',
            'deprecated'
        )
    ),
    source_provider text NOT NULL,
    source_reference text,
    effective_from timestamptz NOT NULL,
    effective_until timestamptz,
    fetched_at timestamptz NOT NULL,
    expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (
        maximum_weight_grams IS NULL OR
        maximum_weight_grams >= minimum_weight_grams
    ),
    CHECK (etd_max_days IS NULL OR etd_min_days IS NULL OR etd_max_days >= etd_min_days),
    CHECK (effective_until IS NULL OR effective_until >= effective_from)
);

CREATE INDEX rate_cards_active_lookup_idx ON rate_cards (
    origin_location_id,
    destination_location_id,
    courier_service_id,
    effective_from DESC
);

CREATE TABLE rate_tiers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    rate_card_id uuid NOT NULL REFERENCES rate_cards(id) ON DELETE CASCADE,
    weight_from_grams bigint NOT NULL CHECK (weight_from_grams >= 0),
    weight_to_grams bigint CHECK (weight_to_grams IS NULL OR weight_to_grams > weight_from_grams),
    flat_price bigint CHECK (flat_price IS NULL OR flat_price >= 0),
    price_per_increment bigint CHECK (price_per_increment IS NULL OR price_per_increment >= 0),
    increment_grams bigint CHECK (increment_grams IS NULL OR increment_grams > 0),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX rate_tiers_lookup_idx ON rate_tiers (rate_card_id, weight_from_grams);

CREATE TABLE rate_surcharges (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    rate_card_id uuid NOT NULL REFERENCES rate_cards(id) ON DELETE CASCADE,
    surcharge_type text NOT NULL,
    condition_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    amount_type text NOT NULL CHECK (amount_type IN ('flat', 'percentage')),
    amount bigint NOT NULL CHECK (amount >= 0),
    taxable boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE rate_snapshots (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    origin_location_id uuid NOT NULL REFERENCES locations(id),
    destination_location_id uuid NOT NULL REFERENCES locations(id),
    courier_code text NOT NULL,
    service_code text NOT NULL,
    requested_weight_grams bigint NOT NULL CHECK (requested_weight_grams > 0),
    requested_dimensions_json jsonb,
    returned_cost bigint NOT NULL CHECK (returned_cost >= 0),
    etd_min_days integer,
    etd_max_days integer,
    raw_response_hash text,
    provider_code text NOT NULL,
    fetched_at timestamptz NOT NULL,
    expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX rate_snapshots_lookup_idx ON rate_snapshots (
    origin_location_id,
    destination_location_id,
    courier_code,
    service_code,
    fetched_at DESC
);

CREATE TABLE provider_quota_ledger (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_code text NOT NULL,
    credential_alias text NOT NULL,
    quota_date date NOT NULL,
    daily_limit bigint NOT NULL CHECK (daily_limit >= 0),
    used_count bigint NOT NULL DEFAULT 0 CHECK (used_count >= 0),
    reserved_count bigint NOT NULL DEFAULT 0 CHECK (reserved_count >= 0),
    reset_at timestamptz NOT NULL,
    health_status text NOT NULL DEFAULT 'healthy',
    last_error_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider_code, credential_alias, quota_date)
);
