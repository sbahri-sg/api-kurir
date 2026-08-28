# Emisell Fulfillment Gateway

Dokumen ini menjelaskan kontrak internal antara Main Service Emisell dan API
Kurir untuk booking pengiriman. Kontrak ini northbound dan stabil; bentuk API
native RajaOngkir, Biteship, atau partner lain tidak diteruskan ke Emisell.

## Alur

1. Merchant mengaktifkan tepat satu provider dari katalog API Kurir.
2. Checkout memanggil `POST /api/v1/integrations/shipping-quotes`. API Kurir
   mengambil quote dari produk fulfillment provider, menyimpan service native
   dan harga server-side, lalu mengembalikan `quote_id` canonical `fq_...`.
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
| `POST` | `/api/v1/integrations/shipping-quotes` | Mengambil dan mengunci quote fulfillment |
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
- booking yang sebelumnya gagal tetap mengembalikan error provider semula dan
  tidak pernah berubah menjadi respons sukses saat key yang sama diulang;
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
`https://api.collaborator.komerce.id`. Sandbox memakai base URL dan key sandbox
tersendiri. Credential sandbox disimpan sebagai record terpisah dengan
`environment: sandbox` serta menerima `shipping_api_key` dan
`delivery_api_key`. Shipping Cost tetap memanggil produk live/read-only karena
rate dan tracking RajaOngkir tidak mempunyai endpoint sandbox terpisah; hanya
Shipping Delivery yang berpindah ke endpoint sandbox. Saat credential disimpan atau dirotasi, API Kurir
menguji Shipping Cost dan Shipping Delivery melalui endpoint read-only produk
masing-masing. Key invalid ditolak sebelum bundle aktif berubah.

Setiap mode instalasi RajaOngkir dapat menerima kedua key:

1. `environment: live` berisi `shipping_api_key` dan, jika fulfillment live
   digunakan, `delivery_api_key` production;
2. `environment: sandbox` berisi `shipping_api_key` Shipping Cost yang sama dan
   `delivery_api_key` sandbox. Rate/tracking tetap nyata dan dapat mengurangi
   kuota Shipping Cost.

Jika `environment` tidak dikirim, API selalu memilih `live`. Karena itu payload
yang berisi kedua key tanpa `environment` hanya mengonfigurasi live. API tidak
menyalin secret otomatis; saat seller memilih sandbox, form mengirim kedua key
ke bundle sandbox.

Main Service tidak perlu meneruskan header environment. Pada quote fulfillment,
API Kurir memilih credential live valid lebih dahulu dan memakai sandbox bila
live tidak tersedia. Environment tersebut dikunci pada quote dan disimpan pada
shipment. Create shipment, pickup, cancel, detail, label, serta worker
rekonsiliasi selalu memakai environment shipment yang sama sehingga tidak
berpindah mode di tengah lifecycle.

`service_code`, `shipping_cost`, serta komponen total pada create shipment
diambil kembali dari snapshot `fq_` oleh API Kurir. Kode native seperti
`JNEFlat` tidak dikirim ke Emisell; response memakai kode canonical `svc_...`
dan label `Regular`, `Next Day`, `Economy`, atau `Cargo`.

## Kontrak payload shipment

Create shipment hanya menerima satu bentuk payload paket yang mengikuti data
order Emisell. Field lama `item_value`, `contents`, `sku`, `unit_value`,
dimensi per item, `funding_source`, dan `shipping_cashback` sudah dihapus dan
akan menghasilkan `400 INVALID_REQUEST` bila masih dikirim.

```json
{
  "package": {
    "weight_grams": 1350,
    "length_cm": 20,
    "width_cm": 15,
    "height_cm": 10,
    "items": [
      {
        "name": "Kaos",
        "variant": "M",
        "quantity": 1,
        "unit_price": 150000,
        "subtotal": 130000,
        "weight_grams": 1200
      }
    ]
  },
  "payment": {
    "type": "non_cod",
    "items_subtotal": 130000,
    "order_discount": 0,
    "tax_amount": 0,
    "shipping_cost": 18000,
    "shipping_discount": 5000,
    "additional_cost": 0,
    "grand_total": 143000
  }
}
```

