INSERT INTO shipping_integration_providers (
    code,
    name,
    logo_url,
    description,
    built_in,
    integration_type,
    distribution_type,
    requires_credential,
    credential_type,
    available,
    display_order
) VALUES (
    'rajaongkir_hosted',
    'RajaOngkir Hosted',
    'https://api-kurir.emisell.com/provider-logos/rajaongkir.svg',
    'Kandidat connector RajaOngkir yang di-host API Kurir untuk rate, tracking, shipment, label, cancel, dan pickup.',
    false,
    'partner_hosted',
    'private',
    false,
    'none',
    false,
    21
)
ON CONFLICT (code) DO NOTHING;

COMMENT ON TABLE shipping_integration_providers IS
    'Provider shipping built-in, managed upstream, dan partner hosted. rajaongkir_hosted tetap private dan unavailable sampai release UAT dipublikasikan.';
