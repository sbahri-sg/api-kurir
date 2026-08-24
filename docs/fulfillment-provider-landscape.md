# Strategi Provider Fulfillment

Status: **contract-first; booking dan pickup belum aktif di production**

Versi: **1.0**

Terakhir ditinjau: **24 Agustus 2026**

## 1. Tujuan

Dokumen ini menetapkan cara API Kurir menangani fitur setelah cek ongkir:

- membuat pesanan pengiriman dan AWB;
- meminta pickup atau memilih drop-off;
- membatalkan shipment;
- mengambil label;
- menerima tracking dan webhook;
- menangani COD, retur, saldo, dan rekonsiliasi biaya.

Emisell hanya berkomunikasi dengan kontrak canonical API Kurir. Perbedaan
payload, status, credential, dan aturan finansial provider tidak boleh bocor ke
checkout atau order service Emisell.

## 2. Keputusan arsitektur

### 2.1 Provider bawaan API Kurir

Provider bawaan diimplementasikan dan dioperasikan langsung oleh API Kurir:

| Provider | Peran saat ini | Target berikutnya |
|---|---|---|
| RajaOngkir Shipping Cost | Sumber utama tarif dan tracking | Tetap menjadi primary untuk fungsi yang sudah tersedia |
| Biteship | Fallback tarif dan tracking Emisell Kurir | Kandidat booking, pickup, cancel, label, dan webhook |
| RajaOngkir Shipping Delivery | Belum diimplementasikan | Kandidat booking, pickup, cancel, label, dan webhook |

Credential provider bawaan disimpan terenkripsi. API Kurir bertanggung jawab
atas idempotency, pembatasan kuota, saldo platform, rekonsiliasi, dan insiden
provider. Fitur target baru boleh dinyatakan aktif setelah lulus sandbox dan
contract test.

### 2.2 Certified Partner Connector

KiriminAja, Lincah, Mengantar, dan vendor baru tidak dibuatkan adapter native
di API Kurir. Mereka menyediakan server **Partner Connector v1** yang
menerjemahkan API native mereka ke kontrak canonical API Kurir.

Konsekuensinya:

- API Kurir tidak menyimpan token API native milik partner;
- perubahan API native menjadi tanggung jawab partner;
- partner harus lulus keamanan, idempotency, webhook, dan contract test;
- extension baru dipublikasikan setelah sertifikasi selesai.

Kontrak teknisnya berada di [Partner Integration Contract v1](partner-api-v1.md)
dan [Partner Event Webhook v1](partner-webhooks-v1.md).

## 3. Arti status capability

Gunakan tiga status berikut agar dokumentasi tidak dianggap sebagai bukti
fitur production:

| Status | Arti |
|---|---|
| `implemented` | Sudah tersedia di kode API Kurir dan telah diuji |
| `documented` | Ada pada dokumentasi resmi provider, tetapi belum tentu diimplementasikan API Kurir |
| `product_evidence` | Ada pada halaman produk/help resmi, tetapi kontrak endpoint lengkap belum diperoleh |

Provider tidak boleh diberi status `live` hanya berdasarkan halaman produk.

## 4. Matriks provider

| Provider | Model integrasi | Rate | Order/AWB | Pickup | Cancel | Tracking/webhook | COD | Saldo via API | Kesiapan |
|---|---|---:|---:|---:|---:|---:|---:|---:|---|
| RajaOngkir Shipping Delivery | Built-in | Documented | Documented | Documented | Documented | Documented | Documented | Belum ditemukan endpoint angka saldo publik | Tinggi untuk sandbox |
| Biteship Fulfillment | Built-in | Implemented | Documented | Documented | Documented | Tracking fallback implemented; order webhook belum | Documented | Belum ditemukan endpoint angka saldo publik | Tinggi untuk sandbox |
| KiriminAja | Certified Partner | Documented | Documented | Documented | Documented | Documented | Documented | Documented | Sangat tinggi setelah connector dan sertifikasi |
| Lincah | Certified Partner | Product evidence | Product evidence | Product evidence | Product evidence | Product evidence | Product evidence | Product evidence | Menengah; perlu OpenAPI lengkap |
| Mengantar | Certified Partner | Product evidence | Product evidence | Product evidence | Belum terverifikasi | Product evidence | Product evidence | Product evidence | Menengah; perlu dokumentasi teknis lengkap |

`Documented` tidak berarti endpoint tersebut sudah tersedia melalui API Kurir.

## 5. Catatan per provider

### 5.1 RajaOngkir Shipping Delivery

