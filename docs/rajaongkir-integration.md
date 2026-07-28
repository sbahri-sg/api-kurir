# Integrasi RajaOngkir V2

## 1. Scope

Integrasi fase ini mencakup:

- cek ongkir domestik sebagai fallback rate miss;
- exact quote snapshot di PostgreSQL;
- quota ledger per credential alias;
- mapping lokasi internal ke ID RajaOngkir;
- master lokasi lokal dari dataset wilayah Kemendagri;
- timeout, request coalescing, dan distributed lock;
- normalisasi respons tanpa mengekspos payload atau API key.
- tracking AWB asynchronous melalui durable worker.

## 2. Kontrak resmi

Base URL:

```text
https://rajaongkir.komerce.id/api/v1/
```

Endpoint yang digunakan:

| Fungsi | Method | Path |
|---|---|---|
| Cek ongkir domestik | POST | `calculate/domestic-cost` |
| Search destination | GET | `destination/domestic-destination` |
| Tracking AWB | POST | `track/waybill` |

Autentikasi menggunakan header:

```http
key: <shipping-cost-api-key>
```

Dokumentasi resmi:

- [Getting Started dan endpoint](https://www.rajaongkir.com/docs/shipping-cost/getting_started/endpoint)
- [Authorization](https://rajaongkir.com/docs/shipping-cost/getting_started/apikey)
- [Calculate Domestic Cost](https://rajaongkir.com/docs/shipping-cost/endpoint-rajaongkir-for-search-base/calculate-domestic-cost)
- [Search Domestic Destination](https://rajaongkir.com/docs/shipping-cost/endpoint-rajaongkir-for-search-base/search-destination-rajaongkir)
- [Tracking AWB](https://www.rajaongkir.com/docs/shipping-cost/tracking)
- [Courier Availability](https://www.rajaongkir.com/docs/shipping-cost/getting_started/courier_availability)

## 3. Penyimpanan API key

API key diperlakukan seperti password:

- tidak boleh di-hardcode;
- tidak boleh masuk `.env.example`;
- hanya boleh dikirim satu kali melalui form admin ber-TLS;
- tidak boleh dicetak pada log;
- tidak boleh masuk raw request audit;
- ciphertext disimpan di PostgreSQL;
- master encryption key production menggunakan secret manager;
- rotasi key jika pernah terekspos.

Lifecycle utama:

```text
Dashboard > Kuota provider > Tambah key
  -> validasi ke endpoint resmi
  -> hit validasi masuk quota ledger
  -> AES-256-GCM encrypt
  -> simpan ciphertext + SHA-256 fingerprint + masked display
  -> langsung tersedia untuk rate dan tracking
```

Master enkripsi aplikasi:

```text
PROVIDER_CREDENTIAL_ENCRYPTION_KEY=<base64 yang decode menjadi 32 byte>
RAJAONGKIR_TRACKING_COURIERS=jne,sap,ninja,jnt,tiki,wahana,pos,lion
RAJAONGKIR_BASE_URL=https://rajaongkir.komerce.id/api/v1/
RAJAONGKIR_TIMEOUT=4s
RAJAONGKIR_SNAPSHOT_TTL=336h
RAJAONGKIR_MIN_REQUEST_INTERVAL=250ms
```

Service tetap dapat berjalan tanpa provider key. `RAJAONGKIR_API_KEY` hanya
fallback kompatibilitas untuk instalasi lama dan sebaiknya kosong pada
instalasi baru.

Setiap key database mendapat alias deterministik dari fingerprint dan limit
awal 50.000 hit/hari. Jika ada beberapa key aktif, resolver memilih rasio
`(used + reserved) / daily_limit` terendah, kemudian yang paling lama belum
dipilih. Pool yang seluruhnya habis mengembalikan quota exhausted dan tidak
jatuh ke fallback legacy.

## 4. Alur rate miss

```text
POST /v1/calculate/domestic-cost
  -> cari active local rate card
  -> bila ada, hitung lokal
  -> cari exact provider quote snapshot
  -> bila ada dan belum expired, kembalikan snapshot
  -> acquire singleflight dan PostgreSQL/Redis lock
  -> periksa snapshot sekali lagi
  -> pilih credential aktif dengan rasio quota terendah
  -> resolve ID lokasi provider
  -> mapping belum ada: cari dan validasi lokasi RajaOngkir secara lazy
  -> consume quota ledger untuk setiap upstream call
  -> panggil RajaOngkir satu kali
  -> normalisasi quote
  -> simpan snapshot
  -> kembalikan source.type=provider_quote
```

Request kedua yang identik tidak membuat hit provider selama snapshot masih
aktif.

## 5. Exact quote, bukan rate card

Respons RajaOngkir berisi total biaya untuk input tertentu. Satu quote tidak
cukup untuk membuktikan:

- tarif per kg;
- minimum berat;
- threshold pembulatan;
- tier cargo;
- surcharge;
- formula volumetrik.

Karena itu hasil provider disimpan pada `rate_snapshots` dengan fingerprint
yang mencakup:

```text
origin
destination
weight
courier set
dimensions
item value
```

Snapshot diberi:

```text
source_type=provider_quote
verification_status=observed
```

Ia tidak otomatis membuat `rate_cards` dan tidak dapat dipromosikan manual dari
dashboard. Snapshot exact-match menjadi sumber operasional selama TTL. Bila
kelak dibutuhkan master tarif per kilogram/minimum berat, pembentukannya harus
melalui proses otomatis yang memerlukan rate sheet resmi atau rangkaian probe
tervalidasi—bukan input operator.

## 6. Berat dan dimensi

Dokumentasi Calculate Domestic V2 saat ini mendokumentasikan form:

```text
origin
destination
weight
courier
price (opsional)
```

Dimensi tidak tercantum sebagai field resmi pada form tersebut. Adapter hanya
mengirim `weight` ke RajaOngkir. Jika request API Kurir menyertakan dimensi:

- dimensi tetap masuk fingerprint snapshot;
- dimensi tidak dikirim ke RajaOngkir;
- respons membawa warning;
- caller harus memastikan `weight` sudah aman sebagai berat yang akan dipakai
  provider bila aturan volumetrik belum dapat dihitung lokal.

Rate card lokal yang sudah memiliki divisor resmi tetap menghitung volumetrik
di API Kurir.

## 7. Master lokasi lokal dan mapping lazy

Customer tidak melakukan search ke RajaOngkir. Endpoint customer membaca tabel
`locations` lokal yang diisi oleh `apps/region-import`.

```bash
make region-import
```

Importer memverifikasi commit, SHA-256, jumlah record, struktur parent-child,
dan kelengkapan kode pos sebelum transaksi disimpan. Cakupan snapshot:

```text
38 provinsi
514 kabupaten/kota
7.285 kecamatan
83.762 desa/kelurahan
83.762 mapping kode pos
```

Public ID diturunkan dari kode wilayah Kemendagri dan tetap stabil walaupun
provider ongkir diganti. Provenance dataset dicatat di
`location_dataset_imports`.

Ketika cek ongkir pertama untuk suatu lokasi:

1. service memeriksa `provider_location_mappings`;
2. bila belum ada, service mencari RajaOngkir berdasarkan kode pos;
3. kandidat wajib sama pada provinsi, kabupaten/kota, kecamatan,
   desa/kelurahan, dan kode pos;
4. tepat satu kandidat boleh disimpan sebagai mapping;
5. kandidat nol atau ambigu ditolak tanpa tebakan;
6. request tarif kemudian memakai ID provider yang tersimpan.

Setiap pencarian mapping dan request tarif dicatat pada quota ledger. HTTP 429
upstream diperlakukan sebagai rate limit sementara, bukan langsung dianggap
50.000 hit harian habis. Semua request RajaOngkir dipacu dengan interval global
yang dikonfigurasi melalui `RAJAONGKIR_MIN_REQUEST_INTERVAL`.

Full sync hierarki RajaOngkir bukan lagi sumber master dan tidak berjalan
secara default. Tool lama hanya tersedia melalui profile
`legacy-provider-full-sync` untuk kebutuhan diagnosis.

## 8. Quota ledger

Sebelum upstream call, service melakukan increment atomik:

```text
provider_code
credential_alias
quota_date
daily_limit
used_count
reset_at
```

Jika `used_count + reserved_count >= daily_limit`, call dibatalkan sebelum
keluar dari sistem dan API mengembalikan:

```text
503 PROVIDER_QUOTA_EXHAUSTED
```

Tanggal quota mengikuti UTC+7. Hit validasi credential, pencarian mapping,
quote tarif, dan tracking semuanya mengurangi ledger alias terkait. Credential
hanya dicatat sebagai alias, bukan nilai API key.

## 9. Error mapping

| Kondisi | API Kurir |
|---|---|
| Mapping lokasi belum ada | `422 PROVIDER_LOCATION_NOT_MAPPED` |
| Provider tidak memiliki tarif | `422 RATE_NOT_AVAILABLE` |
| Local quota habis / upstream 429 | `503 PROVIDER_QUOTA_EXHAUSTED` |
| Provider menolak credential | `502 PROVIDER_AUTHENTICATION_FAILED` |
| Timeout, network, atau upstream 5xx | `502 PROVIDER_ERROR` |

Request `400`, `404`, dan `422` provider tidak di-retry. Versi awal juga tidak
melakukan automatic retry untuk timeout/5xx agar satu request customer tidak
diam-diam menghabiskan beberapa hit.

## 10. Cara menjalankan

Development:

```bash
cp .env.example .env
make db-up
make migrate
make location-sync SEARCH=Jakarta
make location-sync SEARCH=Bandung
make run-api
```

Pastikan `.env` berisi `RAJAONGKIR_API_KEY` yang masih aktif sebagai fallback,
atau tambahkan credential melalui menu **Kuota provider** pada dashboard.
Provider aktif otomatis ketika credential valid tersedia.

Verifikasi:

```bash
curl -X POST http://localhost:8080/v1/calculate/domestic-cost \
  -H "Authorization: Bearer dev-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "origin": "<local-origin-id>",
    "destination": "<local-destination-id>",
    "weight": 1000,
    "courier": "jne:tiki"
  }'
```

Jangan memasukkan provider ID langsung ke public request. Gunakan ID lokal
hasil endpoint destination API Kurir.

## 11. Tracking

Tracking berjalan asynchronous. API mendaftarkan resi terenkripsi, sedangkan
worker melakukan hit provider satu kali untuk shipment yang due. Konfigurasi
dan kebijakan polling dijelaskan di
[Operasional adapter tracking RajaOngkir](rajaongkir-tracking.md).
