# Arsitektur API Kurir

## 1. Konteks

API Kurir adalah internal shipping gateway untuk Emisell. Public contract
dibuat menyerupai pola RajaOngkir agar mudah diadopsi, tetapi implementasi
internal tidak bergantung pada satu provider.

```text
Emisell
   |
   v
API Gateway / Authentication
   |
   v
Shipping Domain Service
   |-- Location Service
   |-- Rate Engine
   |-- Tracking Service
   |-- Provider Router
   |
   +-- Redis (opsional sampai pre-production)
   +-- PostgreSQL
   +-- Queue / Worker
   |
   +-- Legacy Provider Client (rate/tracking yang sudah berjalan)
   |
   +-- Partner Connector Client
          |-- Partner-owned canonical API
```

## 2. Pembagian tanggung jawab

### Emisell

- Menjumlah berat aktual item dan kemasan tanpa melakukan pembulatan provider.
- Mengirim panjang, lebar, dan tinggi paket.
- Mengirim nilai barang untuk asuransi.
- Memilih origin dan destination dari master lokasi API Kurir.
- Menampilkan opsi layanan dan ongkir.

### API Kurir

- Menjaga mapping lokasi internal ke lokasi provider.
- Menentukan layanan yang tersedia pada suatu rute.
- Menghitung chargeable weight.
- Menerapkan aturan minimum dan pembulatan provider.
- Menghitung total tarif dan surcharge.
- Menyimpan snapshot serta versi rate card.
- Menormalisasi event tracking.
- Menjadwalkan refresh resi.

### Karrio

Karrio digunakan sebagai carrier integration layer internal, bukan sebagai
API publik. API Kurir tetap memiliki autentikasi, tenant, quota, cache, dan
kontrak respons sendiri.

Karrio OSS menyediakan server, SDK, tracking, dan carrier plugin. Fitur
multi-tenancy bawaan Karrio berada pada Enterprise Edition, sehingga pada
fondasi ini tenant isolation diletakkan di API Gateway/API Kurir.

Karrio tidak menjadi pola onboarding Partner API. Jika tetap digunakan untuk
integrasi legacy/direct carrier, scope-nya terpisah. Partner baru wajib
menyediakan connector canonical dan menangani API native sendiri.

## 3. Komponen

### API Gateway

- API key Emisell.
- Rate limit per tenant.
- Request ID dan idempotency key.
- Audit log.
- Proteksi payload dan schema validation.

### Location Service

Menyimpan seluruh wilayah, tetapi tidak memuat semua kombinasi tarif.

```text
locations
- id
- public_id
- parent_id
- level
- province
- city
- district
- subdistrict
- postal_code
- official_region_code
- active
```

Kolom `postal_code` dipertahankan untuk kompatibilitas. Relasi lengkap
disimpan terpisah karena satu lokasi dapat memiliki beberapa kode pos dan satu
kode pos dapat muncul pada beberapa lokasi:

```text
postal_codes
- id
- code
- active

location_postal_codes
- location_id
- postal_code_id
- provider_code
- provider_location_id
- provider_granularity
- source_type
- source_endpoint
- normalized_response_hash
- retrieved_at
- verified_at
- active
```

Mapping ID provider dipisahkan:

```text
provider_location_mappings
- id
- location_id
- provider_code
- product_code
- environment
- provider_location_id
- provider_location_name
- granularity
- source_type
- source_endpoint
- normalized_response_hash
- retrieved_at
- confidence
- verified_at
- active
```

Identitas mapping adalah
`provider_code + product_code + environment + granularity +
provider_location_id`. ID RajaOngkir dapat berbeda antara Shipping Cost dan
Shipping Delivery serta dapat berulang pada level provinsi, kota, kecamatan,
dan kelurahan. `provider_code + provider_location_id` saja tidak cukup.

Master wilayah tidak dibentuk dari sinkronisasi provider. Data resmi lokal
menyimpan `official_region_code` dan versi dataset pada
`location_dataset_imports`. Mapping provider dibuat lazy hanya untuk lokasi
yang benar-benar dipakai.

Tool legacy sinkronisasi hierarki masih dapat menyimpan checkpoint per endpoint
dan parent:

```text
provider_location_sync_checkpoints
- provider_code
- credential_alias
- endpoint_level
- parent_provider_location_id
- item_count
- response_hash
- completed_at
```

Tool tersebut berada di profile `legacy-provider-full-sync` dan tidak ikut
startup normal. HTTP 429 diperlakukan sebagai cooldown sementara; kuota harian
tetap dilindungi oleh ledger lokal per credential.

### Rate Engine

Rate Engine membaca aturan lokal. Jika rate card tidak ditemukan atau
kedaluwarsa, Provider Router membuat request upstream, menormalisasi respons,
dan menyimpan snapshot.

### Tracking Service

Tracking Service selalu mengembalikan snapshot lokal. Refresh provider
dijalankan secara asynchronous agar latensi GET customer tidak mengikuti
latensi provider.

### Provider Router

Provider Router memilih adapter berdasarkan:

