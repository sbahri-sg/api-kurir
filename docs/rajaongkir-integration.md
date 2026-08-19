# Integrasi RajaOngkir V2

## 1. Scope

Dokumen ini khusus implementasi **RajaOngkir Shipping Cost** yang sudah ada.
Integrasi ini dipertahankan sebagai jalur legacy rate/tracking. Ini bukan pola
onboarding partner baru. Partner baru wajib menyediakan Partner Connector API
canonical dan mengelola API native serta credential downstream mereka sendiri.

Integrasi fase ini mencakup:

- cek ongkir domestik sebagai fallback rate miss;
- exact quote snapshot di PostgreSQL;
- quota ledger per credential alias;
- mapping lokasi internal ke ID RajaOngkir;
- master lokasi lokal dari dataset wilayah Kemendagri;
- timeout, request coalescing, dan distributed lock;
- normalisasi respons tanpa mengekspos payload atau API key;
- tracking AWB sinkron untuk façade SDK dan asynchronous melalui durable
  worker untuk kontrak lama.

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
| Daftar provinsi | GET | `destination/province` |
| Kota per provinsi | GET | `destination/city/{province_id}` |
| Kecamatan per kota | GET | `destination/district/{city_id}` |
| Kelurahan per kecamatan | GET | `destination/sub-district/{district_id}` |
| Tracking AWB | POST | `track/waybill` |

Autentikasi menggunakan header:

```http
key: <shipping-cost-api-key>
```

API Kurir menerima header `key` maupun Bearer dari customer. Key tersebut
adalah customer API key API Kurir, bukan provider key yang tersimpan
terenkripsi. Bentuk kontrak ditentukan oleh path: `/api/v1` selalu memakai
façade RajaOngkir V2 dan `/v1` selalu memakai kontrak canonical internal.

Façade mendukung:

- ID integer pada destination dan seluruh endpoint hierarki;
- `limit` sampai 1000 serta `offset`;
- request ongkir `application/x-www-form-urlencoded`;
- `POST calculate/domestic-cost` untuk subdistrict;
- `POST calculate/district/domestic-cost` untuk district;
- response sukses/error `meta + data` seperti RajaOngkir V2.

Dokumentasi resmi:

