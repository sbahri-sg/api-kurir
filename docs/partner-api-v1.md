# Partner Integration Contract v1

Status dokumen: **Draft untuk implementasi dan sertifikasi**
Pemilik kontrak: **API Kurir / Emisell**
Versi kontrak: **2026-07-29**

## 1. Tujuan

Partner Integration Contract v1 adalah standar agar ekspedisi, aggregator,
dan penyedia last-mile dapat terhubung ke Emisell melalui API Kurir tanpa
menambahkan logika khusus provider ke aplikasi utama Emisell.

Kontrak ini mempunyai dua permukaan:

1. **Partner Connector API** — endpoint yang disediakan partner dan dipanggil
   API Kurir untuk capability, tarif, pembuatan shipment, pickup,
   pembatalan, label, dan rekonsiliasi status.
2. **Partner Event Webhook** — endpoint API Kurir yang menerima perubahan
   AWB dan status dari partner.

Façade RajaOngkir V2 tetap menjadi kontrak northbound untuk SDK Emisell.
Partner Contract adalah kontrak southbound dan tidak mengikuti model internal
RajaOngkir, KiriminAja, atau provider tertentu.

```text
Emisell
   |
   | API Kurir public contract
   v
API Kurir
   |
   | Partner Integration Contract v1
   +--------> Partner Connector API
   |
   <-------- Partner Event Webhook
```

## 2. Model integrasi

### 2.1 Standard connector

Partner mengimplementasikan kontrak v1 pada server milik partner. API Kurir
memanggil endpoint tersebut secara langsung.

Ini adalah satu-satunya pola onboarding partner. Jika partner telah mempunyai
API native, partner sendiri yang membuat connector/translation layer menuju
kontrak canonical v1. API Kurir tidak membuat adapter khusus per vendor dan
tidak menerima token API native yang dipakai partner ke sistem internalnya.

### 2.2 Batas kepemilikan

| Sistem | Data dan tanggung jawab |
|---|---|
| Emisell | checkout, seller, order, customer, produk |
| API Kurir | routing koneksi, quote, shipment, AWB, tracking, webhook, connector credential, audit |
| Partner | implementasi connector, mapping native, tarif/coverage, booking, pickup, pengiriman fisik, event operasional |

API Kurir menerbitkan `shipment_id` canonical. Partner menerbitkan
`partner_shipment_id` dan AWB. Keduanya tidak boleh dipertukarkan.

## 3. Konvensi umum

- Protokol hanya HTTPS dengan TLS 1.2 atau lebih baru.
- Production tidak boleh memakai self-signed certificate.
- Payload memakai UTF-8 JSON.
- Uang memakai integer rupiah.
- Berat memakai integer gram.
- Dimensi memakai integer sentimeter.
- Koordinat memakai decimal WGS84.
- Timestamp memakai RFC 3339 UTC.
- ID harus diperlakukan sebagai string case-sensitive.
- Field baru dapat ditambahkan secara backward-compatible.
- Field wajib tidak boleh dihapus atau diubah tipenya dalam major version yang
  sama.
- API Kurir mengirim `X-Request-Id` pada setiap request.
- Partner mengembalikan `X-Request-Id` yang sama pada response.

Base path connector:

```text
https://<partner-host>/partner/v1
```

Webhook API Kurir:

```text
https://<api-kurir-host>/v1/partner-events/<connection_id>
```

## 4. Autentikasi dan request signing

Credential partner terpisah dari:

- customer API key API Kurir;
- admin API key;
- credential native/upstream yang tetap dikelola partner;
- webhook secret.

Production minimum menggunakan Bearer credential acak 256-bit yang dapat
dirotasi tanpa downtime. OAuth 2.0 client credentials dengan access token
berumur pendek dapat digunakan apabila kedua pihak mendukungnya.

```http
Authorization: Bearer <partner-access-token>
X-Partner-Key-Id: pk_live_...
```

Semua request `POST` dan webhook wajib ditandatangani HMAC-SHA256:

```http
X-Signature-Version: v1
X-Signature-Timestamp: 1785292800
X-Signature-Nonce: 01J...ULID
X-Signature: <base64url-hmac-sha256>
```

Canonical string:

