# Stack Teknologi dan Concurrency

## 1. Keputusan utama

Stack yang dipilih untuk fondasi API Kurir:

| Area | Pilihan | Fungsi |
|---|---|---|
| Public dan admin API | Go + Echo | HTTP API berlatensi rendah |
| Dashboard | Vite + React + TypeScript | Pengelolaan provider, tarif, aturan, dan operasi |
| Database | PostgreSQL | Source of truth, rate card, tracking, audit, dan job |
| Akses database | pgx + sqlc | Pool koneksi dan query SQL bertipe |
| Cache/koordinasi MVP | Memory + PostgreSQL | Cache lokal, quota ledger, dan advisory lock |
| Cache/koordinasi produksi | Redis, disiapkan opsional | Cache, rate limit, lock, dan coalescing lintas instance |
| Durable job | River | Queue berbasis PostgreSQL untuk refresh rate/tracking |
| Carrier orchestrator | Karrio OSS, opsional | Integrasi carrier internal dan normalisasi |
| API specification | OpenAPI 3.1 | Kontrak publik RajaOngkir-like dan client generation |
| Observability | OpenTelemetry + Prometheus + Grafana | Trace, metric, dashboard, dan alert |
| Packaging | Docker | Artefak deployment yang konsisten |

Vite bukan backend framework. Vite dipakai sebagai build tool dashboard;
aplikasi dashboard menggunakan React dan TypeScript.

## 2. Alasan memilih Go dan Echo

Go cocok untuk workload API Kurir karena sebagian besar waktunya dipakai untuk:

- menunggu PostgreSQL, Redis bila aktif, atau API provider;
- memanggil API provider;
- memproses banyak request independen;
- menjalankan worker terjadwal;
- menjaga penggunaan memory tetap dapat diprediksi.

Go runtime menjadwalkan goroutine ke beberapa OS thread. Karena itu service
tidak perlu membuat dan mengelola thread manual untuk setiap request. Satu
instance dapat menangani banyak koneksi secara bersamaan, lalu kapasitas
ditambah secara horizontal dengan beberapa instance.

Echo dipilih karena:

- menggunakan ekosistem standar `net/http`;
- routing dan middleware sederhana;
- mudah diintegrasikan dengan OpenAPI, tracing, dan profiling;
- performa router sudah lebih dari cukup karena bottleneck utama sistem ini
  adalah provider eksternal, database, cache, dan pengendalian kuota.

Fiber tetap layak jika tim sangat mengutamakan gaya API seperti Express.
Namun, fondasi ini menetapkan Echo agar kompatibilitas dengan library
`net/http` lebih luas dan biaya integrasi lebih rendah.

## 3. Model concurrency

### 3.1 Prinsip

Concurrency harus terbatas dan dapat dikendalikan. Jangan membuat goroutine
tanpa batas hanya karena Go mampu melayani banyak request.

Gunakan empat lapisan:

1. load balancer membagi request ke beberapa instance API;
2. setiap instance memakai goroutine untuk melayani request secara paralel;
3. semaphore/rate limiter membatasi concurrency per provider dan credential;
4. queue worker mengerjakan refresh yang tidak perlu ditunggu customer.

Contoh policy awal, bukan angka permanen:

```text
API request in-flight per instance : dibatasi oleh server dan load test
RajaOngkir rate calls               : 20 concurrent per credential
RajaOngkir tracking calls           : 10 concurrent per credential
Direct carrier                      : mengikuti kontrak masing-masing
Database connections per API        : 20-40
Database connections per worker     : 10-20
```

Angka final harus ditentukan dari limit resmi provider, jumlah instance,
kapasitas database, dan hasil load test. Pool koneksi dihitung secara global:

```text
total_connection =
  api_instance * api_pool
  + worker_instance * worker_pool
  + karrio_pool
  + operational_reserve
```

Nilainya tidak boleh melewati `max_connections` PostgreSQL. Gunakan PgBouncer
jika jumlah instance mulai besar atau koneksi sangat fluktuatif.

### 3.2 Timeout dan cancellation

Setiap request membawa context dari HTTP handler hingga database/provider.
Jika customer membatalkan request atau deadline habis, pekerjaan synchronous
ikut dibatalkan.

Budget awal:

| Operasi | Deadline |
|---|---:|
| Redis, bila aktif | 50–100 ms |
| Query lokal PostgreSQL | 100–300 ms |
| Provider rate | 2–4 detik |
| Provider tracking | 3–5 detik |
| Total synchronous request | maksimum 5 detik |

Deadline provider harus lebih pendek dari total deadline agar masih ada waktu
untuk fallback dan membentuk respons.

## 4. Request coalescing

