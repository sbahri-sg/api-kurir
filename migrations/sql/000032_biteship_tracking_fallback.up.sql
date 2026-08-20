UPDATE couriers
SET supports_tracking = true,
    active = true,
    catalog_source = CASE
        WHEN position(
            'https://biteship.com/id/docs/api/trackings/status'
            IN coalesce(catalog_source, '')
        ) > 0 THEN catalog_source
        ELSE concat_ws(
            '; ',
            nullif(catalog_source, ''),
            'https://biteship.com/id/docs/api/trackings/status'
        )
    END,
    catalog_verified_at = DATE '2026-08-20',
    updated_at = now()
WHERE code IN ('sicepat', 'ide', 'sentral');
