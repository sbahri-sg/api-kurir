INSERT INTO tenant_shipping_service_selections (
    tenant_id,
    courier_service_id
)
SELECT
    preference.tenant_id,
    service.id
FROM tenant_shipping_preferences preference
JOIN courier_services service
  ON service.active
JOIN couriers courier
  ON courier.id = service.courier_id
 AND courier.active
WHERE service.service_group <> 'unknown'
  AND (
      preference.selection_mode = 'all'
      OR (
          preference.selection_mode = 'groups'
          AND service.service_group = ANY(preference.enabled_groups)
      )
  )
ON CONFLICT (tenant_id, courier_service_id) DO NOTHING;

UPDATE tenant_shipping_preferences
SET selection_mode = 'custom',
    enabled_groups = '{}',
    version = version + 1,
    updated_by = 'system:custom-only-migration',
    updated_at = now()
WHERE selection_mode <> 'custom';

ALTER TABLE tenant_shipping_preferences
    DROP CONSTRAINT tenant_shipping_preferences_check,
    DROP CONSTRAINT tenant_shipping_preferences_selection_mode_check;

ALTER TABLE tenant_shipping_preferences
    ADD CONSTRAINT tenant_shipping_preferences_custom_mode_check CHECK (
        selection_mode = 'custom'
    ),
    ADD CONSTRAINT tenant_shipping_preferences_empty_groups_check CHECK (
        cardinality(enabled_groups) = 0
    );