- [Getting Started dan endpoint](https://www.rajaongkir.com/docs/shipping-cost/getting_started/endpoint)
- [Authorization](https://rajaongkir.com/docs/shipping-cost/getting_started/apikey)
- [Calculate Domestic Cost](https://rajaongkir.com/docs/shipping-cost/endpoint-rajaongkir-for-search-base/calculate-domestic-cost)
- [Search Domestic Destination](https://rajaongkir.com/docs/shipping-cost/endpoint-rajaongkir-for-search-base/search-destination-rajaongkir)
- [Search Province](https://rajaongkir.com/docs/shipping-cost/endpoint-rajaongkir-for-form-base-calculate-cost/search_province)
- [Search City](https://rajaongkir.com/docs/shipping-cost/endpoint-rajaongkir-for-form-base-calculate-cost/search_city)
- [Search District](https://rajaongkir.com/docs/shipping-cost/endpoint-rajaongkir-for-form-base-calculate-cost/search_district)
- [Search Subdistrict](https://rajaongkir.com/docs/shipping-cost/endpoint-rajaongkir-for-form-base-calculate-cost/search_subdistrict)
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

## 4. Façade SDK RajaOngkir V2

Emisell dapat mengarahkan base URL SDK ke:

```text
https://<domain-api-kurir>/api/v1
```

Kontrak shipping-cost yang tersedia pada base path tersebut:

- header autentikasi `key` atau Bearer dengan hasil yang sama;
- `GET /destination/domestic-destination`;
- `GET /destination/province`;
- `GET /destination/city/{province_id}`;
- `GET /destination/district/{city_id}`;
- `GET /destination/sub-district/{district_id}`;
- `POST /calculate/domestic-cost`;
- `POST /calculate/district/domestic-cost`;
- `POST /track/waybill`;
- request kalkulasi `application/x-www-form-urlencoded` atau
  `multipart/form-data`; JSON ditolak dengan HTTP `415`;
- respons dan error memakai envelope `meta` dan `data`.

ID wilayah pada `/api/v1` berasal dari dump RajaOngkir milik
`region-service-main` dan harus dipakai kembali pada request berikutnya.
Namespace ID dipisahkan berdasarkan level sehingga angka yang sama dapat
digunakan oleh provinsi, kota, kecamatan, dan kelurahan tanpa bentrok.

Base path lama `/v1` tetap aktif untuk dashboard dan integrasi JSON API Kurir.
Kontrak tracking pada path tersebut tetap asynchronous agar dashboard dan
integrasi lama tidak berubah, terlepas dari header autentikasinya.

Tracking kompatibel SDK tersedia pada:

```http
POST /api/v1/track/waybill?awb=<nomor-resi>&courier=<kode-kurir>
key: <customer-api-key>
```

Parameter `last_phone_number` opsional. Nilai lima digit terakhir nomor
telepon penerima hanya perlu dikirim jika provider meminta validasi tambahan;
API Kurir tidak memaksanya pada semua ekspedisi. Respons sinkron memakai field
RajaOngkir V2 `delivered`, `summary`, `details`, `delivery_status`, dan
`manifest`.

Untuk menghemat kuota, hasil provider disimpan pada snapshot tracking yang
sama dengan worker lama. Snapshot yang masih fresh atau sudah final langsung
dikembalikan tanpa hit provider baru. Miss/stale memanggil provider satu kali
dan menyimpan hasilnya. Request bersamaan untuk kombinasi ekspedisi dan resi
yang sama digabung per proses API.

## 5. Alur rate miss

```text
POST /api/v1/calculate/domestic-cost
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

## 6. Exact quote, bukan rate card

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

## 7. Berat dan dimensi

Dokumentasi Calculate Domestic V2 saat ini mendokumentasikan form:

```text
origin
destination
weight
courier
price (opsional)
include_group (opsional, extension API Kurir)
```

`price=lowest` mengurutkan seluruh opsi termurah ke termahal, sedangkan
`price=highest` mengurutkan seluruh opsi termahal ke termurah. Parameter ini
tidak menghapus opsi layanan lain dari respons.

`include_group=true` menambahkan `canonical_service`, `service_group`, dan
`service_type` pada setiap hasil tarif. Nilai default adalah `false`, sehingga
integrasi yang tidak meminta enrichment tetap menerima kontrak 1:1 RajaOngkir
V2. Kode provider asli tetap berada pada `service`; contoh `JTR>130` tetap
ditampilkan apa adanya dan dinormalisasi menjadi `JTR` pada
`canonical_service`.

Dimensi tidak tercantum sebagai field resmi pada form tersebut. Adapter hanya
mengirim `weight` ke RajaOngkir. Jika request API Kurir menyertakan dimensi:

- dimensi tetap masuk fingerprint snapshot;
- dimensi tidak dikirim ke RajaOngkir;
- respons membawa warning;
- caller harus memastikan `weight` sudah aman sebagai berat yang akan dipakai
  provider bila aturan volumetrik belum dapat dihitung lokal.

Rate card lokal yang sudah memiliki divisor resmi tetap menghitung volumetrik
di API Kurir.

## 8. Master lokasi lokal dan mapping lazy

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

Public ID canonical internal tetap tersedia untuk menyatukan mapping lintas
provider. SDK Emisell tidak perlu mengirim kode Kemendagri; path `/api/v1`
memakai ID dump RajaOngkir, sedangkan ID Mengantar, KiriminAja, atau provider
lain disimpan pada namespace masing-masing di `provider_location_mappings`.

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

Dump `region-service-main/data/regions.db` menjadi sumber ID kompatibilitas
Emisell dan diimpor melalui `make legacy-region-import`. Full sync API
RajaOngkir tidak berjalan secara default; tool tersebut tetap tersedia melalui
profile `legacy-provider-full-sync` untuk pembaruan atau diagnosis.

## 9. Quota ledger

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

## 10. Error mapping

| Kondisi | API Kurir |
|---|---|
| Mapping lokasi belum ada | `422 PROVIDER_LOCATION_NOT_MAPPED` |
| Provider tidak memiliki tarif | `422 RATE_NOT_AVAILABLE` |
| Local daily quota habis | `503 PROVIDER_QUOTA_EXHAUSTED` |
| Upstream HTTP 429 | retry sementara dengan exponential backoff |
| Provider menolak credential | `502 PROVIDER_AUTHENTICATION_FAILED` |
| Timeout, network, atau upstream 5xx | `502 PROVIDER_ERROR` |

Request `400`, `404`, dan `422` provider tidak di-retry. Versi awal juga tidak
melakukan automatic retry untuk timeout/5xx agar satu request customer tidak
diam-diam menghabiskan beberapa hit.

## 11. Cara menjalankan

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

Pada path `/v1`, gunakan public ID canonical hasil endpoint destination API
Kurir. Pada path `/api/v1`, gunakan ID RajaOngkir lokal hasil endpoint yang
sama; jangan mencampurkan ID dari provider atau level lain.

## 12. Tracking

Façade `/api/v1/track/waybill` memberikan respons sinkron kompatibel
RajaOngkir V2 dari snapshot atau provider. Kontrak lama `/v1/track/waybill`
tetap asynchronous: API mendaftarkan resi terenkripsi, sedangkan worker
melakukan hit provider satu kali untuk shipment yang due. Konfigurasi dan
kebijakan polling dijelaskan di
[Operasional adapter tracking RajaOngkir](rajaongkir-tracking.md).
