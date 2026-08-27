CREATE TABLE fulfillment_quotes (
    id text PRIMARY KEY,
    tenant_id text NOT NULL,
    provider_code text NOT NULL,
    environment_code text NOT NULL,
    credential_alias text NOT NULL,
    provider_quote_id text NOT NULL DEFAULT '',
    courier_code text NOT NULL,
    courier_name text NOT NULL,
    service_code text NOT NULL,
    native_service_code text NOT NULL,
    service_name text NOT NULL,
    service_group text NOT NULL,
    delivery_mode text NOT NULL,
    shipping_cost bigint NOT NULL,
    shipping_cashback bigint NOT NULL DEFAULT 0,
    service_fee bigint NOT NULL DEFAULT 0,
    additional_cost bigint NOT NULL DEFAULT 0,
    grand_total bigint NOT NULL,
    cod_value bigint NOT NULL DEFAULT 0,
    insurance_value bigint NOT NULL DEFAULT 0,
    currency char(3) NOT NULL DEFAULT 'IDR',
    etd text NOT NULL DEFAULT '',
    binding_hash bytea NOT NULL,
    expires_at timestamptz NOT NULL,
    consumed_by_shipment_id uuid,
    consumed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT fulfillment_quotes_environment_check
        CHECK (environment_code IN ('live', 'sandbox')),
    CONSTRAINT fulfillment_quotes_group_check
        CHECK (service_group IN ('regular', 'next_day', 'economy', 'cargo')),
    CONSTRAINT fulfillment_quotes_delivery_mode_check
        CHECK (delivery_mode IN ('regular', 'next_day', 'economy', 'cargo')),
    CONSTRAINT fulfillment_quotes_amount_check
        CHECK (
            shipping_cost >= 0 AND shipping_cashback >= 0 AND
            service_fee >= 0 AND additional_cost >= 0 AND grand_total >= 0 AND
            cod_value >= 0 AND insurance_value >= 0
        ),
    CONSTRAINT fulfillment_quotes_tenant_id_unique UNIQUE (tenant_id, id),
    CONSTRAINT fulfillment_quotes_consumed_shipment_fk
        FOREIGN KEY (tenant_id, consumed_by_shipment_id)
        REFERENCES fulfillment_shipments(tenant_id, id)
);

CREATE INDEX fulfillment_quotes_tenant_expiry_idx
    ON fulfillment_quotes (tenant_id, expires_at DESC);
CREATE INDEX fulfillment_quotes_provider_idx
    ON fulfillment_quotes (tenant_id, provider_code, environment_code, created_at DESC);
