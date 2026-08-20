ALTER TABLE tracking_shipments
    ADD COLUMN validation_status text NOT NULL DEFAULT 'unverified' CHECK (
        validation_status IN ('unverified', 'valid', 'not_found', 'invalid')
    ),
    ADD COLUMN validation_checked_at timestamptz,
    ADD COLUMN not_found_count integer NOT NULL DEFAULT 0 CHECK (not_found_count >= 0),
    ADD COLUMN provider_hit_count integer NOT NULL DEFAULT 0 CHECK (provider_hit_count >= 0),
    ADD COLUMN provider_hit_limit integer NOT NULL DEFAULT 10 CHECK (provider_hit_limit > 0),
    ADD COLUMN status_changed_at timestamptz;

ALTER TABLE tracking_shipments
    DROP CONSTRAINT IF EXISTS tracking_shipments_courier_code_waybill_hash_key;

UPDATE tracking_shipments
SET validation_status = CASE
        WHEN provider_fetched_at IS NOT NULL THEN 'valid'
        ELSE 'unverified'
    END,
    validation_checked_at = provider_fetched_at,
    provider_hit_count = CASE
        WHEN provider_fetched_at IS NOT NULL THEN 1
        ELSE 0
    END,
    status_changed_at = provider_fetched_at
WHERE true;

CREATE TABLE tracking_status_history (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    shipment_id uuid NOT NULL REFERENCES tracking_shipments(id) ON DELETE CASCADE,
    normalized_status text NOT NULL,
    status_label text,
    provider_code text,
    provider_fetched_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX tracking_status_history_shipment_idx
    ON tracking_status_history (shipment_id, provider_fetched_at DESC);

CREATE TABLE tracking_subscriptions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id text NOT NULL,
    domain_id text NOT NULL DEFAULT '',
    order_reference text NOT NULL,
    fulfillment_reference text NOT NULL,
    shipment_id uuid NOT NULL REFERENCES tracking_shipments(id) ON DELETE CASCADE,
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, fulfillment_reference)
);

CREATE INDEX tracking_subscriptions_shipment_idx
    ON tracking_subscriptions (shipment_id)
    WHERE active;

CREATE TABLE tracking_webhook_outbox (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    subscription_id uuid NOT NULL REFERENCES tracking_subscriptions(id) ON DELETE CASCADE,
    shipment_id uuid NOT NULL REFERENCES tracking_shipments(id) ON DELETE CASCADE,
    tenant_id text NOT NULL,
    event_type text NOT NULL CHECK (
        event_type IN (
            'tracking.validated',
            'tracking.status_changed',
            'tracking.delivered',
            'tracking.invalid'
        )
    ),
    data_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    status text NOT NULL DEFAULT 'pending' CHECK (
        status IN ('pending', 'running', 'delivered', 'dead')
    ),
    attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    max_attempts integer NOT NULL DEFAULT 10 CHECK (max_attempts > 0),
    available_at timestamptz NOT NULL DEFAULT now(),
    locked_at timestamptz,
    locked_by text,
    last_http_status integer,
    last_error_message text,
    delivered_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX tracking_webhook_outbox_claim_idx
    ON tracking_webhook_outbox (available_at, created_at)
    WHERE status = 'pending';
