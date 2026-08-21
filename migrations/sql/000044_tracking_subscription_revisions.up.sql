ALTER TABLE tracking_subscriptions
    ADD COLUMN revision integer NOT NULL DEFAULT 1 CHECK (revision > 0);

CREATE TABLE tracking_subscription_revisions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    subscription_id uuid NOT NULL REFERENCES tracking_subscriptions(id) ON DELETE CASCADE,
    tenant_id text NOT NULL,
    domain_id text NOT NULL DEFAULT '',
    order_reference text NOT NULL,
    fulfillment_reference text NOT NULL,
    shipment_id uuid NOT NULL REFERENCES tracking_shipments(id) ON DELETE CASCADE,
    revision integer NOT NULL CHECK (revision > 0),
    replacement_reason text NOT NULL DEFAULT 'awb_replaced',
    superseded_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (subscription_id, revision)
);

CREATE INDEX tracking_subscription_revisions_fulfillment_idx
    ON tracking_subscription_revisions (tenant_id, fulfillment_reference, revision DESC);

