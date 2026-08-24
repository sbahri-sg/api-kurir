# Emisell Merchant Gateway

Dokumen ini adalah kontrak komunikasi backend-to-backend antara Main Service
Emisell dan API Kurir. Main Service tidak perlu membuat JWT tenant, menyimpan
credential ID, atau meneruskan domain storefront.

## Kontrak autentikasi

Setiap request gateway mengirim dua header:

```http
key: <dedicated-emisell-service-key>
X-Emisell-Merchant-ID: merchant_123
```

- `key` mengautentikasi Main Service Emisell.
- `X-Emisell-Merchant-ID` menentukan pemilik data dan isolasi kuota.
- Merchant ID harus stabil, 1–128 karakter, dan hanya boleh berisi huruf,
  angka, titik, garis bawah, titik dua, atau tanda hubung.
- Merchant ID berasal dari database Main Service, bukan body/query browser.
- Dedicated service key hanya disimpan pada secret backend dan tidak boleh
  dikirim ke frontend.

API Kurir tidak lagi memerlukan `TENANT_CONTEXT_PUBLIC_KEY`, JWT, scope tenant,
`domain_id`, atau `integration_id`. API key tetap menjadi pengaman utama;
merchant header adalah konteks kepemilikan, bukan secret kedua.

## Multi-domain

Satu merchant dapat memiliki banyak domain. Main Service menyelesaikan domain,
gudang, dan origin sebelum memanggil API Kurir. Semua domain memakai merchant
ID yang sama sehingga provider, credential, preferensi service, kuota, dan
tracking tidak diduplikasi per domain.

```text
domain-a.com ─┐
domain-b.com ─┼─> merchant_123 ─> API Kurir
domain-c.com ─┘
```

## Autentikasi Main Service

Admin membuat key dari dashboard **API Key → Generate API key** dan memilih
jenis **Main Service**. Key ini disimpan sebagai hash dan memperoleh scope
`gateway:access`. Main Service mengirim dua header berikut:

```http
key: <main-service-key>
X-Emisell-Merchant-ID: merchant_123
```

Key jenis **Public API** tetap dapat memakai ongkir dan tracking publik, tetapi
tidak dapat mengakses `/api/v1/integrations/*` atau membawa konteks merchant.
`API_KEYS` dari environment tetap tersedia hanya sebagai recovery/bootstrap.

## Lifecycle credential dan provider

1. Main Service mengirim key seller ke
   `POST /api/v1/integrations/provider-credentials`.
2. API Kurir memvalidasi key langsung ke provider dan menyimpannya terenkripsi.
3. Response hanya berisi metadata termasking; UUID credential internal tidak
   dikembalikan.
4. Satu merchant hanya mempunyai satu credential aktif per provider. Key baru
   menggantikan key lama secara atomik.
   Jika key yang sama pernah dinonaktifkan, record lama diaktifkan kembali.
   Tindakan ini hanya memasang credential dan tidak otomatis mengaktifkan
   provider shipping merchant.
5. Main Service membaca `GET /api/v1/integrations/providers`; `logo` dan
   `description` pada setiap item dapat langsung dipakai untuk listing extension.
   Endpoint hanya mengirim provider berstatus tersedia; provider yang dimatikan
   operator otomatis hilang dari daftar dashboard seller.
6. Aktivasi dilakukan dengan provider code dan `expected_version`; API Kurir
   memilih credential aktif secara otomatis.

Merchant dapat memasang beberapa provider, tetapi maksimal satu provider
shipping efektif aktif. Tenant baru berstatus nonaktif sampai seller memilih
Emisell Kurir atau provider eksternal. Biteship tidak menjadi extension seller
dan hanya bekerja di balik Emisell Kurir. RajaOngkir tetap provider utama,
sedangkan Biteship menangani fallback cek ongkir dan tracking. Integrasi
RajaOngkir BYOK tidak pernah memakai credential atau saldo Biteship platform.

## Endpoint utama

