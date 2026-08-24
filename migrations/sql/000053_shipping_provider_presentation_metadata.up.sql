ALTER TABLE shipping_integration_providers
    ADD COLUMN logo_url text,
    ADD COLUMN description text;

UPDATE shipping_integration_providers
SET logo_url = CASE code
        WHEN 'emisell' THEN 'https://api-kurir.emisell.com/provider-logos/emisell.svg'
        WHEN 'rajaongkir' THEN 'https://api-kurir.emisell.com/provider-logos/rajaongkir.svg'
        WHEN 'kiriminaja' THEN 'https://api-kurir.emisell.com/provider-logos/kiriminaja.svg'
        ELSE 'https://api-kurir.emisell.com/provider-logos/default.svg'
    END,
    description = CASE code
        WHEN 'emisell' THEN 'Layanan pengiriman bawaan Emisell dengan tarif dan pelacakan terpusat tanpa API key provider dari seller.'
        WHEN 'rajaongkir' THEN 'Integrasi RajaOngkir menggunakan API key milik seller untuk cek ongkir dan pelacakan sesuai paket akun seller.'
        WHEN 'kiriminaja' THEN 'Integrasi KiriminAja menggunakan credential milik seller dan tersedia setelah adapter provider diaktifkan.'
        ELSE name || ' adalah provider pengiriman yang terintegrasi dengan API Kurir.'
    END;

ALTER TABLE shipping_integration_providers
    ALTER COLUMN logo_url SET NOT NULL,
    ALTER COLUMN description SET NOT NULL,
    ADD CONSTRAINT shipping_integration_providers_logo_url_check
        CHECK (length(logo_url) BETWEEN 12 AND 2048 AND logo_url ~ '^https://'),
    ADD CONSTRAINT shipping_integration_providers_description_check
        CHECK (length(description) BETWEEN 10 AND 500);
