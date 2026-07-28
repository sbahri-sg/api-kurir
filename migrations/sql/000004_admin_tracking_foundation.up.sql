CREATE UNIQUE INDEX rate_cards_version_unique_idx ON rate_cards (
    origin_location_id,
    destination_location_id,
    courier_service_id,
    effective_from
);

CREATE TABLE admin_audit_logs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_alias text NOT NULL,
    action text NOT NULL,
    resource_type text NOT NULL,
    resource_id text,
    request_id text,
    details_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX admin_audit_logs_created_idx
    ON admin_audit_logs (created_at DESC);

CREATE TABLE tracking_shipments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    courier_code text NOT NULL,
    waybill_hash text NOT NULL,
    waybill_masked text NOT NULL,
    waybill_ciphertext bytea NOT NULL,
    normalized_status text NOT NULL DEFAULT 'unknown' CHECK (
        normalized_status IN (
            'pending_pickup',
            'picked_up',
            'in_transit',
            'out_for_delivery',
            'delivered',
            'delivery_failed',
            'returned',
            'cancelled',
            'unknown'
        )
    ),
    status_label text,
    summary_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    events_json jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (
        jsonb_typeof(events_json) = 'array'
    ),
    provider_code text,
    provider_fetched_at timestamptz,
    next_refresh_at timestamptz,
    is_final boolean NOT NULL DEFAULT false,
    last_error_code text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (courier_code, waybill_hash)
);

CREATE INDEX tracking_shipments_refresh_idx
    ON tracking_shipments (next_refresh_at)
    WHERE NOT is_final;

CREATE TABLE tracking_refresh_jobs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    shipment_id uuid NOT NULL REFERENCES tracking_shipments(id) ON DELETE CASCADE,
    status text NOT NULL DEFAULT 'pending' CHECK (
        status IN ('pending', 'running', 'completed', 'dead')
    ),
    priority integer NOT NULL DEFAULT 100,
    attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    max_attempts integer NOT NULL DEFAULT 8 CHECK (max_attempts > 0),
    available_at timestamptz NOT NULL DEFAULT now(),
    locked_at timestamptz,
    locked_by text,
    last_error_code text,
    last_error_message text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX tracking_refresh_jobs_active_unique_idx
    ON tracking_refresh_jobs (shipment_id)
    WHERE status IN ('pending', 'running');

CREATE INDEX tracking_refresh_jobs_claim_idx
    ON tracking_refresh_jobs (priority, available_at, created_at)
    WHERE status = 'pending';
