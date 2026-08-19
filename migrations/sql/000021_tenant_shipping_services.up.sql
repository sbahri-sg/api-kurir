CREATE TABLE tenant_shipping_preferences (
    tenant_id text PRIMARY KEY CHECK (
        tenant_id ~ '^[A-Za-z0-9._:-]{1,128}$'
    ),
    selection_mode text NOT NULL CHECK (
        selection_mode IN ('all', 'groups', 'custom')
    ),
    enabled_groups text[] NOT NULL DEFAULT '{}',
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    updated_by text NOT NULL DEFAULT '' CHECK (length(updated_by) <= 160),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (
        enabled_groups <@ ARRAY[
            'economy',
            'regular',
            'next_day',
            'express',
            'same_day',
            'instant',
            'cargo',
            'international',
            'special'
        ]::text[]
    ),
    CHECK (
        (selection_mode = 'groups' AND cardinality(enabled_groups) > 0)
        OR
        (selection_mode IN ('all', 'custom') AND cardinality(enabled_groups) = 0)
    )
);

CREATE TABLE tenant_shipping_service_selections (
    tenant_id text NOT NULL REFERENCES tenant_shipping_preferences(tenant_id)
        ON DELETE CASCADE,
    courier_service_id uuid NOT NULL REFERENCES courier_services(id)
        ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, courier_service_id)
);

CREATE INDEX tenant_shipping_service_selections_service_idx
    ON tenant_shipping_service_selections (courier_service_id, tenant_id);