```text
v1
<unix_timestamp>
<nonce>
<UPPERCASE_METHOD>
<path_with_sorted_query>
<lowercase_sha256_hex_of_raw_body>
```

Signature:

```text
base64url(HMAC-SHA256(webhook_or_request_secret, canonical_string))
```

Penerima wajib:

1. membaca raw body sebelum JSON decoding;
2. menolak timestamp dengan selisih lebih dari 300 detik;
3. menolak nonce yang telah dipakai selama minimal 10 menit;
4. menghitung signature dari path dan raw body yang benar-benar diterima;
5. membandingkan signature secara constant-time;
6. tidak menulis token, secret, signature, atau raw data pribadi ke log.

mTLS direkomendasikan untuk partner bervolume tinggi atau pemrosesan COD.
IP allowlist hanya menjadi lapisan tambahan dan tidak menggantikan signature.

## 5. Scope credential

| Scope | Akses |
|---|---|
| `partner:capabilities:read` | health, capability, layanan |
| `partner:rates:read` | cek coverage dan tarif |
| `partner:shipments:write` | membuat shipment |
| `partner:shipments:read` | detail dan rekonsiliasi shipment |
| `partner:shipments:cancel` | pembatalan |
| `partner:pickups:write` | permintaan pickup |
| `partner:labels:read` | mengambil label |
| `partner:tracking:read` | tracking shipment dan AWB eksternal |
| `partner:finance:read` | saldo dan payment inquiry opsional |
| `partner:events:write` | mengirim webhook event |

Credential dengan scope tarif tidak boleh digunakan untuk booking atau COD.

## 6. Endpoint Partner Connector

### 6.1 Health

```http
GET /partner/v1/health
```

Health tidak boleh melakukan booking atau memanggil carrier downstream.

```json
{
  "status": "ok",
  "version": "1.3.2",
  "time": "2026-07-29T06:00:00Z"
}
```

### 6.2 Capabilities

```http
GET /partner/v1/capabilities
```

```json
{
  "provider_code": "vendor_x",
  "provider_name": "Vendor X Logistics",
  "contract_version": "1.0",
  "capabilities": {
    "rates": true,
    "shipment_create": true,
    "shipment_cancel": true,
    "scheduled_pickup": true,
    "on_demand_pickup": true,
    "dropoff": true,
    "label": true,
    "batch_label": true,
    "tracking": true,
    "external_tracking": false,
    "webhook": true,
    "cod": false,
    "insurance": true,
    "instant": true,
    "multi_package": true,
    "multi_destination": false,
    "balance": false,
    "payment_inquiry": false
  },
  "location_granularities": [
    "district",
    "subdistrict",
    "postal_code",
    "coordinate"
  ],
  "webhook_event_types": [
    "shipment.awb_assigned",
    "shipment.picked_up",
    "shipment.in_transit",
    "shipment.out_for_delivery",
    "shipment.delivered",
    "shipment.problem",
    "shipment.returned",
    "shipment.cancelled"
  ]
}
```

API Kurir hanya mengaktifkan capability yang lulus sertifikasi. Nilai yang
dikembalikan partner bukan persetujuan otomatis.

### 6.3 Daftar layanan

```http
GET /partner/v1/services
```

```json
{
  "data": [
    {
      "code": "REG",
      "name": "Regular",
      "courier_code": "jne",
      "courier_name": "JNE",
      "service_group": "regular",
      "service_type": "parcel",
      "delivery_type": "express",
      "delivery_mode": "regular",
      "active": true,
      "supports_cod": false,
      "supports_insurance": true,
      "supports_pickup": true,
      "supports_dropoff": true,
      "supports_multi_destination": false,
      "vehicle_types": [],
      "cut_off_time": "16:00:00",
      "minimum_weight_grams": 1000,
      "maximum_weight_grams": 30000,
      "volumetric_divisor": 6000,
      "rounding_threshold_grams": 300
    }
  ]
}
```

Service code partner disimpan sebagai raw alias. API Kurir memetakan service
tersebut ke katalog canonical tanpa input harga manual.

