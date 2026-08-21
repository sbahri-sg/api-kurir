ALTER TABLE tenant_shipping_preferences
    DROP CONSTRAINT tenant_shipping_preferences_empty_groups_check,
    DROP CONSTRAINT tenant_shipping_preferences_custom_mode_check;

ALTER TABLE tenant_shipping_preferences
    ADD CONSTRAINT tenant_shipping_preferences_selection_mode_check CHECK (
        selection_mode IN ('all', 'groups', 'custom')
    ),
    ADD CONSTRAINT tenant_shipping_preferences_check CHECK (
        (selection_mode = 'groups' AND cardinality(enabled_groups) > 0)
        OR
        (selection_mode IN ('all', 'custom') AND cardinality(enabled_groups) = 0)
    );