- kemampuan kurir;
- status kesehatan provider;
- sisa kuota;
- freshness data;
- prioritas direct carrier API;
- policy fallback.

Urutan target:

```text
direct carrier API -> authorized aggregator -> stale local snapshot
```

Scraping website publik tidak menjadi provider produksi.

## 4. Alur cek ongkir

```text
POST /v1/calculate/domestic-cost
   |
   v
Validate location, weight, dimension
   |
   v
Find active rate card
   | yes
   +------> calculate locally -> cache -> response
   |
   | no/expired
   v
Acquire distributed lock
   |
   +-- another request refreshing -> return recent snapshot or wait briefly
   |
   v
Call provider -> normalize -> persist -> calculate -> response
```

Lock key:

```text
rate-refresh:{origin}:{destination}:{courier}
```

## 5. Alur tracking

```text
POST /v1/track/waybill
   |
   v
Read memory cache/PostgreSQL/Redis bila aktif
   |
   +-- snapshot available -> return immediately
   |
   +-- not available -> create tracking subscription
                         enqueue refresh
                         return pending/short wait
```

Nomor resi disimpan sebagai hash, masked value, dan ciphertext AES-256-GCM.
Worker mengambil durable job melalui PostgreSQL `SKIP LOCKED`; hanya adapter
carrier yang diaktifkan secara eksplisit yang boleh mendekripsi resi.

Unique tracking identity:

```text
tenant_id + courier_code + normalized_waybill
```

Jika data tracking boleh dibagi di dalam satu badan usaha, deduplication dapat
diturunkan menjadi:

```text
courier_code + normalized_waybill
```

Keputusan tersebut harus mengikuti kontrak provider dan kebijakan data.

## 6. Partner gateway

Gateway mempunyai empat permukaan dengan trust boundary berbeda:

```text
Emisell -> Public API -> API Kurir
Seller/Admin -> Provider Account API -> API Kurir
API Kurir -> Partner Connector API -> partner
Partner -> Partner Event Webhook -> API Kurir
```

Provider baru terhubung melalui satu pola:

```text
API Kurir -> Partner Integration Contract v1 -> partner
```

Façade RajaOngkir berada pada sisi northbound untuk Emisell. Kontrak partner
berada pada sisi southbound dan menggunakan model shipment canonical agar
Emisell tidak bergantung pada KiriminAja, RajaOngkir, atau provider tertentu.

Partner yang sudah mempunyai API native membuat translation layer pada
infrastrukturnya sendiri. API Kurir tidak membuat adapter per vendor, tidak
menyimpan token native vendor, dan tidak menerima webhook native carrier.

Partner mengirim perubahan AWB/status ke satu webhook API Kurir. API Kurir
menyimpan event dan outbox secara atomik, lalu meneruskannya ke Emisell secara
asynchronous.

Seller dapat mengaktifkan lebih dari satu provider account. Quote menyimpan
`provider_account_id` dan connection ID sehingga hasil partner A tidak
tertukar dengan booking partner B. Fallback provider hanya boleh terjadi
sebelum booking berhasil.

Rincian:

- [Peta kontrak](api-surface-map.md);
- [Provider Account API](provider-account-api-v1.md);
- [Partner Integration Contract](partner-api-v1.md);
- [Partner Event Webhook](partner-webhooks-v1.md);
- [Keamanan dan signing](security-and-signing.md);
- [Sertifikasi partner](partner-certification.md).

## 7. Availability

### Deployment awal

- 2 instance API Kurir berbasis Go/Echo.
- 2 instance worker Go dengan queue terpisah untuk rate dan tracking.
- 1 instance Karrio internal bila adapter Karrio sudah digunakan.
- 1 PostgreSQL, sebaiknya managed atau memiliki standby dan backup teruji.
- Redis belum wajib pada MVP satu instance; adapter dan Compose profile tetap
  tersedia.
- Aktifkan Redis dengan persistence/failover sebelum deployment multi-instance
  menerima traffic produksi besar.
- Object storage untuk backup.
- Reverse proxy/load balancer.

Detail pemilihan framework, concurrency, ukuran pool, dan struktur repository
ada di [Stack teknologi dan concurrency](technology-stack.md).

### Failure handling

- Timeout provider maksimal dan terukur per adapter.
- Circuit breaker per credential dan endpoint.
- Retry hanya untuk timeout, connection error, `429`, dan `5xx`.
- Jangan retry `400`, invalid AWB, atau unsupported route.
- Gunakan exponential backoff dengan jitter.
- Sediakan stale response dengan penanda `is_stale`.

## 8. Observability

Metric minimum:

- `api_request_total`;
- `api_request_duration_ms`;
- `rate_cache_hit_ratio`;
- `provider_request_total`;
- `provider_error_total`;
- `provider_quota_remaining`;
- `tracking_active_total`;
- `tracking_refresh_lag_seconds`;
- `rate_card_age_seconds`;
- `queue_depth`.

Setiap upstream call menyimpan:

- request ID internal;
- provider;
- credential alias, bukan secret;
- endpoint;
- status code;
- duration;
- quota cost;
- sanitized response hash.