| Endpoint | Fungsi |
|---|---|
| `GET /api/v1/integrations/provider-credentials` | Metadata key merchant tanpa secret/UUID internal |
| `POST /api/v1/integrations/provider-credentials` | Validasi dan simpan/ganti key provider |
| `POST /api/v1/integrations/provider-credentials/{provider_code}/disable` | Putuskan key berdasarkan provider code |
| `GET /api/v1/integrations/providers` | Katalog provider dan provider efektif aktif |
| `POST /api/v1/integrations/providers/{provider_code}/activate` | Aktifkan provider; credential dipilih internal |
| `POST /api/v1/integrations/providers/{provider_code}/deactivate` | Nonaktifkan shipping merchant bila provider tersebut sedang aktif |
| `GET/PUT /api/v1/integrations/shipping-services` | Baca limit/selectable dan simpan pilihan custom grup regular, next_day, economy, atau cargo |
| `POST /api/v1/integrations/tracking/subscriptions` | Daftarkan AWB fulfillment |
| `GET /api/v1/integrations/tracking/subscriptions/{fulfillment_id}` | Baca snapshot tanpa hit provider |
| `DELETE /api/v1/integrations/tracking/subscriptions/{fulfillment_id}` | Hentikan tracking fulfillment merchant; snapshot tetap disimpan |

Semua integrasi baru menggunakan base path `/api/v1`. Alias `/v1` tetap ada
sementara untuk kompatibilitas.

## Rate dan tracking

Rate kompatibel RajaOngkir V2 tetap menggunakan form-urlencoded:

```http
POST /api/v1/calculate/district/domestic-cost
key: <dedicated-emisell-service-key>
X-Emisell-Merchant-ID: merchant_123
Content-Type: application/x-www-form-urlencoded

origin=442&destination=1354&weight=1200
```

Main Service tidak mengirim `courier`. API Kurir membentuk daftar kurir dari
service yang telah disimpan merchant melalui `PUT
/api/v1/integrations/shipping-services`. Bila field `courier` masih dikirim oleh
client lama, nilainya diabaikan agar tidak dapat melewati konfigurasi seller.
API Kurir lalu membaca provider aktif merchant dan memilih credential internal.
Snapshot dan ledger dipisahkan berdasarkan merchant serta credential internal,
tetapi ID credential tidak menjadi bagian kontrak Main Service.
Jika provider aktif adalah `emisell`, credential merchant memang kosong dan API
Kurir meneruskan request memakai pool credential platform yang telah
diotorisasi. Nilai kosong tersebut bukan kondisi error dan Main Service tidak
perlu mengirim `credential_id`. Jika provider aktif adalah `rajaongkir`, API
Kurir memakai credential BYOK milik merchant yang dipilih secara internal.
Jika belum ada provider aktif, endpoint rate mengembalikan HTTP `409` dengan
kode `SHIPPING_DISABLED`; Main Service dapat menyembunyikan opsi pengiriman dan
menampilkan ajakan mengaktifkan extension kurir.

Tracking sinkron memakai header yang sama pada
`POST /api/v1/track/waybill`. Untuk fulfillment, gunakan subscription agar
pembacaan berulang mengambil snapshot PostgreSQL dan worker saja yang melakukan
refresh provider. Webhook HMAC mengirim perubahan status ke Emisell.

## Error penting

| Kode | Arti |
|---|---|
| `MERCHANT_ID_REQUIRED` | Header merchant tidak dikirim ke endpoint gateway |
| `INVALID_MERCHANT_ID` | Format merchant ID tidak valid |
| `MERCHANT_CONTEXT_FORBIDDEN` | Public API key mencoba membawa merchant header |
| `UNAUTHORIZED` | Key bukan Main Service, tidak aktif, atau tidak valid |
| `INVALID_PROVIDER_KEY` | Key ditolak provider |
| `PROVIDER_KEY_EXISTS` | Key sudah terikat pada scope atau merchant lain |
| `PROVIDER_CREDENTIAL_UNAVAILABLE` | Provider tidak mempunyai key aktif dan valid |
| `SHIPPING_PROVIDER_VERSION_CONFLICT` | State berubah sejak katalog terakhir dibaca |
| `PROVIDER_QUOTA_EXHAUSTED` | Kuota provider habis |
| `RATE_NOT_AVAILABLE` | Merchant belum memilih service atau tarif rute tidak tersedia |

Kontrak mesin tersedia di [`openapi/public.yaml`](../openapi/public.yaml), dan
contoh siap pakai tersedia pada dashboard **Dokumentasi API → Emisell Gateway**.
