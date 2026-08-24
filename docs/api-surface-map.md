# Peta Kontrak API Kurir

Status: **contract-first**
Versi: **1.0**
Terakhir ditinjau: **24 Agustus 2026**

## 1. Tujuan

API Kurir adalah gerbang tunggal antara Emisell, seller, aggregator, dan
ekspedisi. Setiap pihak memakai kontrak yang berbeda agar credential, data
tenant, dan tanggung jawab operasional tidak tercampur.

## 2. Permukaan API

| Permukaan | Pemanggil | Penerima | Base path/kontrak |
|---|---|---|---|
| RajaOngkir V2 compatible | Emisell atau SDK RajaOngkir | API Kurir | `/api/v1`, `openapi/public.yaml` |
| Emisell Legacy | Modul region-service lama | API Kurir | `/regions`, `/shipping`, `openapi/public.yaml` |
| Canonical/Internal | Dashboard atau service internal | API Kurir | `/v1`, `openapi/public.yaml` |
| Merchant Shipping Services | Backend Extension Kurir Emisell | API Kurir | `/api/v1/integrations/shipping-services`, `openapi/public.yaml` |
| Merchant Shipping Provider | Backend Extension Kurir Emisell | API Kurir | `/api/v1/integrations/providers`, `openapi/public.yaml` |
| Admin | Operator API Kurir | API Kurir | `/v1/admin`, `openapi/public.yaml` |
| Provider Account API | Seller/admin Emisell | API Kurir | `openapi/provider-account-v1.yaml` |
| Partner Connector API | API Kurir | Sistem partner | `openapi/partner-v1.yaml` |
| Partner Event Webhook | Sistem partner | API Kurir | bagian `webhooks` pada `openapi/partner-v1.yaml` |

### 2.1 Public API

Public API mempertahankan kompatibilitas RajaOngkir V2 untuk destination,
cek ongkir, dan tracking. Emisell tidak menerima credential provider dan
tidak memanggil provider secara langsung.

Kontrak customer dipilih melalui path dan tidak berubah karena jenis header:
`/api/v1` memakai ID/respons RajaOngkir V2, `/v1` memakai public ID canonical
internal, sedangkan `/regions` dan `/shipping` mempertahankan kontrak
region-service lama. Semua menerima customer API key melalui header `key` atau
Bearer.

### 2.2 Provider Account API

Provider Account API dipakai seller untuk mengaktifkan satu atau beberapa
koneksi partner. Credential yang diterima API Kurir adalah connection token
canonical yang diterbitkan partner, bukan token API native yang dipakai
partner ke carrier atau sistem internalnya. Token koneksi divalidasi
server-side, dienkripsi, dan tidak pernah dikembalikan lagi.

Satu seller dapat mempunyai, misalnya:

```text
partner_a / regular_and_cargo / live
partner_b / instant / live
```

Keduanya adalah koneksi berbeda meskipun berada di bawah seller yang sama.

Pilihan layanan yang tampil di checkout tidak disimpan pada provider account.
Konfigurasi tersebut memakai pasangan canonical `courier_code:service_code`
melalui [Merchant Shipping Services](merchant-shipping-services.md), sehingga
JNE REG tetap satu pilihan logis walaupun tersedia dari lebih dari satu akun
provider.

### 2.3 Partner Connector API

Partner Connector API adalah kontrak canonical yang harus disediakan vendor
baru seperti KiriminAja, Lincah, atau Mengantar. API Kurir memanggil endpoint
partner untuk rate, booking, pickup, label, rekonsiliasi, dan fungsi opsional
lainnya.

Partner boleh mempertahankan API native yang sudah ada, tetapi translation
layer ke kontrak canonical dibuat dan dioperasikan oleh partner sendiri.
API Kurir tidak memelihara adapter per vendor.

RajaOngkir dan Biteship dapat menjadi provider bawaan yang adapter-nya
dioperasikan API Kurir. Batas, capability, dan urutan implementasi keduanya
dibandingkan dengan partner pada
[Strategi Provider Fulfillment](fulfillment-provider-landscape.md).

## 3. Routing multi-provider

Saat seller mengaktifkan beberapa provider, API Kurir meminta quote secara
concurrent sesuai capability dan kebijakan seller. Setiap quote wajib
menyimpan:

```text
seller_id
provider_account_id
provider_code
product_code
environment
courier_code
service_code
provider_quote_id
expires_at
```

`provider_account_id` dan `provider_quote_id` dikunci saat shipment dibuat.
Tarif dari provider A tidak boleh digunakan untuk booking melalui provider B.

### 3.1 Pemilihan quote

Kebijakan yang didukung:

- harga total terendah;
- SLA tercepat;
- prioritas seller;
- provider yang mendukung COD/asuransi;
- provider yang mampu melakukan booking;
- fallback ketika provider gagal.

Fallback hanya boleh terjadi sebelum shipment berhasil dibuat. Shipment yang
sudah mempunyai `partner_shipment_id` tidak boleh dipindahkan otomatis.

### 3.2 Deduplikasi pilihan customer

Beberapa provider dapat mengembalikan kurir dan layanan fisik yang sama.
API Kurir mengelompokkan berdasarkan:

```text
courier_code + canonical_service_group + delivery_mode
```

Pilihan yang disembunyikan dari customer tetap disimpan untuk audit dan
fallback sebelum booking.

## 4. Aturan lokasi

Pencarian customer selalu membaca master wilayah lokal API Kurir. Partner
Connector menerima kode wilayah resmi, kode pos, atau koordinat sesuai
capability yang disertifikasi.

Penerjemahan kode canonical tersebut ke ID native RajaOngkir, KiriminAja,
carrier, atau sistem internal lain dilakukan sepenuhnya di sisi partner.
API Kurir tidak menyimpan mapping ID native untuk partner connector baru.

Mapping provider yang sudah terdapat pada integrasi legacy Shipping Cost hanya
dipertahankan untuk fungsi legacy tersebut dan bukan pola onboarding partner.

## 5. Pemisahan tracking

| Jenis | Jalur |
|---|---|
| Shipment dibuat API Kurir | provider account asal shipment |
| AWB eksternal marketplace | provider dengan capability `external_tracking` |
| Event realtime shipment | webhook provider |
| Rekonsiliasi | detail/history endpoint provider asal |

Tracking eksternal tidak mengubah kepemilikan shipment. Jika provider meminta
validasi tambahan seperti digit terakhir nomor penerima, data tersebut hanya
diteruskan untuk request terkait dan tidak dijadikan syarat global.

## 6. Status implementasi

Dokumen dan OpenAPI pada peta ini adalah kontrak target. Endpoint yang belum
terdaftar dalam implementasi Go atau migration tidak boleh dianggap aktif di
production hanya karena sudah tercantum dalam dokumen.
