# Partner Event Webhook v1

Status: **contract-first**
Audience: **partner dan tim integrasi API Kurir**

## 1. Arah komunikasi

Partner mengirim event ke callback tetap milik API Kurir:

```text
POST https://api-kurir.example.com/v1/partner-events/{connection_id}
```

URL callback dibuat API Kurir saat onboarding. Partner tidak boleh menerima URL
callback arbitrary pada setiap shipment.

## 2. Autentikasi

Webhook standard wajib membawa:

```http
Authorization: Bearer <event-token>
X-Partner-Key-Id: <event-key-id>
X-Event-Id: evt_01J...
X-Signature-Version: v1
X-Signature-Timestamp: 1785290400
X-Signature-Nonce: 01J...
X-Signature: <base64url-hmac-sha256>
Content-Type: application/json
```

Event token dan signing secret harus berbeda dari credential Partner Connector
API. Detail canonical string dan rotasi secret berada di
`docs/security-and-signing.md`.

## 3. Envelope

```json
{
  "event_id": "evt_01J...",
  "event_type": "shipment.in_transit",
  "event_version": 1,
  "occurred_at": "2026-07-29T06:10:00Z",
  "shipment": {
    "shipment_id": "shp_01J...",
    "partner_shipment_id": "VENDOR-10001",
    "awb": "AWB123",
    "status": "in_transit",
    "partner_status": "WITH_COURIER"
  },
  "event": {
    "code": "WITH_COURIER",
    "description": "Paket dibawa kurir",
    "location": "Bandung",
    "occurred_at": "2026-07-29T06:09:55Z"
  }
}
```

## 4. Event canonical

| Event | Makna |
|---|---|
| `shipment.booked` | booking diterima |
| `shipment.awb_assigned` | AWB tersedia |
| `shipment.pickup_requested` | pickup diminta |
| `shipment.driver_searching` | instant courier mencari driver |
| `shipment.driver_allocated` | driver instant ditetapkan |
| `shipment.driver_arrived` | driver tiba di pickup |
| `shipment.picked_up` | paket telah diambil |
| `shipment.in_transit` | paket dalam perjalanan |
| `shipment.out_for_delivery` | kurir menuju penerima |
| `shipment.delivery_failed` | percobaan antar gagal |
| `shipment.problem` | terjadi masalah operasional |
| `shipment.delivered` | diterima |
| `shipment.cancellation_pending` | pembatalan belum final |
| `shipment.cancelled` | pembatalan final |
| `shipment.returning` | proses retur |
| `shipment.returned` | retur selesai |
| `shipment.damaged` | paket rusak |
| `shipment.lost` | paket hilang |
| `shipment.cost_adjusted` | biaya aktual berubah |
| `shipment.refunded` | refund provider tercatat |

Partner tetap mengirim `partner_status` dan raw event code agar mapping dapat
diaudit tanpa mengekspos raw payload ke Emisell.

## 5. Event instant

Event instant dapat menambahkan:

```json
{
  "driver": {
    "name": "Nama Driver",
    "phone": "628123456789",
    "photo_url": "https://cdn.partner.example/driver/123"
  },
  "live_tracking": {
    "url": "https://tracking.partner.example/t/opaque-token",
    "expires_at": "2026-07-29T08:00:00Z",
    "polyline": null
  }
}
```

`live_tracking.url`, driver, dan polyline bersifat opsional. API Kurir tidak
boleh menyimpulkan ketersediaan peta realtime hanya dari capability tracking.

## 6. Proof of delivery

Event delivered dapat membawa POD:

```json
{
  "proof_of_delivery": {
    "receiver_name": "Budi",
    "received_at": "2026-07-29T07:30:00Z",
    "photo_url": "https://cdn.partner.example/pod/opaque",
    "signature_url": null
  }
}
```

URL harus HTTPS, berumur pendek, dan tidak boleh menunjuk private IP. API Kurir
menyalin bukti ke object storage privat bila kontrak dan retention policy
mengizinkan. URL provider tidak diteruskan langsung tanpa validasi.

## 7. Event biaya

```json
{
  "event_type": "shipment.cost_adjusted",
  "cost": {
    "quoted_total": 18000,
    "booked_total": 18000,
    "actual_total": 21500,
    "currency": "IDR",
    "adjustment": 3500,
    "reason_code": "INSTANT_SURGE",
    "reason": "Harga berubah saat driver dialokasikan"
  }
}
```

Event biaya tidak langsung mengubah tagihan seller. API Kurir membuat ledger
dan menjalankan rekonsiliasi terlebih dahulu.

## 8. Idempotency dan urutan

- `event_id` unik minimal 30 hari.
- Retry event yang sama harus mempertahankan `event_id` dan body yang identik.
- Event ID sama dengan body berbeda menghasilkan `409`.
- Event boleh tiba tidak berurutan.
- API Kurir membandingkan `occurred_at`, versi status, dan state machine.
- Event lama disimpan sebagai audit tetapi tidak boleh menurunkan status final.

Response:

```json
{
  "event_id": "evt_01J...",
  "accepted": true,
  "duplicate": false
}
```

API Kurir mengembalikan `202` setelah event masuk durable inbox, bukan setelah
seluruh proses downstream selesai.

## 9. Retry partner

Retry hanya untuk timeout, koneksi gagal, `408`, `425`, `429`, dan `5xx`.

Jadwal minimum:

```text
0 detik
30 detik
2 menit
10 menit
30 menit
2 jam
6 jam
24 jam
```

Gunakan exponential backoff dan jitter. `4xx` selain `408`, `425`, dan `429`
tidak boleh diulang tanpa perubahan konfigurasi.

## 10. Outbox dan inbox

Partner wajib memakai transactional outbox agar event tidak hilang setelah
status shipment berubah. API Kurir memakai durable inbox dengan unique index
pada:

```text
connection_id + event_id
```

Raw payload terenkripsi hanya disimpan selama masa investigasi yang disetujui.
Payload canonical disimpan lebih lama sesuai kebijakan shipment.

## 11. Rekonsiliasi

Webhook bukan satu-satunya sumber kebenaran. API Kurir memanggil detail atau
tracking partner ketika:

- webhook terlambat melewati SLA;
- event melompat melewati state penting;
- signature legacy tidak dapat diverifikasi;
- biaya aktual berubah;
- shipment belum final setelah batas waktu;
- operator meminta audit.

## 12. Webhook native partner

Partner boleh menerima webhook dari carrier atau sistem native dengan format
apa pun pada infrastrukturnya sendiri. Sebelum dikirim ke API Kurir, partner
wajib:

- memverifikasi webhook native sesuai aturan sistem asal;
- melakukan dedup dan state transition;
- memetakan raw status ke status canonical;
- menerbitkan `event_id` stabil;
- menandatangani event canonical dengan HMAC v1;
- menyediakan endpoint rekonsiliasi shipment.

API Kurir tidak menerima webhook native RajaOngkir, KiriminAja, atau carrier
sebagai bagian onboarding partner. Hanya Partner Event Webhook canonical yang
diterima.