Shipping Delivery adalah produk dan credential terpisah dari Shipping Cost.
Alur targetnya:

```text
calculate Delivery -> store order -> pickup -> AWB/label -> webhook/history
```

Quote Shipping Cost tidak boleh langsung digunakan untuk membuat order
Shipping Delivery. API Kurir harus menyimpan ID quote/order produk Delivery
yang benar dan memisahkan biaya `quoted`, `booked`, serta `actual`.

Pickup mendukung kumpulan order, tanggal, waktu, dan kendaraan. Callback yang
tidak mempunyai signature atau delivery ID yang cukup kuat wajib dilengkapi
deduplication dan rekonsiliasi pull.

### 5.2 Biteship

Biteship sudah dipakai sebagai fallback tarif dan tracking. Dukungan create
order, pickup/drop-off, cancel, label, dan webhook masih merupakan target
implementasi.

Pada model Biteship, metode pengambilan dapat menjadi bagian dari pembuatan
order. API Kurir tetap harus mengekspos shipment dan pickup sebagai resource
canonical terpisah agar kontrak Emisell tidak berubah ketika provider berbeda.

Saldo operasional Biteship tidak boleh diasumsikan dapat dibaca melalui API
sebelum endpoint resmi tersedia. Sampai saat itu, gunakan threshold dan alert
manual di dashboard provider, bukan angka saldo palsu di API Kurir.

### 5.3 KiriminAja

Dokumentasi KiriminAja mencakup sandbox/production, rate, express/instant
order, pickup terjadwal, cancel, tracking, callback, COD/non-COD, pembayaran,
KA Credit, PIN, service discovery, volumetric, rounding, dan cut-off.

Walaupun capability-nya lengkap, onboarding tetap melalui Partner Connector.
KiriminAja atau integrator resminya yang memelihara translasi native. PIN
bersifat write-only dan tidak boleh disimpan API Kurir.

Hanya kelompok layanan yang disetujui untuk Emisell yang boleh diekspos:
`regular`, `next_day`, `economy`, dan `cargo`. Instant/same-day tetap dimatikan
sampai scope produk diperluas.

### 5.4 Lincah

Sumber resmi Lincah menunjukkan environment production dan sandbox, API Token
serta Partner ID, multi-courier rate, COD, wilayah, multi-origin, pembuatan
pengiriman, tracking/webhook, pickup, dan cancel.

Sebelum sertifikasi, API Kurir harus memperoleh:

- OpenAPI lengkap dan contoh error;
- skema signature webhook dan replay protection;
- aturan idempotency create/cancel/pickup;
- rate limit, SLA, dan kebijakan retry;
- endpoint saldo, mutasi, serta rekonsiliasi biaya;
- status shipment dan retur yang lengkap.

### 5.5 Mengantar

Sumber publik Mengantar menunjukkan API key, order, request pickup, tracking,
COD/non-COD, saldo, label, dan retur sebagai capability produk. Detail kontrak
publik lengkap belum cukup untuk implementasi production.

Sebelum sertifikasi, minta sandbox, OpenAPI, signature webhook, idempotency,
rate limit, SLA, status mapping, cancellation rules, serta API saldo/mutasi.
Mengantar kemudian menerapkan Partner Connector v1 pada infrastrukturnya.

## 6. Capability canonical

Provider dan account harus menerbitkan capability yang dapat dibaca mesin.
Contoh:

```json
{
  "rates": true,
  "create_shipment": true,
  "cancel_shipment": true,
  "pickup": true,
  "scheduled_pickup": true,
  "batch_pickup": true,
  "dropoff": true,
  "tracking": true,
  "external_tracking": true,
  "webhook": true,
  "label": true,
  "cod": true,
  "insurance": true,
  "balance": false,
  "instant": false
}
```

Nilai capability berasal dari hasil sertifikasi dan konfigurasi admin, bukan
ditebak dari nama provider. Capability `instant=false` mengikuti scope Emisell
saat ini.

## 7. Kontrak northbound target untuk Emisell

Endpoint berikut adalah **target**, bukan pernyataan bahwa semuanya sudah
aktif:

```text
POST /api/v1/integrations/shipments
GET  /api/v1/integrations/shipments/{shipment_id}
POST /api/v1/integrations/shipments/{shipment_id}/cancel
GET  /api/v1/integrations/shipments/{shipment_id}/label
POST /api/v1/integrations/pickups
GET  /api/v1/integrations/pickups/{pickup_id}
GET  /api/v1/integrations/providers/{provider_code}/balance
```