Partner Connector wajib menerima lokasi canonical sesuai
`location_granularities`. Jika sistem native partner memakai ID lokasi lain,
partner melakukan mapping tersebut secara internal. Partner tidak boleh
meminta API Kurir menyimpan atau mengirim token/ID native yang tidak tercantum
dalam kontrak.

### 6.4 Cek tarif

```http
POST /partner/v1/rates
```

```json
{
  "request_id": "req_01J...",
  "delivery_mode": "regular",
  "vehicle_type": null,
  "origin": {
    "official_code": "3273061001",
    "postal_code": "40174",
    "latitude": -6.907,
    "longitude": 107.581
  },
  "destination": {
    "official_code": "3212122001",
    "postal_code": "45281",
    "latitude": -6.462,
    "longitude": 108.292
  },
  "package": {
    "weight_grams": 1200,
    "length_cm": 20,
    "width_cm": 15,
    "height_cm": 10,
    "item_value": 150000,
    "contents": "Pakaian"
  },
  "service_codes": ["REG", "CARGO"],
  "features": {
    "cod": false,
    "insurance": false,
    "pickup": true
  }
}
```

```json
{
  "request_id": "req_01J...",
  "quotes": [
    {
      "quote_id": "qt_vendor_01J...",
      "expires_at": "2026-07-29T06:15:00Z",
      "service": {
        "code": "REG",
        "name": "Regular",
        "service_type": "parcel",
        "delivery_mode": "regular"
      },
      "price_type": "fixed",
      "chargeable_weight_grams": 2000,
      "cost": {
        "shipping": 18000,
        "insurance": 0,
        "cod_fee": 0,
        "service_fee": 0,
        "discount": 0,
        "surcharge": 0,
        "additional": 0,
        "total": 18000,
        "currency": "IDR"
      },
      "etd": {
        "min_days": 2,
        "max_days": 3,
        "text": "2-3 hari"
      }
    }
  ]
}
```

`quote_id` mengikat rute, paket, layanan, fitur, harga, dan waktu berlaku.
Partner harus menolak booking jika quote tidak sesuai atau kedaluwarsa.

### 6.5 Membuat shipment

```http
POST /partner/v1/shipments
Idempotency-Key: 01J...ULID
```

```json
{
  "shipment_id": "shp_01J...",
  "merchant_reference": "ORDER-EMISELL-10001",
  "quote_id": "qt_vendor_01J...",
  "service_code": "REG",
  "delivery_mode": "regular",
  "fulfillment": "pickup",
  "sender": {
    "name": "Toko Emisell",
    "phone": "628123456789",
    "email": null,
    "address": "Jalan Contoh No. 1",
    "address_note": "Pintu warna hijau",
    "official_code": "3273061001",
    "postal_code": "40174",
    "latitude": -6.907,
    "longitude": 107.581
  },
  "recipient": {
    "name": "Budi",
    "phone": "628987654321",
    "email": null,
    "address": "Jalan Tujuan No. 2",
    "address_note": null,
    "official_code": "3212122001",
    "postal_code": "45281",
    "latitude": -6.462,
    "longitude": 108.292
  },
  "package": {
    "weight_grams": 1200,
    "length_cm": 20,
    "width_cm": 15,
    "height_cm": 10,
    "item_value": 150000,
    "contents": "Pakaian",
    "items": [
      {
        "sku": "TSHIRT-BLACK-M",
        "name": "Kaos Hitam",
        "quantity": 1,
        "unit_value": 150000,
        "weight_grams": 1200
      }
    ]
  },
  "payment": {
    "type": "non_cod",
    "funding_source": "provider_balance",
    "cod_value": 0,
    "insurance_value": 0
  },
  "pickup": {
    "mode": "scheduled",
    "scheduled_at": "2026-07-29T09:00:00Z"
  }
}
```

Response `201 Created`:

```json
{
  "shipment_id": "shp_01J...",
  "partner_shipment_id": "VENDOR-10001",
  "awb": null,
  "status": "booking_pending",
  "service_code": "REG",
  "delivery_mode": "regular",
  "cost": {
    "total": 18000,
    "currency": "IDR"
  },
  "created_at": "2026-07-29T06:02:00Z"
}
```

AWB boleh `null` apabila dibuat asynchronous. Partner kemudian mengirim
`shipment.awb_assigned` melalui webhook.

