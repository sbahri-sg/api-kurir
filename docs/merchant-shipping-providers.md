# Provider Shipping Merchant

Dokumen ini mendefinisikan lifecycle extension shipping antara Main Service
Emisell dan API Kurir.

## Aturan state

- Emisell Kurir adalah provider bawaan dan selalu terpasang.
- Merchant dapat memasang beberapa provider eksternal.
- Merchant baru tidak mempunyai provider aktif. Seller harus mengaktifkan
  Emisell Kurir atau provider eksternal bila membutuhkan pengiriman.
- Maksimal satu provider shipping efektif aktif per merchant; state tanpa
  provider aktif diperbolehkan.
- Satu merchant mempunyai maksimal satu credential aktif per provider.
- Credential ID sepenuhnya internal API Kurir.
- Biteship hanya fallback tracking internal dan tidak dapat diaktifkan seller.

| Status | Arti |
|---|---|
| `installed=true` | Provider siap dipilih; provider eksternal mempunyai key aktif dan valid. |
| `active=true` | Provider menjadi jalur shipping efektif merchant. Maksimal satu yang aktif. |
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

Jika key sedang digunakan, database otomatis menonaktifkan shipping merchant.
Seller dapat memilih Emisell Kurir atau memasang key provider lain setelahnya.

## Membaca katalog

```http
GET /api/v1/integrations/providers
```

```json
{
  "data": {
    "active_provider_code": null,
    "version": 0,
    "providers": [
      {
        "code": "emisell",
        "name": "Emisell Kurir",
        "installed": true,
        "active": false
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

Untuk menonaktifkan pengiriman provider tersebut:

```http
POST /api/v1/integrations/providers/rajaongkir/deactivate
Content-Type: application/json

{
  "expected_version": 1
}
```

Endpoint yang sama berlaku untuk Emisell Kurir:

```http
POST /api/v1/integrations/providers/emisell/deactivate
```

Setelah berhasil, `active_provider_code` bernilai `null` dan semua item katalog
memiliki `active=false`. Permintaan ongkir bertenant akan mengembalikan HTTP
`409` dengan kode `SHIPPING_DISABLED` sampai seller mengaktifkan provider.

## Alur dashboard Emisell

1. Baca `GET /integrations/providers`.
2. Jangan mengaktifkan provider otomatis saat merchant dibuat.
3. Bila seller mengaktifkan Emisell Kurir, panggil endpoint activate untuk
   `emisell`.
4. Jika seller memasang RajaOngkir, kirim key ke endpoint credential.
5. Baca ulang katalog sampai `installed=true`, lalu aktifkan RajaOngkir dengan
   provider code dan version katalog.
6. Saat seller mematikan extension kurir, panggil endpoint deactivate untuk
   provider yang sedang aktif.
7. Render kurir/service dari `GET /integrations/shipping-services` hanya ketika
   `active_provider_code` tidak `null`; patuhi `limits` dan `selectable` dari
   response agar checkbox tidak melewati batas merchant.

## Error kontrak

| Kode | Kondisi |
|---|---|
| `SHIPPING_PROVIDER_NOT_FOUND` | Provider code tidak dikenal. |
| `SHIPPING_PROVIDER_UNAVAILABLE` | Adapter provider belum siap. |
| `PROVIDER_CREDENTIAL_UNAVAILABLE` | Key aktif dan valid belum tersedia. |
| `SHIPPING_PROVIDER_VERSION_CONFLICT` | Version katalog sudah berubah. |
| `SHIPPING_DISABLED` | Merchant belum mengaktifkan provider pengiriman. |

Pilihan kurir dan layanan tetap dikelola terpisah melalui
`/api/v1/integrations/shipping-services`.