Jika 1.000 customer mencari rute dan service yang sama ketika cache kosong,
sistem tidak boleh membuat 1.000 hit RajaOngkir.

Gunakan tingkatan sesuai fase:

1. `singleflight` di dalam satu proses Go;
2. PostgreSQL advisory lock untuk koordinasi MVP;
3. distributed lock Redis ketika mulai memakai beberapa instance produksi.

Identitas rate request harus memuat semua input yang mengubah hasil:

```text
rate:v1:
  origin_location_id:
  destination_location_id:
  courier_code:
  service_code:
  chargeable_weight_bucket:
  dimension_bucket:
  item_value_bucket:
  rate_version
```

Alur:

```text
request
  -> read memory cache/Redis bila aktif
  -> read active rate card PostgreSQL
  -> acquire local singleflight
  -> acquire PostgreSQL/Redis lock
  -> call provider satu kali
  -> persist snapshot/rate card
  -> populate cache aktif
  -> release lock
  -> fan-out hasil ke request yang menunggu
```

Request yang gagal mendapatkan lock:

- membaca snapshot terbaru jika masih aman;
- menunggu singkat dengan jitter lalu membaca cache kembali; atau
- menerima respons `refresh_pending` bila tidak ada data lokal.

Lock harus memiliki TTL dan token kepemilikan. Proses hanya boleh melepas lock
yang tokennya cocok agar lock baru milik proses lain tidak terhapus.

## 5. Worker dan queue

Gunakan proses worker terpisah dari API. Dengan demikian lonjakan tracking
tidak mengambil semua CPU, koneksi database, atau slot provider milik request
checkout.

Queue minimum:

| Queue | Prioritas | Contoh pekerjaan |
|---|---:|---|
| `rate_urgent` | tertinggi | Rate miss pada checkout |
| `tracking_urgent` | tinggi | Out for delivery dan permintaan manual sah |
| `tracking_normal` | normal | Refresh resi in transit |
| `rate_refresh` | rendah | Revalidasi rate card yang mendekati expired |
| `maintenance` | terendah | Cleanup, reconciliation, dan canary |

River dipilih karena job tersimpan secara durable di PostgreSQL dan mendukung
transaksi dengan perubahan data domain. Redis tidak menjadi syarat queue.
Ketika sudah diaktifkan, Redis digunakan untuk cache, lock, counter berumur
pendek, dan rate limiter lintas instance.

Syarat setiap job:

- idempotent;
- memiliki unique key;
- memiliki timeout;
- retry hanya pada error yang memang retryable;
- exponential backoff dengan jitter;
- dead-letter/manual review setelah batas percobaan;
- mencatat provider, credential alias, durasi, dan quota cost.

Unique key contoh:

```text
rate:{origin}:{destination}:{courier}:{service}:{weight_bucket}
tracking:{courier}:{normalized_waybill}
```

## 6. Karrio dan adapter langsung

Karrio berjalan sebagai service internal opsional. Public request tidak
langsung masuk ke Karrio.

```text
Emisell
  -> API Kurir (Go/Echo)
      -> local rate/tracking
      -> direct Go adapter
      -> Karrio internal adapter
      -> authorized aggregator
```

Policy:

- gunakan direct Go adapter untuk provider bertrafik tinggi atau kontrak yang
  membutuhkan kontrol khusus;
- gunakan Karrio untuk mempercepat onboarding carrier yang adaptornya cocok;
- normalisasi akhir tetap dilakukan oleh domain API Kurir;
- limit, credential, cache, tenant, dan audit tetap dikelola API Kurir;
- outage Karrio tidak boleh memutus hasil lokal yang masih valid.

Karrio tidak menambah quota RajaOngkir dan tidak mengubah ketentuan provider.

## 7. Dashboard

Stack dashboard:

- Vite;
- React;
- TypeScript;
- TanStack Router;
- TanStack Query;
- TanStack Table;
- React Hook Form;
- Zod;
- Tailwind CSS;
- shadcn/ui.

Dashboard hanya memanggil Admin API. Dashboard tidak boleh terhubung langsung
ke PostgreSQL, Redis, Karrio, atau provider.

Modul dashboard:

1. provider dan credential alias;
2. service serta capability;
3. master wilayah lokal dan status provider mapping otomatis;
4. snapshot tarif hasil provider;
5. aturan berat/minimum/tier/surcharge yang terverifikasi;
6. pencarian serta refresh tracking;
7. quota ledger dan health provider;
8. queue, retry, dan dead-letter;
9. audit log dan approval;
10. dashboard latency, cache hit, error, serta mismatch canary.