Partner wajib menjadikan `shipment_id` unik. Request dengan shipment yang
sama tidak boleh membuat booking kedua.

### 6.6 Rekonsiliasi shipment

```http
GET /partner/v1/shipments/{partner_shipment_id}
```

Endpoint ini adalah fallback bila webhook terlambat atau gagal. Response
memuat identitas, AWB, status canonical, status mentah partner, biaya aktual,
dan event terakhir.

### 6.7 Pembatalan

```http
POST /partner/v1/shipments/{partner_shipment_id}/cancel
Idempotency-Key: 01J...ULID
```

```json
{
  "reason_code": "customer_request",
  "reason": "Pembeli membatalkan pesanan"
}
```

Response `202 Accepted` berarti pembatalan belum final. Status hanya menjadi
`cancelled` setelah partner atau carrier mengonfirmasi melalui webhook atau
rekonsiliasi.

### 6.8 Pickup

```http
POST /partner/v1/pickups
Idempotency-Key: 01J...ULID
```

Satu pickup dapat berisi beberapa `partner_shipment_id`, selama partner
mendukung `multi_package`.

### 6.9 Label

```http
GET /partner/v1/shipments/{partner_shipment_id}/label?format=pdf
```

Response dapat berupa:

- `application/pdf`;
- JSON dengan URL HTTPS sementara yang kedaluwarsa paling lama 15 menit.

API Kurir tidak menerima URL `file:`, private IP, loopback, atau redirect ke
host yang tidak disetujui.

### 6.10 Mencari window pickup

```http
POST /partner/v1/pickup-windows/search
```

Endpoint opsional ini mengembalikan jadwal provider untuk pickup regular.
Instant memakai `on_demand` dan tidak memerlukan window terjadwal.

### 6.11 Pickup parsial

Response pickup batch wajib mempunyai hasil per shipment:

```json
{
  "pickup_id": "pku_01J...",
  "partner_pickup_id": "PICKUP-10001",
  "status": "partial",
  "items": [
    {
      "partner_shipment_id": "VENDOR-10001",
      "status": "accepted",
      "awb": "AWB123",
      "error": null
    },
    {
      "partner_shipment_id": "VENDOR-10002",
      "status": "failed",
      "awb": null,
      "error": {
        "code": "PICKUP_ADDRESS_UNSUPPORTED",
        "message": "Pickup tidak tersedia."
      }
    }
  ]
}
```

Status batch adalah `accepted`, `partial`, atau `failed`. Retry hanya
menyertakan item gagal yang `retryable`.

### 6.12 Label batch

```http
POST /partner/v1/labels/batch
Idempotency-Key: 01J...ULID
```

Request memuat beberapa `partner_shipment_id`, format PDF, dan layout:

```text
a4_1
a4_2
a4_4
thermal_100x100
thermal_100x150
```

Response adalah URL HTTPS sementara atau file PDF. Partner tidak boleh
membuat label kedua untuk request idempotent yang sama.

### 6.13 Tracking AWB eksternal

```http
POST /partner/v1/tracking/waybills
```

Endpoint hanya diwajibkan jika capability `external_tracking=true`.

```json
{
  "courier_code": "jne",
  "awb": "AWB123456789",
  "last_phone_digits": null
}
```

`last_phone_digits` bersifat conditional dan hanya diminta ketika carrier
memerlukannya. Tracking eksternal tidak membuat shipment dan tidak mengubah
provider account asal suatu order.

### 6.14 Saldo dan payment inquiry

```http
GET /partner/v1/account/balance
GET /partner/v1/payments/{payment_id}
```

Endpoint opsional ini hanya aktif jika capability terkait telah disertifikasi.
Saldo provider dan saldo Emisell tetap dicatat pada ledger terpisah. PIN tidak
pernah disimpan atau dikirim melalui endpoint read.

### 6.15 Instant delivery

Instant wajib memakai:

- `delivery_mode=instant`;
- origin/destination latitude-longitude;
- `vehicle_type` dari quote;
- quote dengan expiry pendek;
- fulfillment `on_demand`;
- status driver dan live tracking yang bersifat opsional.

