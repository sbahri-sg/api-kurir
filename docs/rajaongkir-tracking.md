# Operasional Adapter Tracking RajaOngkir

## 1. Scope

Adapter menggunakan API Shipping Cost RajaOngkir V2:

```text
POST https://rajaongkir.komerce.id/api/v1/track/waybill
key: <shipping-cost-api-key>
Content-Type: application/x-www-form-urlencoded
```

Form upstream:

```text
awb
courier
last_phone_number (opsional untuk kompatibilitas)
```

Sumber resmi:

- [Tracking AWB](https://www.rajaongkir.com/docs/shipping-cost/tracking)
- [Courier Availability](https://www.rajaongkir.com/docs/shipping-cost/getting_started/courier_availability)
- [Authorization](https://rajaongkir.com/docs/shipping-cost/getting_started/apikey)

## 2. Kurir aktif

Matriks capability RajaOngkir yang ditinjau pada 28 Juli 2026 menandai AWB
aktif untuk:

| Kurir | Code |
|---|---|
| JNE | `jne` |
| SAP Express | `sap` |
| Ninja | `ninja` |
| J&T Express | `jnt` |
| TIKI | `tiki` |
| Wahana Express | `wahana` |
| POS Indonesia | `pos` |
| Lion Parcel | `lion` |

SiCepat, IDExpress, Sentral Cargo, dan REX tidak dimasukkan ke konfigurasi
default karena capability AWB tidak ditandai aktif pada matriks tersebut.
Capability provider dapat berubah, sehingga daftar harus diverifikasi sebelum
diubah.

## 3. Aktivasi

Tambahkan key RajaOngkir dari `Dashboard > Kuota provider > Tambah key`.
Provider key disimpan terenkripsi di database dan langsung tersedia bagi
worker tanpa perlu menambahkannya ke `.env`.

Konfigurasi worker:

```text
TRACKING_ENABLED=true
TRACKING_ENCRYPTION_KEY=<base64 yang decode menjadi 32 byte>
TRACKING_WORKER_CONCURRENCY=8
RAJAONGKIR_TRACKING_COURIERS=jne,sap,ninja,jnt,tiki,wahana,pos,lion
RAJAONGKIR_TIMEOUT=4s
```

Rate dan tracking menggunakan pool key serta quota ledger yang sama.
`PROVIDER_CREDENTIAL_ENCRYPTION_KEY` harus identik pada proses API dan worker.

## 4. Alur

Mode SDK RajaOngkir V2 bersifat sinkron:

```text
POST /api/v1/track/waybill?awb=<nomor-resi>&courier=<kode-kurir>
key: <customer-api-key>
  -> validasi dan hash
  -> snapshot fresh/final tersedia: respons langsung tanpa hit provider
  -> snapshot miss/stale: satu hit provider, simpan snapshot, lalu respons
```

Mode API Kurir lama tetap asynchronous:

```text
POST /v1/track/waybill
Authorization: Bearer <customer-api-key>
  -> validasi courier dan AWB
  -> hash untuk deduplikasi
  -> enkripsi AWB dan provider context
  -> upsert shipment
  -> satu active refresh job
  -> worker claim dengan SKIP LOCKED
  -> consume quota ledger secara atomik
  -> call RajaOngkir
  -> normalisasi status dan event
  -> simpan snapshot non-sensitif
  -> jadwalkan refresh berikutnya bila belum final
```

Nomor resi tidak dikirim ke log. Tabel `provider_api_calls` hanya menyimpan
fingerprint SHA-256, endpoint, status HTTP, durasi, outcome, quota cost, dan
error code.

Satu proses worker menjalankan delapan consumer secara default. Nilai
`TRACKING_WORKER_CONCURRENCY` dibatasi `1–64`. PostgreSQL `SKIP LOCKED` dan
partial unique index tetap mencegah dua consumer memproses job yang sama.
Nilai ini adalah batas per proses, bukan batas global per credential. Mulai
dengan satu instance worker; sebelum horizontal scaling, tambahkan limiter
terdistribusi jika kontrak provider menetapkan batas request per detik.

## 5. Data sensitif

Dashboard tidak meminta nomor telepon penerima. Field `last_phone_number`
hanya dipertahankan sebagai konteks provider opsional untuk kompatibilitas.

Database menyimpan:

```text
waybill_hash
waybill_masked
waybill_ciphertext
provider_context_ciphertext
summary_json
events_json
```

Raw response provider dan nomor telepon lengkap tidak disimpan. Untuk
kompatibilitas respons V2, snapshot terstruktur dapat memuat nama
pengirim/penerima serta alamat yang memang dikembalikan provider. Batasi akses
database, enkripsi volume/backup, terapkan retention, dan jangan mencetak field
tersebut pada log.

## 6. Normalisasi status dan interval

| Status lokal | Refresh berikutnya |
|---|---:|
| `out_for_delivery` | 15 menit |
| `in_transit` | 45 menit |
| `picked_up` | 60 menit |
| `delivery_failed` | 2 jam |
| `pending_pickup` / `unknown` | 3 jam |
| `delivered` / `returned` / `cancelled` | tidak polling lagi |

Interval ini merupakan kebijakan operasional API Kurir, bukan SLA carrier.
Ubah setelah memiliki data produksi tentang latency update dan konsumsi quota.

## 7. Error dan retry

| Kondisi | Error job | Kebijakan |
|---|---|---|
| Local/upstream quota habis | `PROVIDER_QUOTA_EXHAUSTED` | tunggu reset Jakarta + 5 menit |
| HTTP 429 / throttle sementara | `PROVIDER_RATE_LIMITED` | exponential 30 detik, maksimum 5 menit |
| Credential ditolak | `PROVIDER_UNAUTHORIZED` | retry 6 jam; alert operator |
| Resi belum ditemukan | `WAYBILL_NOT_FOUND` | 15 menit, exponential hingga 8 jam |
| Validasi telepon kurang | `PHONE_VALIDATION_REQUIRED` | 24 jam; perbaiki input |
| Timeout/5xx/network | `PROVIDER_ERROR` | exponential 1–64 menit |

Job menjadi `dead` setelah `max_attempts`. Proses requeue manual harus
memeriksa penyebab terlebih dahulu agar tidak membakar quota.

## 8. Efisiensi hit

- Satu pasangan `courier + waybill` menghasilkan satu shipment.
- Partial unique index mencegah dua job aktif untuk shipment yang sama.
- Banyak customer membaca snapshot lokal yang sama.
- Request sinkron bersamaan untuk resi yang sama digabung per proses API.
- Status final tidak dipolling lagi.
- Refresh menyesuaikan status; tidak memakai interval agresif global.
- Ledger dicek sebelum request keluar.
- Beberapa credential aktif dirotasi otomatis berdasarkan rasio pemakaian;
  key yang habis kuota dilewati sampai reset.

Gunakan beberapa akun hanya bila skema tersebut diizinkan oleh kontrak
provider.

## 9. Checklist go-live

1. Rotasi credential yang pernah terekspos dan tambahkan key baru dari
   dashboard.
2. Konfirmasi paket serta quota aktual pada dashboard RajaOngkir.
3. Jalankan migration `000005_rajaongkir_tracking_adapter`.
4. Uji satu resi non-produksi atau resi milik sendiri per kurir.
5. Pastikan plaintext AWB/phone tidak ada di database dan log.
6. Buat alert quota 50%, 75%, 90%, credential error, dan dead job.
7. Mulai dengan satu worker dan concurrency rendah.
8. Naikkan worker berdasarkan queue lag, bukan berdasarkan traffic API.
