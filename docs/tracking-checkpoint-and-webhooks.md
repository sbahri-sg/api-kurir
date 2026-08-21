# Checkpoint Tracking Hemat dan Webhook Emisell

## Tujuan

API Kurir menjadi satu-satunya jalur tracking bagi Emisell. Seller dan customer
tidak memanggil RajaOngkir atau Biteship secara langsung. Satu AWB diperiksa
terjadwal lalu snapshot yang sama dapat dibaca berkali-kali tanpa menambah hit
provider.

Kebijakan default shared credential membatasi satu AWB hingga **10 hit provider
sepanjang siklus tracking**. Biteship hanya menjadi fallback untuk kurir yang
tidak dicakup RajaOngkir; gangguan sementara RajaOngkir tidak otomatis
menggandakan request ke Biteship.

## Alur Emisell

```text
Emisell membuat fulfillment + AWB
  -> POST /api/v1/integrations/tracking/subscriptions
  -> API Kurir mengenkripsi AWB dan membuat satu refresh job
  -> worker mengambil snapshot provider sesuai checkpoint
  -> perubahan status masuk ke transactional webhook outbox
  -> webhook HMAC dikirim ke backend Emisell
  -> Emisell memperbarui fulfillment/order
  -> seller dan customer membaca status lokal, tanpa hit provider
```

## Checkpoint economy

| Kondisi | Jadwal berikutnya |
|---|---:|
| AWB baru | langsung satu kali |
| `pending_pickup` pertama | 12 jam |
| `pending_pickup` berikutnya | 24 jam |
| `picked_up` / `in_transit` | 12 jam |
| `out_for_delivery` | 2 jam |
| `delivery_failed` | 12 jam |
| `delivered` / `returned` / `cancelled` | berhenti |
| Hit provider mencapai 10 | berhenti |

Timeout, rate limit, dan error provider juga disimpan sebagai checkpoint
negatif. Retry pertama dilakukan satu jam kemudian, retry kedua enam jam, lalu
24 jam. Selama jeda tersebut request berulang tidak menembak provider lagi.

Target operasionalnya adalah sekitar 6–10 hit selama umur satu pengiriman,
bukan polling tetap setiap beberapa jam. Membuka atau me-refresh halaman tidak
mengubah `next_refresh_at` dan tidak memanggil provider.

## Validasi resi acak

Format lokal ditolak sebelum provider bila AWB bukan 6–40 karakter
alfanumerik/tanda hubung. AWB yang formatnya benar tetapi belum ditemukan
menggunakan negative cache:

1. Pemeriksaan pertama: `not_found`, tunggu 12 jam.
2. Pemeriksaan kedua: `not_found`, tunggu 24 jam.
3. Pemeriksaan ketiga: `invalid`, polling dihentikan.

Nilai `validation_status` adalah `unverified`, `valid`, `not_found`, atau
`invalid`. Status `not_found` tidak langsung dianggap salah karena AWB baru
dapat membutuhkan waktu sebelum tersedia di sistem ekspedisi.

## Mendaftarkan fulfillment

Sebelum menyimpan AWB dari seller, Emisell sebaiknya memanggil
`POST /api/v1/tracking/verify`. Courier pilihan diperiksa lebih dulu. Bila tidak
ditemukan, API Kurir mencoba maksimal dua kandidat format terkuat. Pola hanya
petunjuk dan tidak menjadi alasan tunggal untuk menolak format baru.

Hasil `verified` boleh disimpan. `courier_mismatch` menyertakan
`detected_courier` agar seller dapat memperbaiki ekspedisi. `not_found` berarti
provider sudah diperiksa tetapi AWB belum ditemukan; ini berbeda dari
`invalid_format`.

```http
POST /api/v1/integrations/tracking/subscriptions
key: <customer-api-key>
X-Emisell-Merchant-ID: merchant_123
Content-Type: application/json
```

```json
{
  "order_id": "order_123",
  "fulfillment_id": "fulfillment_123",
  "courier": "jne",
  "waybill": "TEST123456789"
}
```

Merchant tidak diterima dari body. `merchant_id` selalu berasal dari header
backend. Upsert memakai `merchant_id + fulfillment_id`, sehingga retry dari
Emisell idempotent.

Respons `202` berarti validasi/refresh sedang antre. Respons `200` berarti
snapshot sudah tersedia.

## Mengganti AWB yang salah

AWB aktif tidak ditimpa langsung. Emisell membaca `revision` terbaru lalu
memanggil:

```http
PUT /api/v1/integrations/tracking/subscriptions/{fulfillment_id}
key: <customer-api-key>
X-Emisell-Merchant-ID: merchant_123
Content-Type: application/json

{
  "order_id": "order_123",
  "courier": "jnt",
  "waybill": "JY1224870535",
  "expected_revision": 1
}
```