Urutan canonical:

```text
booking_pending
driver_searching
driver_allocated
arrived_at_pickup
picked_up
in_transit
delivered
```

Harga instant dapat berubah. Detail shipment menyimpan `quoted_cost`,
`booked_cost`, `actual_cost`, adjustment, dan alasan. Perubahan biaya baru
menjadi authoritative setelah rekonsiliasi.

### 6.16 Proof of delivery dan biaya aktual

Detail/event boleh memuat penerima, waktu penerimaan, URL foto, atau tanda
tangan. URL POD dan live tracking tunduk pada kontrol SSRF dan retention.

Biaya tidak ditimpa:

```text
quoted_cost  = harga saat checkout
booked_cost  = harga yang diterima saat booking
actual_cost  = harga final provider
```

## 7. Idempotency

`Idempotency-Key` wajib untuk:

- membuat shipment;
- membatalkan shipment;
- membuat pickup;
- membuat label batch;
- mutasi pembayaran/COD di masa depan.

Aturan:

1. panjang 16–128 karakter;
2. scope adalah partner + operation + credential;
3. disimpan minimal 24 jam;
4. key sama dan hash request sama mengembalikan status/body pertama;
5. key sama dengan payload berbeda mengembalikan `409 IDEMPOTENCY_CONFLICT`;
6. retry setelah timeout wajib menggunakan key yang sama;
7. API Kurir tidak mengulang request mutasi tanpa rekonsiliasi apabila status
   upstream tidak diketahui.

`shipment_id` unik tetap menjadi perlindungan kedua, bukan pengganti
`Idempotency-Key`.

## 8. Partner Event Webhook

Partner mengirim event ke:

```http
POST https://api-kurir.example.com/v1/partner-events/{connection_id}
Authorization: Bearer <event-token>
X-Event-Id: evt_01J...
X-Signature-Version: v1
X-Signature-Timestamp: 1785292800
X-Signature-Nonce: 01J...
X-Signature: <signature>
```

```json
{
  "event_id": "evt_01J...",
  "event_type": "shipment.in_transit",
  "event_version": 5,
  "occurred_at": "2026-07-29T07:20:00Z",
  "shipment": {
    "shipment_id": "shp_01J...",
    "partner_shipment_id": "VENDOR-10001",
    "awb": "AWB123456789",
    "status": "in_transit",
    "partner_status": "HUB_DEPARTED"
  },
  "event": {
    "code": "HUB_DEPARTED",
    "description": "Paket berangkat dari hub Bandung",
    "location": "Bandung",
    "occurred_at": "2026-07-29T07:19:58Z"
  }
}
```

API Kurir:

1. memverifikasi token, signature, timestamp, nonce, body size, dan schema;
2. menyimpan `event_id` secara unik;
3. mencatat event dan outbox dalam satu transaksi PostgreSQL;
4. mengembalikan `202 Accepted`;
5. meneruskan event ke Emisell secara asynchronous.

Duplikat `event_id` yang payload-nya identik tetap mendapat `202`.
`event_id` sama dengan payload berbeda mendapat `409 EVENT_CONFLICT`.

Partner wajib mempertahankan `event_id` yang sama pada retry dan mencoba ulang
dengan exponential backoff selama minimal 24 jam. API Kurir melakukan polling
rekonsiliasi bila webhook tidak tersedia.

## 9. Status canonical

| Status | Terminal | Keterangan |
|---|---:|---|
| `booking_pending` | Tidak | request diterima, belum ada konfirmasi carrier |
| `booked` | Tidak | booking berhasil |
| `pickup_requested` | Tidak | pickup diminta |
| `driver_searching` | Tidak | instant courier mencari driver |
| `driver_allocated` | Tidak | driver instant telah ditetapkan |
| `arrived_at_pickup` | Tidak | driver tiba di lokasi pickup |
| `picked_up` | Tidak | paket diambil carrier |
| `in_transit` | Tidak | perjalanan antar hub |
| `out_for_delivery` | Tidak | dibawa kurir tujuan |
| `delivery_failed` | Tidak | gagal antar, masih dapat dicoba kembali |
| `problem` | Tidak | perlu perhatian, belum final |
| `cancellation_pending` | Tidak | pembatalan belum final |
| `delivered` | Ya | diterima |
| `cancelled` | Ya | dibatalkan |
| `returning` | Tidak | retur sedang berjalan |
| `returned` | Ya | retur selesai |
| `damaged` | Ya | dinyatakan rusak sesuai status provider |
| `lost` | Ya | dinyatakan hilang |

