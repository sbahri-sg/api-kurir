ALTER TABLE couriers
    ADD COLUMN logo_url text NOT NULL
        DEFAULT 'https://api-kurir.emisell.com/courier-logos/default.svg',
    ADD COLUMN logo_source_url text,
    ADD COLUMN logo_verified_at date;

ALTER TABLE couriers
    ADD CONSTRAINT couriers_logo_url_check
        CHECK (length(logo_url) BETWEEN 12 AND 2048 AND logo_url ~ '^https://'),
    ADD CONSTRAINT couriers_logo_source_url_check
        CHECK (
            logo_source_url IS NULL OR
            (length(logo_source_url) BETWEEN 12 AND 2048 AND logo_source_url ~ '^https://')
        );

UPDATE couriers
SET logo_url = CASE code
        WHEN 'jne' THEN 'https://api-kurir.emisell.com/courier-logos/jne.webp'
        WHEN 'jnt' THEN 'https://api-kurir.emisell.com/courier-logos/jnt.webp'
        WHEN 'sicepat' THEN 'https://api-kurir.emisell.com/courier-logos/sicepat.webp'
        WHEN 'ide' THEN 'https://api-kurir.emisell.com/courier-logos/ide.webp'
        WHEN 'sap' THEN 'https://api-kurir.emisell.com/courier-logos/sap.webp'
        WHEN 'ninja' THEN 'https://api-kurir.emisell.com/courier-logos/ninja.webp'
        WHEN 'tiki' THEN 'https://api-kurir.emisell.com/courier-logos/tiki.webp'
        WHEN 'wahana' THEN 'https://api-kurir.emisell.com/courier-logos/wahana.webp'
        WHEN 'pos' THEN 'https://api-kurir.emisell.com/courier-logos/pos.webp'
        WHEN 'sentral' THEN 'https://api-kurir.emisell.com/courier-logos/sentral.webp'
        WHEN 'lion' THEN 'https://api-kurir.emisell.com/courier-logos/lion.webp'
        WHEN 'rpx' THEN 'https://api-kurir.emisell.com/courier-logos/rpx.webp'
        WHEN 'anteraja' THEN 'https://api-kurir.emisell.com/courier-logos/anteraja.webp'
        WHEN 'rex' THEN 'https://api-kurir.emisell.com/courier-logos/rex.png'
        ELSE 'https://api-kurir.emisell.com/courier-logos/default.svg'
    END,
    logo_source_url = CASE code
        WHEN 'jne' THEN 'https://biteship.com/images/couriers/jne.webp'
        WHEN 'jnt' THEN 'https://biteship.com/images/couriers/jnt.webp'
        WHEN 'sicepat' THEN 'https://biteship.com/images/couriers/sicepat.webp'
        WHEN 'ide' THEN 'https://biteship.com/images/couriers/idexpress.webp'
        WHEN 'sap' THEN 'https://biteship.com/images/couriers/sap.webp'
        WHEN 'ninja' THEN 'https://biteship.com/images/couriers/ninja.webp'
        WHEN 'tiki' THEN 'https://biteship.com/images/couriers/tiki.webp'
        WHEN 'wahana' THEN 'https://biteship.com/images/couriers/wahana.webp'
        WHEN 'pos' THEN 'https://biteship.com/images/couriers/pos.webp'
        WHEN 'sentral' THEN 'https://biteship.com/images/couriers/sentralcargo.webp'
        WHEN 'lion' THEN 'https://biteship.com/images/couriers/lion.webp'
        WHEN 'rpx' THEN 'https://biteship.com/images/couriers/rpx.webp'
        WHEN 'anteraja' THEN 'https://biteship.com/images/couriers/anteraja.webp'
        WHEN 'rex' THEN 'https://rex.co.id/images/templates/logo.png'
        ELSE NULL
    END,
    logo_verified_at = CASE
        WHEN code IN (
            'jne', 'jnt', 'sicepat', 'ide', 'sap', 'ninja', 'tiki',
            'wahana', 'pos', 'sentral', 'lion', 'rpx', 'anteraja', 'rex'
        ) THEN DATE '2026-08-24'
        ELSE NULL
    END,
    updated_at = now();
