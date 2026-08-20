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

```http
POST /api/v1/integrations/tracking/subscriptions
key: <customer-api-key>
X-Emisell-Tenant-Token: <tenant-jwt>
Content-Type: application/json
```

Tenant token memerlukan scope `tracking:write`.

```json
{
  "order_id": "order_123",
  "fulfillment_id": "fulfillment_123",
  "courier": "jne",
  "waybill": "TEST123456789"
}
```

Merchant tidak diterima dari body. `merchant_id` selalu berasal dari claim
`sub`, sedangkan `domain_id` menjadi metadata dari tenant token. Upsert memakai
`merchant_id + fulfillment_id`, sehingga retry dari Emisell idempotent.

Respons `202` berarti validasi/refresh sedang antre. Respons `200` berarti
snapshot sudah tersedia.

## Membaca snapshot tanpa hit provider

```http
GET /api/v1/integrations/tracking/subscriptions/{fulfillment_id}
key: <customer-api-key>
X-Emisell-Tenant-Token: <tenant-jwt>
```

Tenant token memerlukan scope `tracking:read`. Endpoint hanya membaca
PostgreSQL dan tidak memanggil provider.

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
    "domain_id": "domain_abc",
    "order_id": "order_123",
    "fulfillment_id": "fulfillment_123",
    "shipment": {
      "courier": "jne",
      "waybill": "********6789",
      "validation_status": "valid",
      "status": "delivered",
      "status_label": "Terkirim",
      "provider": "rajaongkir",
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

Receiver wajib memverifikasi HMAC dengan perbandingan constant-time, menolak
timestamp terlalu lama, menyimpan event ID sebagai idempotency key, dan
mengabaikan event yang `provider_fetched_at`-nya lebih lama dari status
fulfillment saat ini. Respons
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