`problem` dan `delivery_failed` tidak boleh langsung dianggap terminal.
Partner tetap mengirim raw status untuk audit, tetapi Emisell hanya bergantung
pada status canonical.

Event dengan `event_version` lebih kecil dari versi tersimpan dicatat sebagai
out-of-order dan tidak boleh menurunkan status shipment.

## 10. Error contract

```json
{
  "error": {
    "code": "INVALID_LOCATION",
    "message": "Destination tidak termasuk coverage.",
    "retryable": false,
    "request_id": "req_01J...",
    "details": {
      "field": "destination.official_code"
    }
  }
}
```

| HTTP | Code umum | Retry |
|---:|---|---:|
| 400 | `INVALID_REQUEST` | Tidak |
| 401 | `UNAUTHORIZED` | Tidak sebelum credential diperbaiki |
| 403 | `SCOPE_FORBIDDEN` | Tidak |
| 404 | `SHIPMENT_NOT_FOUND` | Tidak |
| 409 | `IDEMPOTENCY_CONFLICT`, `INVALID_STATE_TRANSITION` | Tidak |
| 422 | `INVALID_LOCATION`, `SERVICE_UNAVAILABLE` | Tidak |
| 429 | `RATE_LIMITED` | Ya, ikuti `Retry-After` |
| 500 | `INTERNAL_ERROR` | Ya secara terbatas |
| 502 | `CARRIER_ERROR` | Ya secara terbatas |
| 503 | `TEMPORARILY_UNAVAILABLE` | Ya, ikuti `Retry-After` |
| 504 | `PROVIDER_TIMEOUT` | Rekonsiliasi sebelum retry mutasi |

Partner harus mengirim header `Retry-After` pada `429` dan `503`.

## 11. Timeout, retry, dan circuit breaker

Default API Kurir:

| Operasi | Timeout |
|---|---:|
| health/capability | 2 detik |
| rate | 4 detik |
| rate instant | 6 detik |
| read shipment | 4 detik |
| create/cancel/pickup | 8 detik |
| label | 10 detik |
| tracking eksternal | 5 detik |

Retry otomatis hanya aman untuk:

- GET;
- rate request yang tidak membuat booking;
- POST dengan `Idempotency-Key` dan kontrak partner yang telah disertifikasi.

Circuit breaker dipisahkan per partner dan operasi. Kegagalan booking tidak
boleh mematikan pembacaan tracking atau webhook.

## 12. Keamanan data

Data berikut dianggap pribadi atau sensitif:

- nama, telepon, email, dan alamat;
- koordinat lokasi;
- isi dan nilai barang;
- AWB serta bukti penerimaan;
- driver contact;
- COD value, saldo, PIN, dan informasi pembayaran.

Kontrol minimum:

1. minimisasi field sesuai capability;
2. TLS selama transit;
3. enkripsi volume, backup, dan field sensitif di database;
4. secret disimpan melalui credential vault/encrypted database;
5. log hanya memakai masked AWB, shipment ID, request ID, dan error code;
6. role database dan dashboard dipisahkan;
7. retention ditentukan per tujuan dan kontrak;
8. data production dilarang masuk sandbox;
9. export data dan akses support tercatat pada audit log;
10. partner mempunyai proses incident response dan penghapusan data.

PIN pembayaran tidak boleh disimpan API Kurir. Jika partner membutuhkan PIN,
gunakan token sekali pakai atau mekanisme otorisasi partner.

## 13. Threat model

