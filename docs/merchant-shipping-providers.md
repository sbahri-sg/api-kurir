# Provider Shipping Merchant

Dokumen ini mendefinisikan lifecycle extension shipping antara Main Service
Emisell dan API Kurir.

## Aturan state

- Emisell Kurir adalah provider bawaan dan selalu terpasang.
- Merchant dapat memasang beberapa provider eksternal.
- Hanya satu provider shipping yang efektif aktif per merchant.
- Satu merchant mempunyai maksimal satu credential aktif per provider.
- Credential ID sepenuhnya internal API Kurir.
- Biteship hanya fallback tracking internal dan tidak dapat diaktifkan seller.

| Status | Arti |
|---|---|
| `installed=true` | Provider siap dipilih; provider eksternal mempunyai key aktif dan valid. |
| `active=true` | Provider menjadi jalur shipping efektif merchant. Hanya satu yang aktif. |
| `available=false` | Provider ada di katalog, tetapi adapter production belum siap. |

## Autentikasi

```http
key: <dedicated-emisell-service-key>
X-Emisell-Merchant-ID: merchant_123
```

Merchant ID ditentukan backend Emisell. Domain storefront tidak dikirim karena
request berlangsung antar-service.

## Memasang atau mengganti key

```http
POST /api/v1/integrations/provider-credentials
Content-Type: application/json

{
  "provider_code": "rajaongkir",
  "api_key": "<seller-provider-key>",
  "daily_limit": 50000
}
```

Key divalidasi dan dienkripsi. Jika merchant sudah mempunyai key RajaOngkir
aktif, key baru menggantikannya atomik. Response tidak mengandung credential ID.
Jika RajaOngkir sedang aktif, pilihan provider dipindahkan ke credential baru
tanpa intervensi Main Service.

Untuk memutuskan key:

```http
POST /api/v1/integrations/provider-credentials/rajaongkir/disable
```

Jika key sedang digunakan, database otomatis mengembalikan provider efektif
merchant ke Emisell Kurir.

## Membaca katalog

```http
GET /api/v1/integrations/providers
```

```json
{
  "data": {
    "active_provider_code": "emisell",
    "version": 0,
    "providers": [
      {
        "code": "emisell",
        "name": "Emisell Kurir",
        "installed": true,
        "active": true
      },
      {
        "code": "rajaongkir",
        "name": "RajaOngkir",
        "installed": true,
        "active": false
      }
    ]
  }
}
```

## Mengaktifkan provider

```http
POST /api/v1/integrations/providers/rajaongkir/activate
Content-Type: application/json

{
  "expected_version": 0
}
```

API Kurir memilih key aktif milik merchant dan provider tersebut. Main Service
tidak menyimpan atau mengirim credential ID. `expected_version` mencegah dua
tab dashboard saling menimpa.

Untuk kembali ke provider gratis:

```http
POST /api/v1/integrations/providers/rajaongkir/deactivate
Content-Type: application/json

{
  "expected_version": 1
}
```

## Alur dashboard Emisell

1. Baca `GET /integrations/providers`.
2. Jika seller memasang RajaOngkir, kirim key ke endpoint credential.
3. Baca ulang katalog sampai `installed=true`.
4. Aktifkan RajaOngkir dengan provider code dan version katalog.
5. Render kurir/service dari `GET /integrations/shipping-services`.
6. Semua rate dan tracking berikutnya cukup membawa API key dan merchant ID.

## Error kontrak

| Kode | Kondisi |
|---|---|
| `SHIPPING_PROVIDER_NOT_FOUND` | Provider code tidak dikenal. |
| `SHIPPING_PROVIDER_UNAVAILABLE` | Adapter provider belum siap. |
| `PROVIDER_CREDENTIAL_UNAVAILABLE` | Key aktif dan valid belum tersedia. |
| `SHIPPING_PROVIDER_VERSION_CONFLICT` | Version katalog sudah berubah. |
| `DEFAULT_PROVIDER_REQUIRED` | Emisell Kurir dicoba dinonaktifkan tanpa pengganti. |

Pilihan kurir dan layanan tetap dikelola terpisah melalui
`/api/v1/integrations/shipping-services`.
