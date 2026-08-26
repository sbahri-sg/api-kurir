# Emisell Fulfillment Gateway

Dokumen ini menjelaskan kontrak internal antara Main Service Emisell dan API
Kurir untuk booking pengiriman. Kontrak ini northbound dan stabil; bentuk API
native RajaOngkir, Biteship, atau partner lain tidak diteruskan ke Emisell.

## Alur

1. Merchant mengaktifkan tepat satu provider dari katalog API Kurir.
2. Checkout memperoleh quote ongkir dari endpoint rate yang sudah ada.
3. Main Service membuat shipment dengan `merchant_reference`, `quote_id`, dan
   `Idempotency-Key` yang stabil.
4. API Kurir mereservasi operasi di PostgreSQL sebelum memanggil provider.
5. Setelah booking berhasil, Main Service menjadwalkan pickup dan mengambil
   label dari `shipment_id` internal.
6. Jika AWB belum tersedia, worker merekonsiliasi detail order provider tanpa
   mengulangi create shipment. Jadwal awal berjalan 15 menit setelah booking,
   lalu mengikuti retry bertahap sampai AWB atau status final ditemukan.
7. Ketika AWB tersedia dari pickup atau rekonsiliasi, API Kurir otomatis
   mendaftarkannya ke pipeline tracking durable. Main Service tidak perlu
   memanggil endpoint subscription tracking lagi untuk shipment ini.
8. Perubahan lifecycle fulfillment dan tracking dikirim ke webhook Emisell
   melalui outbox PostgreSQL yang terpisah, ditandatangani HMAC, dan dapat
   di-retry tanpa membuat shipment baru.

## Endpoint

| Method | Path | Fungsi |
| --- | --- | --- |
| `POST` | `/api/v1/integrations/shipments` | Membuat booking shipment |
| `GET` | `/api/v1/integrations/shipments/{shipment_id}` | Membaca snapshot shipment |
| `POST` | `/api/v1/integrations/shipments/{shipment_id}/pickup` | Menjadwalkan pickup |
| `GET` | `/api/v1/integrations/shipments/{shipment_id}/label` | Mengambil label terenkripsi/cache-first |
| `POST` | `/api/v1/integrations/shipments/{shipment_id}/cancel` | Meminta pembatalan |
| `GET` | `/api/v1/integrations/shipments/{shipment_id}/history` | Membaca timeline audit |

Semua endpoint memakai dedicated Main Service API key dan header
`X-Emisell-Merchant-ID`. Semua `POST` yang menimbulkan side effect juga wajib
memakai `Idempotency-Key` sepanjang 8–128 karakter.

## Aturan idempotency

- key yang sama dan payload yang sama mengembalikan hasil lama;
- key yang sama dengan payload berbeda menghasilkan HTTP `409`;
- `merchant_reference` tidak boleh membuat dua shipment pada merchant yang
  sama;
- jika koneksi provider timeout setelah request dikirim, status operasi tetap
  tersimpan dan API tidak melakukan retry booking otomatis. Admin harus
  merekonsiliasi hasil provider agar tidak tercipta dua AWB.

## Credential dan provider

RajaOngkir mempunyai dua capability credential:

- `shipping_api_key`: ongkir dan tracking;
- `delivery_api_key`: create shipment, baca detail order, pickup, label, dan
  cancel (`shipments:write`, `shipments:read`, `pickup:write`, `labels:read`,
  dan `shipments:cancel`).

Credential BYOK disimpan terenkripsi dan dipilih dari provider aktif merchant.
Merchant dengan provider built-in `emisell` memakai pool platform. Provider
lain tidak boleh meminjam credential atau saldo platform.

Adapter RajaOngkir memakai base URL Shipping Delivery
`https://api.collaborator.komerce.id`. Sandbox harus memakai base URL dan key
sandbox tersendiri. Endpoint internal tidak berubah ketika environment provider
diganti.

## Perlindungan data

Alamat, telepon, email, detail barang, dan label disimpan dengan AES-256-GCM
menggunakan master key credential yang sudah diwajibkan server. Log HTTP tidak
mencetak request body atau credential. Query shipment selalu memuat
`tenant_id`, sehingga UUID dari merchant lain tetap menghasilkan `404`.

## Status canonical

Status awal adalah `booking_pending`, kemudian `booked`, `pickup_requested`,
`picked_up`, `in_transit`, `out_for_delivery`, dan `delivered`. Jalur gagal atau
pembatalan memakai `booking_failed`, `problem`, `cancellation_pending`, dan
`cancelled`. Status mentah provider disimpan terpisah sebagai
`provider_status`.

## Lifecycle otomatis setelah booking

Snapshot `shipment` juga mengembalikan field operasional berikut:

- `tracking_registration_status`: `not_ready`, `pending`, `registered`, atau
  `failed`;
- `tracking_shipment_id`: ID snapshot tracking setelah AWB berhasil didaftarkan;
- `live_tracking_url`: URL provider bila tersedia;
- `last_reconciled_at` dan `next_reconcile_at`;
- `reconcile_attempt_count` dan `reconcile_error`.

Worker memproses dua jenis job durable: `reconcile` dan `register_tracking`.
Job diklaim dengan locking PostgreSQL sehingga beberapa replica worker aman
berjalan bersamaan. Job yang berhenti di tengah proses dapat diklaim ulang
setelah lease lima menit. Retry gagal bersifat bertahap dan berhenti setelah
batas percobaan atau shipment mencapai `delivered`/`cancelled`.

## Webhook fulfillment

Webhook memakai destination dan secret yang sama dengan webhook tracking.
Request menggunakan `POST`, `Content-Type: application/json`, serta header
`X-Emisell-Event-ID`, `X-Emisell-Event-Type`,
`X-Emisell-Webhook-Timestamp`, dan
`X-Emisell-Webhook-Signature: v1=<hex-hmac>`.

Event lifecycle yang tersedia adalah `shipment.booked`,
`shipment.pickup_requested`, `shipment.awb_created`,
`shipment.status_changed`, `shipment.delivered`, dan `shipment.cancelled`.
Payload `data` berisi snapshot shipment canonical dan tidak memuat alamat,
telepon, email, isi paket, maupun secret credential.

Outbox melakukan retry setelah 1 menit, 5 menit, 30 menit, 2 jam, 6 jam, dan
24 jam. HTTP `2xx` dianggap berhasil; respons lain disimpan sebagai bukti
operasional untuk ditinjau staff.

## Operasional admin

Menu **Monitor Fulfillment** membaca
`GET /v1/admin/fulfillment-operations`. Staff dapat melihat status provider,
AWB, registrasi tracking, antrean lifecycle, error terakhir, dan status webhook.
Tombol refresh memakai
`POST /v1/admin/fulfillment-operations/{shipment_id}/reconcile`; endpoint ini
hanya menambah job rekonsiliasi dan tidak pernah mengulang create shipment.

## Batas implementasi saat ini

Adapter transaksi aktif pertama adalah RajaOngkir Shipping Delivery. Detail
order provider saat ini direkonsiliasi melalui polling hemat; penerimaan webhook
native provider menjadi optimasi berikutnya dan bukan syarat agar lifecycle
berjalan. Provider
partner-hosted akan menggunakan kontrak `/partner/v1`, tetapi baru boleh
menjalankan transaksi setelah package, credential, dan release partner sudah
dipublikasikan. API Kurir tidak melakukan fallback transaksi ke provider lain
setelah booking dimulai karena dapat membuat dua AWB dan dua tagihan.