| Risiko | Kontrol wajib |
|---|---|
| Credential dicuri | vault, masking, rotasi, scope minimum, mTLS opsional |
| Webhook palsu | Bearer terpisah, HMAC, timestamp, nonce, constant-time compare |
| Replay webhook | unique event ID, nonce cache, batas waktu signature |
| Shipment ganda | idempotency key, unique shipment ID, rekonsiliasi |
| Harga dimanipulasi | signed quote ID, expiry, server-side validation |
| SSRF label/callback | HTTPS allowlist, blok private IP/redirect, fixed callback |
| PII bocor melalui log | structured logging allowlist, redaction, body logging off |
| Provider outage | timeout, circuit breaker, retry budget, fallback policy |
| Event out-of-order | event version dan state transition guard |
| Tenant melihat data tenant lain | tenant ownership check pada setiap shipment |

## 14. Rate limit

Rate limit dipisahkan per credential, partner, dan operasi. Response minimum:

```http
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 42
X-RateLimit-Reset: 1785292860
Retry-After: 30
```

Limit production harus menjadi bagian kontrak komersial. API Kurir tidak
mengasumsikan limit sandbox sama dengan production.

## 15. Versioning dan perubahan

- Minor additive change memakai versi kontrak yang sama.
- Breaking change memakai base path major baru, misalnya `/partner/v2`.
- Partner memberi pemberitahuan deprecation minimal 90 hari.
- API Kurir menjalankan contract test harian pada sandbox.
- Perbedaan sandbox dan production harus didokumentasikan sebagai capability,
  bukan diketahui setelah go-live.

## 16. Sertifikasi partner

Partner hanya berstatus `active` setelah:

### Legal dan komersial

- izin tertulis multi-merchant/white-label;
- izin menampilkan kembali tarif dan tracking;
- struktur biaya, COD, retur, settlement, serta klaim;
- SLA dan rate limit;
- data processing agreement;
- kontak insiden 24/7 atau sesuai SLA.

### Keamanan

- credential dan rotasi diuji;
- HMAC webhook lulus test vector;
- tidak ada credential pada log;
- replay dan duplicate event ditolak dengan benar;
- TLS dan certificate chain valid;
- vulnerability/security questionnaire selesai.

### Fungsional

- location dan coverage mapping;
- rate normal, cargo, COD, insurance, volumetric;
- duplicate create request;
- timeout saat create lalu rekonsiliasi;
- AWB asynchronous;
- pickup, cancel, dan label;
- event duplikat dan out-of-order;
- delivered, failed, problem, returned, dan cancelled;
- sandbox tidak memakai data pribadi production.

### Operasional

- dashboard health dan circuit breaker;
- alert credential, quota, timeout, dan webhook lag;
- runbook outage;
- rollback/disable satu partner tanpa deployment;
- rekonsiliasi shipment harian.

## 17. Partner yang telah mempunyai API native

RajaOngkir dan KiriminAja dipakai sebagai referensi kelengkapan fitur saat
merancang kontrak ini. Referensi tersebut tidak berarti API Kurir membuat
adapter untuk API native mereka.

Jika provider tersebut atau vendor lain ingin terhubung ke Emisell, provider
wajib menyediakan Partner Connector API v1 pada infrastrukturnya sendiri:

```text
API Kurir
  -> Partner Connector API v1 milik provider
       -> API/sistem native provider (dikelola provider)
```

Partner bertanggung jawab atas:

- penerjemahan official location code ke ID internal;
- pemetaan service/status native ke canonical;
- penyimpanan token native dan rotasinya;
- idempotency terhadap booking internal;
- penerimaan webhook carrier native;
- penerbitan webhook canonical bertanda tangan ke API Kurir;
- rekonsiliasi saldo, biaya, dan shipment internal.

API Kurir hanya menyimpan credential untuk koneksi canonical yang diterbitkan
partner. Token RajaOngkir, KiriminAja, carrier, atau sistem internal partner
tidak boleh dikirim melalui Provider Account API.

## 18. Dokumen pendukung

- `docs/api-surface-map.md`;
- `docs/provider-account-api-v1.md`;
- `docs/partner-webhooks-v1.md`;
- `docs/security-and-signing.md`;
- `docs/partner-certification.md`;
- `docs/partner-portal.md`.

## 19. OpenAPI

Kontrak machine-readable tersedia di:

```text
openapi/partner-v1.yaml
```

OpenAPI tersebut bersifat contract-first. Keberadaan endpoint dalam dokumen
tidak berarti endpoint sudah aktif di production.