Request antar-backend memakai dedicated service API key dan `merchant_id`.
Domain storefront bukan identitas otorisasi. Semua resource wajib terikat ke
merchant hasil autentikasi dan tidak boleh dapat dibaca lintas merchant.

## 8. Penguncian quote dan provider

Setiap quote yang dapat dibooking wajib membawa minimal:

```text
provider_code
provider_account_id
quote_token
expires_at
courier_code
service_code
total_price
```

`quote_token` mengikat merchant, origin, destination, paket, harga, provider,
dan masa berlaku. Shipment tidak boleh dibuat memakai quote provider A pada
provider B.

## 9. Alur canonical

```text
Emisell checkout
  -> API Kurir quote
  <- opsi + provider + quote_token
Emisell membuat order
  -> API Kurir create shipment (idempotency key)
  -> built-in provider atau certified Partner Connector
  <- shipment_id + AWB + pickup/drop-off state
Provider/partner
  -> webhook API Kurir
API Kurir
  -> normalisasi, dedup, snapshot, webhook Emisell
```

Emisell tidak perlu mengetahui endpoint native atau credential provider.

## 10. Aturan fallback transaksi

Fallback tarif dan tracking boleh otomatis karena keduanya read-only. Fallback
untuk create shipment, pickup, cancel, pembayaran, dan COD tidak boleh blind:

1. retry ke provider yang sama memakai idempotency key;
2. jika timeout, lakukan inquiry/reconciliation sebelum retry;
3. jangan pindah provider jika shipment mungkin sudah terbentuk;
4. ganti provider hanya melalui keputusan eksplisit sebelum booking berhasil;
5. simpan `quoted_cost`, `booked_cost`, dan `actual_cost` terpisah.

Aturan ini mencegah AWB ganda, pickup ganda, dan saldo terpotong dua kali.

## 11. Data minimum

Modul fulfillment sekurangnya membutuhkan:

- `fulfillment_shipments`;
- `fulfillment_shipment_items`;
- `pickup_requests`;
- `shipment_labels`;
- `provider_events`;
- `provider_reconciliation_jobs`.

Simpan canonical ID, provider/account ID, provider order ID, AWB, revision,
idempotency key, biaya, status canonical/raw, timestamps, dan error aman.
Alamat, telepon, label, serta payload provider termasuk data sensitif dan tidak
boleh dicetak utuh ke application log.

## 12. Urutan implementasi

1. Finalisasi domain shipment, pickup, label, finance, dan idempotency.
2. Tambahkan migration, audit, outbox, dan reconciliation worker.
3. Implementasikan RajaOngkir Shipping Delivery pada sandbox.
4. Implementasikan Biteship fulfillment pada mode test/sandbox.
5. Sertifikasi KiriminAja melalui Partner Connector.
6. Sertifikasi Lincah setelah OpenAPI dan kontrol keamanan lengkap diterima.
7. Sertifikasi Mengantar setelah dokumentasi teknis lengkap diterima.
8. Aktifkan per provider memakai feature flag, canary merchant, dan kill switch.

Jangan melakukan pickup live hanya untuk pengujian. Gunakan sandbox atau order
yang secara eksplisit disetujui untuk uji operasional.

## 13. Sumber

- RajaOngkir: [base URL dan endpoint Delivery](https://www.rajaongkir.com/docs/delivery-order-api/getting_started/base-url), [store order](https://rajaongkir.com/docs/delivery-order-api/Store_order/store_order), [pickup](https://rajaongkir.com/docs/delivery-order-api/pickup_order), dan [webhook](https://www.rajaongkir.com/docs/delivery-order-api/webhook).
- Biteship: [create order](https://biteship.com/en/docs/api/orders/create) dan [order lifecycle](https://biteship.com/id/docs/api/orders/overview).
- KiriminAja: [API documentation](https://developer.kiriminaja.com/docs/api).
- Lincah: [plugin resmi Lincah Shipping](https://id.wordpress.org/plugins/lincah-shipping/), [platform pengiriman](https://lincah.id/platform-kirim-paket-online/), dan [pembayaran dengan saldo](https://lincah.id/help-center-bayar-ongkir-dengan-saldo/).
- Mengantar: [generate API key](https://help.mengantar.com/id/articles/14715369-mengantar-update-verifikasi-akun-pengiriman-full-darat-generate-api-key-dan-update-lainnya), [produk](https://mengantar.id/page/), dan [help center](https://help.mengantar.com/id).

Sumber publik harus diverifikasi ulang saat kontrak komersial dibuat. OpenAPI,
SLA, DPA, rate limit, serta kebijakan saldo yang berlaku mengalahkan ringkasan
di dokumen ini.