`subtotal` item adalah total baris setelah diskon produk. API memverifikasi
`items_subtotal` sama dengan jumlah seluruh subtotal item, berat paket tidak
lebih kecil daripada jumlah berat item, dan menghitung:

```text
grand_total = items_subtotal - order_discount + tax_amount
            + shipping_cost - shipping_discount
            + additional_cost + penyesuaian biaya/diskon provider dari quote
```

Nilai barang untuk mencocokkan quote dihitung oleh API Kurir sebagai
`items_subtotal - order_discount + tax_amount`; Main Service tidak lagi
mengirim `item_value` pada create shipment. `item_value` tetap digunakan hanya
pada request shipping quote. Selisih nominal menghasilkan
`422 AMOUNT_MISMATCH` sebelum hit provider sehingga tidak dapat membuat AWB
dengan total yang salah.

Endpoint `/api/v1/calculate/district/domestic-cost` menggunakan produk
RajaOngkir Shipping Cost. Responsnya tetap sah untuk cek ongkir dan checkout
tanpa fulfillment, tetapi tidak menjadi sumber booking Shipping Delivery.
Quote terkunci pada merchant, provider, environment, credential, origin,
destination, berat, dimensi, nilai barang, jenis pembayaran, service, harga,
dan waktu berlaku. Perubahan menghasilkan `409 QUOTE_MISMATCH`; quote
kedaluwarsa menghasilkan `409 QUOTE_EXPIRED`; pemakaian kedua menghasilkan
`409 QUOTE_ALREADY_USED`.

RajaOngkir Hosted mengambil quote dari `/tariff/api/v1/calculate` Shipping
Delivery. Provider partner lain menggunakan operasi quote fulfillment pada
connector masing-masing. Dengan demikian Main Service selalu memakai kontrak
yang sama untuk RajaOngkir, Mengantar, KiriminAja, atau provider berikutnya.

Jika provider sudah menolak booking, API mempertahankan `merchant_reference`
dan `Idempotency-Key` sebagai bukti percobaan. Retry payload identik mengembalikan
error lama; payload yang diperbaiki wajib memakai reference dan key baru. Aturan
ini mencegah dua AWB ketika status upstream belum pasti.

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

Pickup hanya dapat dijadwalkan ketika booking berstatus `booked` dan
`provider_shipment_id` sudah tersedia. Shipment `booking_pending`,
`booking_failed`, `problem`, atau booking tanpa ID provider ditolak dengan HTTP
`409 PICKUP_NOT_ALLOWED` tanpa memanggil provider. Request pickup yang sudah
berstatus `pickup_requested` dibaca sebagai replay agar tidak membuat pickup
ganda. Connector juga memeriksa hasil setiap order di dalam respons RajaOngkir.
HTTP 2xx dari envelope tidak dianggap berhasil bila item order berstatus
`failed`; API mengembalikan `422 PROVIDER_REJECTED`, shipment tetap `booked`,
dan operator dapat memperbaiki jadwal atau data lalu mencoba kembali.

Endpoint tetap `POST /api/v1/integrations/shipments/{shipment_id}/pickup` agar
Main Service tidak perlu mengikuti kontrak native setiap provider. Body
canonical hanya mempunyai dua bentuk:

```json
{ "mode": "now" }
```

atau:

```json
{
  "mode": "scheduled",
  "scheduled_at": "2026-08-28T09:00:00+07:00"
}
```

`scheduled_at` wajib berada di masa depan untuk mode `scheduled` dan tidak
boleh dikirim pada mode `now`. API Kurir memberi lead time singkat untuk mode
`now`. Emisell tidak mengirim `vehicle`, alamat pickup, berat, maupun rincian
paket lagi karena semuanya sudah terkunci pada snapshot shipment. Adapter
provider menerjemahkan snapshot tersebut. Untuk RajaOngkir, kendaraan dipilih
dari berat paket: sampai 5 kg `Motor`, di atas 5 kg dan di bawah 10 kg `Mobil`,
serta mulai 10 kg `Truk`. Aturan provider lain tetap berada di adapter masing-
masing tanpa mengubah endpoint Emisell.

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
