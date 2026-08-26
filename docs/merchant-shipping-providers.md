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
- Biteship hanya fallback internal cek ongkir/resi Emisell Kurir dan tidak dapat
  dipilih atau diaktifkan langsung oleh seller.

| Status | Arti |
|---|---|
| `installed=true` | Provider siap dipilih; provider eksternal mempunyai key aktif dan valid. |
| `active=true` | Provider menjadi jalur shipping efektif merchant. Maksimal satu yang aktif. |
| `available=true` | Provider siap dan dikirim pada katalog yang dibaca dashboard seller. |

Setiap provider juga membawa klasifikasi berikut:

| Jenis | Perilaku |
|---|---|
| `built_in` | Emisell Kurir; dikelola penuh oleh platform. |
| `managed_upstream` | API Kurir memiliki adapter; seller memasang credential sendiri bila diwajibkan. |
| `partner_hosted` | Connector package lulus sertifikasi dan wajib memiliki active release; dapat memakai credential BYOK seller. |

Distribusi tidak dapat dipilih. Provider eksternal selalu memakai nilai
`merchant` dan hanya dapat ditemukan melalui gateway merchant Emisell yang
terautentikasi. Provider bawaan memakai nilai `built_in`; tidak ada katalog
provider publik.

Provider berstatus `available=false` hanya terlihat pada dashboard admin API
Kurir. Provider tersebut tidak dikirim oleh `GET /api/v1/integrations/providers`,
sehingga otomatis hilang dari daftar extension di dashboard Emisell.
Response katalog memakai `Cache-Control: no-store`, sehingga Main Service harus
mengambil ulang daftar ketika halaman pengaturan shipping dibuka atau dimuat ulang.

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
Key yang sama dapat dipasang kembali oleh merchant yang sama. API Kurir
mengaktifkan kembali record credential lama agar audit dan kuota tetap
konsisten; provider shipping tidak ikut aktif sampai endpoint aktivasi dipanggil.

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
        "logo": "https://api-kurir.emisell.com/provider-logos/emisell.svg",
        "description": "Layanan pengiriman bawaan Emisell dengan tarif dan pelacakan terpusat tanpa API key provider dari seller.",
        "installed": true,
        "active": false
      },
      {
        "code": "rajaongkir",
        "name": "RajaOngkir",
        "logo": "https://api-kurir.emisell.com/provider-logos/rajaongkir.svg",
        "description": "Integrasi RajaOngkir menggunakan API key milik seller untuk cek ongkir dan pelacakan sesuai paket akun seller.",
        "installed": true,
        "active": false
      }
    ]
  }
}
```

`logo` selalu berupa URL HTTPS permanen yang dapat langsung dipakai sebagai
`src` gambar oleh dashboard Emisell. `description` adalah teks biasa tanpa HTML.
Metadata ini berasal dari master provider API Kurir, bukan disimpan ulang per
merchant. Seluruh item pada response ini selalu mempunyai `available=true`.
Response juga membawa `integration_type`, `distribution_type`,
`active_release_version`, `required_scopes`, dan `granted_scopes`. Main Service
dapat mengabaikannya untuk kompatibilitas, tetapi sebaiknya mencatat versi
release dan scope pada log aktivasi.

## Pengelolaan master provider

Operator mengelola provider dari menu **Integrasi Provider → Provider** atau
melalui endpoint admin berikut:

| Method | Endpoint | Fungsi |
|---|---|---|
| `GET` | `/v1/admin/shipping-providers` | Membaca metadata dan penggunaan provider. |
| `POST` | `/v1/admin/shipping-providers` | Mendaftarkan provider eksternal baru. |
| `PUT` | `/v1/admin/shipping-providers/{provider_code}` | Mengubah presentasi, urutan, dan status kesiapan. |

Provider baru selalu dibuat `built_in=false` dan `available=false`. Jenis
default-nya `partner_hosted` dengan distribusi `public`; jenis dapat dipilih
menjadi `managed_upstream`. Partner-hosted dapat memakai credential seller
apabila connector meneruskan autentikasi BYOK. RajaOngkir memakai model ini:
release connector wajib published dan merchant wajib memasang Shipping Cost
API key; Shipping Delivery API key ditambahkan untuk fulfillment. Operator baru
mengaktifkan `available` setelah connector atau adapter selesai diuji.
Kode provider tidak dapat diedit setelah provider dibuat. Provider yang masih dipakai merchant
aktif tidak dapat dinonaktifkan langsung. Setelah provider tanpa merchant aktif
dibuat `available=false`, provider langsung hilang dari katalog Emisell tetapi
tetap tersedia pada menu admin agar dapat diaktifkan kembali.

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

## Katalog layanan per provider

`GET /api/v1/integrations/shipping-services` tetap menjadi satu-satunya endpoint
yang dipakai dashboard Emisell untuk merender pilihan kurir. Endpoint tidak
berubah, tetapi isi `couriers` sekarang mengikuti `active_provider_code`:

- `emisell` membaca katalog internal Emisell Kurir;
- `rajaongkir` membaca katalog layanan RajaOngkir;
- partner lain hanya membaca layanan yang sudah disinkronkan dan disertifikasi
  untuk provider tersebut.

Response menambahkan `data.provider_code` dan
`data.preference.provider_code`. Keduanya membantu Main Service memastikan
bahwa checkbox yang sedang dirender memang milik provider aktif. Field tambahan
ini bersifat kompatibel; client lama yang mengabaikan field asing tetap dapat
memakai response sebelumnya.

Preference seller disimpan dengan kunci `(tenant_id, provider_code)`. Jika
seller berpindah dari Emisell Kurir ke provider partner, pilihan lama tidak
dihapus dan tidak diterapkan ke provider baru. Ketika seller kembali ke provider
sebelumnya, konfigurasi provider itu dapat dibaca kembali. `PUT` hanya menerima
service yang terdapat pada katalog provider aktif dan akan mengembalikan
`SHIPPING_DISABLED` apabila tidak ada provider aktif.

Katalog partner tidak boleh diisi manual dari dashboard merchant. Data layanan
masuk dari hasil sinkronisasi connector partner yang sudah lulus sertifikasi;
provider baru akan memiliki katalog kosong sampai proses tersebut selesai.

Perubahan ini tidak memindahkan routing runtime. Emisell Kurir tetap first-party
dengan RajaOngkir utama dan Biteship fallback, sedangkan provider partner baru
boleh menerima trafik checkout setelah connector runtime-nya disertifikasi dan
provider ditandai available oleh admin.

## Error kontrak

| Kode | Kondisi |
|---|---|
| `SHIPPING_PROVIDER_NOT_FOUND` | Provider code tidak dikenal. |
| `SHIPPING_PROVIDER_UNAVAILABLE` | Adapter provider belum siap. |
| `SHIPPING_PROVIDER_RELEASE_UNAVAILABLE` | Partner-hosted belum mempunyai release published. |
| `PROVIDER_CREDENTIAL_UNAVAILABLE` | Key aktif dan valid belum tersedia. |
| `SHIPPING_PROVIDER_VERSION_CONFLICT` | Version katalog sudah berubah. |
| `SHIPPING_DISABLED` | Merchant belum mengaktifkan provider pengiriman. |

Pilihan kurir dan layanan tetap dikelola terpisah melalui
`/api/v1/integrations/shipping-services`.