Dashboard fase ini read-only untuk tarif dan mapping. Tidak ada publish rate
card, edit aturan pembulatan, atau upsert mapping manual. Jika operasi berisiko
tersebut kelak dibutuhkan untuk konfigurasi internal, ia harus dipisahkan dari
alur customer serta dilindungi RBAC, approval, dan audit log.

## 8. Struktur repository

Struktur monorepo yang direkomendasikan:

```text
api-kurir/
  apps/
    api/                 # public dan admin API Go/Echo
    worker/              # River workers
    dashboard/           # Vite + React + TypeScript
    karrio/              # config/deployment Karrio, bila digunakan
  internal/
    auth/
    locations/
    rates/
    tracking/
    quota/
    providers/
      rajaongkir/
      kiriminaja/
      jne/
      karrio/
  migrations/
  openapi/
    public.yaml
    admin.yaml
  deployments/
    compose/
  docs/
```

`apps/api` dan `apps/worker` boleh menggunakan package domain yang sama di
`internal`, tetapi menghasilkan binary dan deployment terpisah.

## 9. Deployment awal

Fondasi awal:

```text
Load balancer / reverse proxy
  -> api-1
  -> api-2

worker-rate-1
worker-tracking-1
karrio-1 (opsional)
PostgreSQL
Redis
Object storage / backup
```

Pada MVP satu instance, baris Redis di atas bersifat opsional. Development
default menjalankan PostgreSQL saja. Redis diaktifkan pada pre-production
sebelum API diskalakan menjadi beberapa instance dan sebelum traffic besar
masuk.

Docker Compose cukup untuk development dan staging. Untuk produksi awal,
gunakan VM/container service dengan health check, restart policy, secret
manager, backup, dan rolling deployment. Kubernetes belum diperlukan sebelum
ada kebutuhan autoscaling, multi-node orchestration, atau standar platform
internal yang mewajibkannya.

Scaling dilakukan per fungsi:

- tambah instance API bila request concurrency/CPU meningkat;
- tambah worker tracking bila queue tracking tertinggal;
- tambah worker rate bila rate miss meningkat;
- jangan menaikkan worker provider melewati limit provider;
- optimalkan cache hit dan coalescing sebelum menambah instance.

## 10. Target performa

Target awal:

| Jalur | Target p95 |
|---|---:|
| Memory/Redis cache hit | <50 ms |
| Kalkulasi dari rate card lokal | <150 ms |
| Tracking dari snapshot lokal | <200 ms |
| Rate miss ke provider | <3 detik bila provider sehat |

Target tracking lokal dapat diturunkan menuju 100 ms setelah profiling.
Latency provider tidak dijadikan SLA API lokal; response membawa
`data_freshness`, `is_stale`, atau `refresh_pending` bila diperlukan.

Load test minimum:

- campuran 80% cache hit dan 20% local database;
- burst route yang sama untuk menguji coalescing;
- rate miss terkontrol untuk menguji limiter;
- tracking snapshot bersamaan;
- provider timeout/429/5xx;
- Redis unavailable, khusus environment yang sudah mengaktifkannya;
- queue backlog;
- satu instance API dimatikan saat traffic berjalan.

Kriteria lulus tidak hanya latency. Pastikan error rate, jumlah hit provider,
jumlah koneksi database, memory, queue lag, dan cache hit ratio tetap dalam
batas.

## 11. Konfigurasi yang wajib tersedia

Konfigurasi per environment:

```text
HTTP read/write/idle timeout
maximum request body
database pool min/max/lifetime
Redis enabled, timeout, dan pool
provider timeout
provider concurrency
provider requests per second
provider daily quota
retry count dan backoff
cache TTL dan stale window
worker count per queue
graceful shutdown deadline
```

Secret tidak masuk ke repository atau environment file yang dikomit. Simpan
secret provider dan database pada secret manager.

## 12. Urutan implementasi

1. Buat OpenAPI public dan admin.
2. Siapkan Go/Echo, PostgreSQL, pgx/sqlc, migration, tracing, serta interface
   cache/lock dengan Redis default nonaktif.
3. Implementasikan location, rate card, dan calculation engine lokal.
4. Implementasikan adapter RajaOngkir dengan quota ledger dan coalescing.
5. Pisahkan worker River untuk rate refresh dan tracking.
6. Bangun dashboard Vite/React untuk provider, rate card, rules, dan operasi.
7. Implementasikan Partner Connector client, Provider Account API, dan
   contract test canonical.
8. Load test, chaos test partner, dan drill quota exhaustion.
9. Onboard partner berdasarkan volume penggunaan tertinggi; partner menangani
   API native dan mapping internalnya sendiri.

Fondasi dianggap siap produksi setelah gate pada dokumen
[Operasional, Kuota, dan Kepatuhan](operations-and-compliance.md) terpenuhi.
