ALTER TABLE fulfillment_shipments
    ADD COLUMN tracking_registration_status text NOT NULL DEFAULT 'not_ready',
    ADD COLUMN tracking_shipment_id uuid REFERENCES tracking_shipments(id) ON DELETE SET NULL,
    ADD COLUMN live_tracking_url text NOT NULL DEFAULT '',
    ADD COLUMN last_reconciled_at timestamptz,
    ADD COLUMN next_reconcile_at timestamptz,
    ADD COLUMN reconcile_attempt_count integer NOT NULL DEFAULT 0,
    ADD COLUMN reconcile_error text NOT NULL DEFAULT '',
    ADD CONSTRAINT fulfillment_tracking_registration_status_check CHECK (
        tracking_registration_status IN ('not_ready', 'pending', 'registered', 'failed')
    );

CREATE INDEX fulfillment_shipments_reconcile_idx
    ON fulfillment_shipments (next_reconcile_at)
    WHERE next_reconcile_at IS NOT NULL
      AND normalized_status NOT IN ('delivered', 'cancelled');

CREATE TABLE fulfillment_lifecycle_jobs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id text NOT NULL,
    shipment_id uuid NOT NULL,
    job_type text NOT NULL,
    status text NOT NULL DEFAULT 'pending',
    attempt_count integer NOT NULL DEFAULT 0,
    max_attempts integer NOT NULL DEFAULT 12,
    available_at timestamptz NOT NULL DEFAULT now(),
    locked_at timestamptz,
    locked_by text,
    last_error_code text NOT NULL DEFAULT '',
    last_error_message text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT fulfillment_lifecycle_jobs_type_check
        CHECK (job_type IN ('register_tracking', 'reconcile')),
    CONSTRAINT fulfillment_lifecycle_jobs_status_check
        CHECK (status IN ('pending', 'running', 'completed', 'dead')),
    CONSTRAINT fulfillment_lifecycle_jobs_tenant_shipment_fk
        FOREIGN KEY (tenant_id, shipment_id)
        REFERENCES fulfillment_shipments(tenant_id, id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX fulfillment_lifecycle_jobs_active_unique
    ON fulfillment_lifecycle_jobs (shipment_id, job_type)
    WHERE status IN ('pending', 'running');

CREATE INDEX fulfillment_lifecycle_jobs_claim_idx
    ON fulfillment_lifecycle_jobs (status, available_at, created_at);

CREATE TABLE fulfillment_webhook_outbox (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id text NOT NULL,
    shipment_id uuid NOT NULL,
    event_type text NOT NULL,
    deduplication_key text NOT NULL UNIQUE,
    data_json jsonb NOT NULL,
    status text NOT NULL DEFAULT 'pending',
    attempt_count integer NOT NULL DEFAULT 0,
    max_attempts integer NOT NULL DEFAULT 8,
    available_at timestamptz NOT NULL DEFAULT now(),
    locked_at timestamptz,
    locked_by text,
    last_http_status integer,
    last_error_message text NOT NULL DEFAULT '',
    delivered_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT fulfillment_webhook_outbox_status_check
        CHECK (status IN ('pending', 'running', 'delivered', 'dead')),
    CONSTRAINT fulfillment_webhook_outbox_tenant_shipment_fk
        FOREIGN KEY (tenant_id, shipment_id)
        REFERENCES fulfillment_shipments(tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX fulfillment_webhook_outbox_claim_idx
    ON fulfillment_webhook_outbox (status, available_at, created_at);
