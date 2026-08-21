# Provider Shipping Merchant

Dokumen ini mendefinisikan lifecycle extension provider antara Main Service
Emisell dan API Kurir. Implementasi berada sepenuhnya di API Kurir; Main Service
cukup memakai kontrak tenant-aware yang tersedia.

## Tujuan

- seluruh provider shipping dapat tampil dalam satu katalog;
- Emisell Kurir menjadi provider bawaan dan selalu terpasang;
- merchant dapat mempunyai lebih dari satu provider terpasang;
- tepat satu provider efektif aktif untuk satu merchant;
- memilih kurir dan service tetap dikelola terpisah melalui
  `/api/v1/integrations/shipping-services`;
- credential seller tidak pernah keluar dari API Kurir setelah disimpan.

## Pemisahan status

`installed` dan `active` mempunyai arti berbeda:

| Status | Arti |
|---|---|
| `installed=true` | Provider siap dipilih. Untuk provider eksternal berarti terdapat credential aktif dan valid milik merchant. |
| `active=true` | Provider tersebut menjadi jalur shipping efektif merchant. Hanya satu provider yang bernilai true. |
| `available=false` | Provider tercatat dalam katalog, tetapi adapter production belum siap dan aktivasi ditolak. |

Menambah credential melalui
`POST /api/v1/integrations/provider-credentials` membuat provider eksternal
menjadi terpasang. Menonaktifkan credential bukan operasi yang sama dengan
menonaktifkan extension. Namun, bila credential tersebut sedang dipakai oleh
provider aktif, API Kurir otomatis mengembalikan merchant ke Emisell Kurir.

## Katalog awal

| Kode | Provider | Bawaan | Credential | Tersedia |
|---|---|---:|---:|---:|
| `emisell` | Emisell Kurir | Ya | Tidak | Ya |
| `rajaongkir` | RajaOngkir | Tidak | Ya | Ya |
| `kiriminaja` | KiriminAja | Tidak | Ya | Belum |

Biteship tidak ditampilkan sebagai extension seller karena perannya tetap
sebagai fallback tracking internal API Kurir.

## Autentikasi

Semua endpoint memakai dua bukti:

```http
key: <customer-api-key>
X-Emisell-Tenant-Token: <signed-tenant-jwt>
```

Claim `sub` menjadi merchant ID. Merchant ID tidak pernah diterima melalui body
atau parameter query.

Scope:

- `shipping:read` untuk membaca katalog;
- `shipping:write` untuk activate/deactivate;
- `provider-credentials:read/write` untuk lifecycle credential.

## Membaca katalog

```http
GET /api/v1/integrations/providers
```

Tenant baru tanpa record eksplisit mendapatkan:

```json
{
  "data": {
    "active_provider_code": "emisell",
    "version": 0,
    "providers": [
      {
        "code": "emisell",
        "name": "Emisell Kurir",
        "built_in": true,
        "requires_credential": false,
        "available": true,
        "installed": true,
        "active": true
      }
    ]
  }
}
```

## Mengaktifkan provider

Provider eksternal:

```http
POST /api/v1/integrations/providers/rajaongkir/activate
Content-Type: application/json

{
  "credential_id": "11111111-2222-4333-8444-555555555555",
  "expected_version": 0
}
```

Emisell Kurir:

```http
POST /api/v1/integrations/providers/emisell/activate
Content-Type: application/json

{
  "expected_version": 1
}
```

Aktivasi dilakukan dalam transaksi PostgreSQL dan memakai advisory lock per
merchant. `expected_version` bersifat opsional tetapi sangat disarankan agar dua
tab dashboard tidak saling menimpa. Credential eksternal harus:

- aktif;
- berstatus valid;
- dimiliki tenant pada claim `sub`;
- mempunyai `provider_code` yang sama dengan provider yang diaktifkan.

## Menonaktifkan provider

```http
POST /api/v1/integrations/providers/rajaongkir/deactivate
Content-Type: application/json

{
  "expected_version": 1
}
```

Jika RajaOngkir sedang aktif, API Kurir mengaktifkan Emisell Kurir dan menaikkan
version. Jika RajaOngkir sudah tidak aktif, request aman diulang dan tidak
mengubah state. Emisell Kurir tidak dapat dinonaktifkan tanpa provider pengganti.

## Alur dashboard yang disarankan

1. Baca `GET /providers`.
2. Seller memasang provider eksternal dengan menambahkan credential.
3. Simpan UUID dari response credential.
4. Baca ulang `GET /providers` sampai `installed=true`.
5. Panggil `/activate` dengan `credential_id` dan `expected_version`.
6. Setelah sukses, gunakan `active_credential_id` sebagai `integration_id` pada
   tenant token request tarif dan tracking.
7. Untuk mode Emisell Kurir, kosongkan `integration_id`.

Main Service tidak perlu menyimpan secret provider. UUID credential bukan secret,
tetapi tetap harus divalidasi kepemilikannya oleh API Kurir pada setiap aktivasi.

## Multi-domain

Pilihan provider terikat pada `merchant_id`, bukan domain. Semua storefront
dalam satu merchant memakai provider aktif yang sama. `domain_id` tetap boleh
dikirim dalam tenant token untuk audit dan pemilihan origin warehouse.

## Error kontrak

| Kode | Kondisi |
|---|---|
| `SHIPPING_PROVIDER_NOT_FOUND` | Kode provider tidak ada dalam katalog. |
| `SHIPPING_PROVIDER_UNAVAILABLE` | Adapter provider belum dapat digunakan. |
| `PROVIDER_CREDENTIAL_REQUIRED` | Provider eksternal diaktifkan tanpa UUID credential. |
| `PROVIDER_CREDENTIAL_UNAVAILABLE` | Credential invalid, inactive, beda provider, atau bukan milik merchant. |
| `SHIPPING_PROVIDER_VERSION_CONFLICT` | State sudah berubah setelah dashboard terakhir membaca katalog. |
| `DEFAULT_PROVIDER_REQUIRED` | Emisell Kurir dicoba dinonaktifkan tanpa pengganti. |

## Penegakan pada rate dan tracking

Main Service tetap menyertakan `integration_id` dari response provider aktif
dalam tenant token. API Kurir kemudian memastikan UUID tersebut sama dengan
credential pada provider aktif merchant. Credential yang valid dan masih
terpasang tetapi berstatus provider tidak aktif ditolak dengan
`RATE_NOT_AVAILABLE` atau error tracking terkait. Dengan demikian UUID lama
tidak dapat digunakan untuk melewati pilihan extension merchant.