AWB lama tetap aktif selama verifikasi. AWB baru hanya menjadi aktif bila
provider mengonfirmasi valid dan courier sesuai. Pergantian atomik menaikkan
revision dan menyimpan hubungan lama ke `tracking_subscription_revisions`.
Konflik revision menghasilkan HTTP 409. Shipment final dikunci agar order yang
sudah selesai tidak berubah akibat edit tidak sengaja. POST subscription tetap
idempotent untuk AWB yang sama, tetapi tidak dapat mengganti AWB diam-diam.

## Membaca snapshot tanpa hit provider

```http
GET /api/v1/integrations/tracking/subscriptions/{fulfillment_id}
key: <customer-api-key>
X-Emisell-Merchant-ID: merchant_123
```

Endpoint hanya membaca PostgreSQL dan tidak memanggil provider.

Field penting pada `shipment`:

```json
{
  "validation_status": "valid",
  "status": "in_transit",
  "status_label": "Dalam perjalanan",
  "provider_fetched_at": "2026-08-20T10:00:00Z",
  "next_refresh_at": "2026-08-20T22:00:00Z",
  "provider_hit_count": 3,
  "provider_hit_limit": 10,
  "polling_stopped": false
}
```

Endpoint kompatibel RajaOngkir `POST /api/v1/track/waybill` tetap tersedia.
Pada snapshot miss endpoint melakukan validasi sinkron satu kali; request ulang
dalam jendela checkpoint menggunakan snapshot/negative cache. Integrasi order
Emisell sebaiknya memakai subscription agar seluruh refresh berjalan di worker.

## Menghapus tracking milik merchant

Saat AWB dihapus dari fulfillment Emisell, backend memanggil:

```http
DELETE /api/v1/integrations/tracking/subscriptions/{fulfillment_id}
key: <customer-api-key>
X-Emisell-Merchant-ID: merchant_123
```

Penghapusan bersifat idempotent dan tenant-aware. Subscription menjadi tidak
aktif, GET berikutnya mengembalikan 404, webhook yang belum dikirim dibatalkan,
dan worker berhenti bila tidak ada subscription aktif lain pada snapshot yang
sama. Shipment, status history, dan revision tetap disimpan untuk audit. POST
dengan fulfillment yang sama dapat mengaktifkan tracking kembali, termasuk
dengan AWB baru.

```json
{
  "data": {
    "id": "subscription_uuid",
    "fulfillment_id": "fulfillment_123",
    "active": false,
    "revision": 1,
    "status": "removed",
    "polling_stopped": true,
    "snapshot_retained": true
  }
}
```

Hard delete shipment dan seluruh data turunannya tetap khusus admin development
melalui `/v1/admin/tracking-operations/{id}`.

## Webhook ke Emisell

Event yang dikirim:

- `tracking.validated`
- `tracking.status_changed`
- `tracking.delivered`
- `tracking.invalid`

Payload:

```json
{
  "id": "8fd90a2e-1c2e-4ca2-9b15-4546396d94de",
  "type": "tracking.delivered",
  "api_version": "2026-08-20",
  "occurred_at": "2026-08-20T10:00:00Z",
  "data": {
    "merchant_id": "merchant_123",
    "order_id": "order_123",
    "fulfillment_id": "fulfillment_123",
    "tracking_revision": 2,
    "shipment": {
      "courier": "jne",
      "waybill": "JY1224876789",
      "validation_status": "valid",
      "status": "delivered",
      "status_label": "Terkirim",
      "provider": "rajaongkir",
      "provider_fetched_at": "2026-08-20T10:00:00Z",
      "next_refresh_at": null,
      "is_final": true
    }
  }
}
```

Header keamanan:

```text
X-Emisell-Event-ID: <event-id>
X-Emisell-Event-Type: tracking.delivered
X-Emisell-Webhook-Timestamp: <unix-seconds>
X-Emisell-Webhook-Signature: v1=<hex-hmac-sha256>
```

String yang ditandatangani:

```text
<timestamp>.<raw-request-body>
```

Contoh verifikasi pada backend Node.js Emisell:

```js
import { createHmac, timingSafeEqual } from "node:crypto";

export function verifyApiKurirWebhook(rawBody, headers, secret) {
  const timestamp = headers["x-emisell-webhook-timestamp"];
  const received = headers["x-emisell-webhook-signature"];
  const timestampNumber = Number(timestamp);
  const age = Math.abs(Date.now() / 1000 - timestampNumber);

  if (!timestamp || !received || !Number.isFinite(timestampNumber) || age > 300) {
    return false;
  }

  const expected = "v1=" + createHmac("sha256", secret)
    .update(timestamp + ".")
    .update(rawBody)
    .digest("hex");
  const left = Buffer.from(received);
  const right = Buffer.from(expected);

  return left.length === right.length && timingSafeEqual(left, right);
}
```

