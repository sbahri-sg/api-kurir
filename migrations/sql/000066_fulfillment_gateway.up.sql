CREATE TABLE fulfillment_shipments (
    id uuid PRIMARY KEY,
    tenant_id text NOT NULL,
    provider_code text NOT NULL,
    merchant_reference text NOT NULL,
    quote_id text NOT NULL,
    courier_code text NOT NULL,
    service_code text NOT NULL,
    delivery_mode text NOT NULL,
    fulfillment_mode text NOT NULL,
    provider_shipment_id text,
    awb text,
    normalized_status text NOT NULL DEFAULT 'booking_pending',
    provider_status text NOT NULL DEFAULT '',
    shipping_cost bigint NOT NULL DEFAULT 0,
    currency char(3) NOT NULL DEFAULT 'IDR',
    create_idempotency_key text NOT NULL,
    create_request_hash bytea NOT NULL,
    request_ciphertext bytea NOT NULL,
    label_available boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    cancelled_at timestamptz,
    CONSTRAINT fulfillment_shipments_tenant_idempotency_unique
        UNIQUE (tenant_id, create_idempotency_key),
    CONSTRAINT fulfillment_shipments_tenant_reference_unique
        UNIQUE (tenant_id, merchant_reference),
    CONSTRAINT fulfillment_shipments_tenant_id_unique
        UNIQUE (tenant_id, id),
    CONSTRAINT fulfillment_shipments_status_check CHECK (
        normalized_status IN (
            'booking_pending', 'booked', 'pickup_requested', 'picked_up',
            'in_transit', 'out_for_delivery', 'delivered',
            'cancellation_pending', 'cancelled', 'problem', 'booking_failed'
        )
    )
);

CREATE INDEX fulfillment_shipments_tenant_updated_idx
    ON fulfillment_shipments (tenant_id, updated_at DESC);
CREATE UNIQUE INDEX fulfillment_shipments_provider_reference_unique
    ON fulfillment_shipments (provider_code, provider_shipment_id)
    WHERE provider_shipment_id IS NOT NULL;

CREATE TABLE fulfillment_operations (
    id uuid PRIMARY KEY,
    tenant_id text NOT NULL,
    shipment_id uuid NOT NULL,
    operation_type text NOT NULL,
    idempotency_key text NOT NULL,
    request_hash bytea NOT NULL,
    status text NOT NULL DEFAULT 'pending',
    provider_operation_id text NOT NULL DEFAULT '',
    provider_status text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT fulfillment_operations_type_check
        CHECK (operation_type IN ('pickup', 'cancel')),
    CONSTRAINT fulfillment_operations_status_check
        CHECK (status IN ('pending', 'completed', 'failed')),
    CONSTRAINT fulfillment_operations_idempotency_unique
        UNIQUE (tenant_id, operation_type, idempotency_key),
    CONSTRAINT fulfillment_operations_tenant_shipment_fk
        FOREIGN KEY (tenant_id, shipment_id)
        REFERENCES fulfillment_shipments(tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX fulfillment_operations_shipment_idx
    ON fulfillment_operations (tenant_id, shipment_id, created_at DESC);

CREATE TABLE fulfillment_history (
    id bigserial PRIMARY KEY,
    tenant_id text NOT NULL,
    shipment_id uuid NOT NULL,
    normalized_status text NOT NULL,
    provider_status text NOT NULL DEFAULT '',
    description text NOT NULL,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT fulfillment_history_tenant_shipment_fk
        FOREIGN KEY (tenant_id, shipment_id)
        REFERENCES fulfillment_shipments(tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX fulfillment_history_shipment_idx
    ON fulfillment_history (tenant_id, shipment_id, occurred_at, id);

CREATE TABLE fulfillment_shipment_labels (
    shipment_id uuid NOT NULL,
    tenant_id text NOT NULL,
    format text NOT NULL,
    content_type text NOT NULL,
    label_ciphertext bytea NOT NULL,
    expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (shipment_id, format),
    CONSTRAINT fulfillment_labels_tenant_shipment_fk
        FOREIGN KEY (tenant_id, shipment_id)
        REFERENCES fulfillment_shipments(tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX fulfillment_shipment_labels_tenant_idx
    ON fulfillment_shipment_labels (tenant_id, shipment_id);
