ALTER TABLE tracking_shipments
    ADD COLUMN shipped_at timestamptz,
    ADD COLUMN delivered_at timestamptz;

-- Existing snapshots may not be polled again after reaching a final state.
-- Backfill the best observation time already available in local history. New
-- provider refreshes replace this approximation with the actual manifest/POD
-- timestamp when the provider exposes it.
UPDATE tracking_shipments shipment
SET shipped_at = coalesce(
        (
            SELECT min(history.provider_fetched_at)
            FROM tracking_status_history history
            WHERE history.shipment_id = shipment.id
              AND history.normalized_status IN (
                  'picked_up',
                  'in_transit',
                  'out_for_delivery',
                  'delivered',
                  'delivery_failed',
                  'returned'
              )
        ),
        CASE
            WHEN shipment.normalized_status IN (
                'picked_up',
                'in_transit',
                'out_for_delivery',
                'delivered',
                'delivery_failed',
                'returned'
            ) THEN shipment.provider_fetched_at
        END
    ),
    delivered_at = coalesce(
        (
            SELECT min(history.provider_fetched_at)
            FROM tracking_status_history history
            WHERE history.shipment_id = shipment.id
              AND history.normalized_status = 'delivered'
        ),
        CASE
            WHEN shipment.normalized_status = 'delivered'
            THEN coalesce(shipment.status_changed_at, shipment.provider_fetched_at)
        END
    )
WHERE shipment.provider_fetched_at IS NOT NULL;