`rawBody` wajib berupa byte/`Buffer` asli sebelum body diubah oleh JSON parser.
Setelah signature valid, simpan `X-Emisell-Event-ID` sebagai kunci deduplikasi.

Receiver wajib memverifikasi HMAC dengan perbandingan constant-time, menolak
timestamp terlalu lama, menyimpan event ID sebagai idempotency key, dan
mengabaikan event yang `provider_fetched_at`-nya lebih lama dari status
fulfillment saat ini. Event dengan `tracking_revision` lebih kecil daripada
revision aktif di Emisell juga wajib diabaikan. Respons
2xx menandai event terkirim. Network error, 429, dan 5xx diulang dengan interval
1 menit, 5 menit, 30 menit, 2 jam, 6 jam, lalu 24 jam. Respons 4xx lain masuk
dead-letter tanpa retry agresif.

URL webhook hanya berasal dari konfigurasi operator, bukan request seller.
Deployment baru mengelolanya dari menu **Developer → Webhook**. Secret dibuat
backend, ditampilkan satu kali, kemudian disimpan terenkripsi di database.
Worker membaca perubahan tanpa restart.

Konfigurasi environment berikut hanya menjadi fallback untuk deployment lama
yang belum mempunyai konfigurasi database:

```text
TRACKING_WEBHOOK_ENABLED=true
EMISELL_TRACKING_WEBHOOK_URL=https://api.emisell.com/api/webhooks/api-kurir/tracking
EMISELL_TRACKING_WEBHOOK_SECRET=<minimal-32-karakter>
```

Endpoint admin pengelolaan:

| Method | Endpoint | Fungsi |
|---|---|---|
| GET | `/v1/admin/tracking-webhook` | Status aman tanpa plaintext secret |
| PUT | `/v1/admin/tracking-webhook` | URL dan aktivasi |
| POST | `/v1/admin/tracking-webhook/secret` | Generate/rotate; secret tampil satu kali |
| POST | `/v1/admin/tracking-webhook/test` | Event test tanpa data pelanggan |

## Aturan update order Emisell

- `tracking.invalid` hanya memberi peringatan seller; jangan mengubah status order.
- `in_transit` memperbarui shipment fulfillment menjadi `IN_TRANSIT`.
- `tracking.delivered` mengubah fulfillment terkait menjadi `DELIVERED/FULFILLED`.
- Order menjadi `COMPLETED` hanya jika seluruh fulfillment aktif sudah delivered.
- Receiver selalu mencocokkan `merchant_id`, `order_id`, dan `fulfillment_id`.

## Penyimpanan

`tracking_shipments` menyimpan snapshot terbaru. `tracking_status_history` hanya
menambah baris ketika status berubah, bukan setiap polling. Webhook menggunakan
transactional outbox agar perubahan database dan event tidak terpisah. Riwayat
order permanen tetap dimiliki Emisell.

Operator memantau data ini melalui menu **Operasional → Monitor Resi**. Tabel
bersumber dari `GET /v1/admin/tracking-operations`, diperbarui setiap
15 detik, dan dapat difilter berdasarkan merchant/order, courier, validasi,
serta antrean `pending`, `running`, `dead`, `final`, atau `idle`. Plaintext AWB
ditampilkan penuh hanya pada response endpoint admin ini setelah didekripsi di
memory. Endpoint customer, webhook, audit, dan log tetap memakai AWB termasking.

Untuk pembersihan data development, staff dapat memanggil
`DELETE /v1/admin/tracking-operations/{id}` dari action **Hapus permanen**.
Endpoint tetap tersedia di production untuk dashboard internal, dilindungi
admin API key, memakai UUID shipment, meminta konfirmasi pada UI, dan mencatat
audit. Seluruh data operasional turunan ikut dihapus secara atomik.

Target retention operasional:

| Data | Masa simpan |
|---|---:|
| Shipment aktif | sampai status final |
| Shipment final dan status history | 90 hari |
| AWB `not_found` / `invalid` | 7 hari |
| Percobaan provider | 14 hari |
| Webhook berhasil | 30 hari |
| Dead-letter webhook | 90 hari |
| Statistik kuota anonim | 12 bulan |

Pembersihan retention dijalankan sebagai maintenance terpisah; perubahan ini
tidak menghapus data lama secara otomatis. AWB tetap terenkripsi selama masa
simpan dan plaintext tidak dimasukkan ke payload webhook atau log.
