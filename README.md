# API Kurir

Dokumentasi fondasi layanan proxy ekspedisi untuk Emisell. Layanan ini
menyediakan kontrak API yang familier seperti RajaOngkir, tetapi menggunakan
master lokasi, tarif, aturan berat, dan histori tracking milik sendiri.

## Tujuan

- Memisahkan aturan ekspedisi dari aplikasi utama Emisell.
- Melayani cek ongkir dari database lokal sebanyak mungkin.
- Menggunakan provider eksternal hanya ketika tarif belum tersedia atau perlu
  diverifikasi ulang.
- Menyatukan respons tracking dari beberapa ekspedisi.
- Mengurangi ketergantungan terhadap satu aggregator.
- Mendukung penambahan ekspedisi melalui adapter tanpa mengubah kontrak API
  publik.

## Batas tanggung jawab

Emisell bertanggung jawab menghitung berat aktual total barang dan dimensi
paket. API Kurir bertanggung jawab menghitung:

- berat volumetrik;
- chargeable weight;
- pembulatan berat;
- minimum berat layanan;
- tarif bertingkat;
- surcharge;
- total ongkir;
- normalisasi status tracking.

API Kurir tidak boleh hanya mengembalikan `price_per_kg`, karena beberapa
layanan memakai minimum berat, harga berat pertama, tier, atau divisor
volumetrik yang berbeda.

## Implementasi saat ini

Fondasi versi `0.9.0` sudah mencakup:

- API Go 1.26 dan Echo v5;
- migration PostgreSQL untuk lokasi, kurir, layanan, rate card, rule, snapshot,
  dan quota ledger;
- endpoint health, pencarian lokasi, daftar kurir, dan cek ongkir lokal;
- mesin tarif `flat`, `per_kg`, `base_plus_increment`, dan
  `minimum_then_per_kg`;
- minimum berat, volumetrik, pembulatan `ceil`, `floor`, dan threshold;
- profile JNE JTR dengan minimum 10 kg dan boundary 300 gram;
- request coalescing dalam satu instance menggunakan `singleflight`;
- memory cache dan PostgreSQL advisory lock untuk fase MVP;
- adapter Redis yang baru aktif jika `REDIS_ENABLED=true`;
- adapter RajaOngkir V2 dengan timeout, quota ledger, dan exact quote snapshot;
- fallback otomatis saat rate card lokal belum tersedia;
- katalog canonical dan pengelompokan layanan otomatis untuk 12 kurir aktif;
- penyimpanan alias kode mentah provider tanpa input mapping manual, termasuk
  varian JNE seperti `REG23`, `CTCYES`, `CTCSPS`, dan `JTR>130`;
- import master wilayah lokal Kemendagri sampai kelurahan/desa dan kode pos;
- kode pos many-to-many dengan provenance dataset, versi/checksum sumber, dan
  validasi lima digit;
- dashboard operasional Vite untuk snapshot tarif, mapping otomatis, kuota,
  dan lifecycle credential provider;
- menu Cek Ongkir dengan pencarian wilayah lokal, pilihan ekspedisi, serta
  hasil harga/ETD/berat tagihan/sumber tarif;
- menu Cek Resi dengan pilihan ekspedisi, status
  antrean refresh, ringkasan, dan timeline perjalanan;
- menu Daftar Ekspedisi dengan pencarian, filter kemampuan, katalog domestik,
  internasional, tracking, serta layanan lokal yang sudah terdaftar;
- tambah, validasi, masking, enkripsi, rotasi, dan nonaktifkan key RajaOngkir
  dari dashboard tanpa restart API atau memasukkan provider key ke environment;
- menu dokumentasi API dengan daftar endpoint, contoh request/response, Postman
  Collection, dan Postman Environment tanpa credential;
- credential admin terpisah dari API key customer;
- generate, daftar, dan revoke customer API key dari dashboard; secret acak
  256-bit hanya tampil sekali, database hanya menyimpan hash, dan key aktif
  sampai di-revoke tanpa nama atau masa berlaku;
- versioning rate card dan audit log perubahan admin;
- registrasi tracking terdeduplikasi dengan resi terenkripsi AES-256-GCM;
- durable tracking job menggunakan PostgreSQL `SKIP LOCKED`;
- concurrency internal worker yang dapat diatur `1–64` consumer;
- adapter tracking resmi RajaOngkir untuk delapan kurir yang capability AWB-nya
  tercantum aktif pada dokumentasi provider;
- normalisasi status, jadwal refresh adaptif, quota ledger bersama, retry
  terkontrol, dan audit panggilan provider;
- OpenAPI 3.1 di `openapi/public.yaml`.

Adapter tidak melakukan hit jika belum ada credential provider dan tidak pernah
melakukan scraping.
Tarif pada `testdata/e2e_seed.sql` hanya fixture pengujian dan tidak boleh
menjadi tarif produksi.

## Menjalankan lokal

Kebutuhan: Go 1.26+, Node.js 24+, Docker, dan Docker Compose.

```bash
cp .env.example .env
make db-up
make migrate
make run-api
```

Untuk menjalankan seluruh aplikasi di Docker dari root proyek:

```bash
docker compose up -d --build
```

`docker-compose.yml` membaca `.env` dari root proyek. File
`deployments/compose/compose.yaml` tetap tersedia sebagai wrapper kompatibilitas
untuk perintah lama. Secret tidak disalin ke file Compose atau source code.
Dashboard tersedia di `http://localhost:5173`.
Dashboard memakai same-origin API: browser selalu meminta `/v1/*` relatif ke
domain dashboard. Vite meneruskannya ke `127.0.0.1:8080` saat development,
sedangkan Nginx meneruskannya ke service `api:8080` saat Docker/production.
Karena itu deployment cukup memakai satu domain publik dan bundle production
tidak bergantung pada alamat `localhost:8080`.

Port dashboard dan API secara default bind ke seluruh interface. Stack juga
dapat diuji dari perangkat lain menggunakan
`http://<IP-SERVER>:5173`. Gunakan `DASHBOARD_BIND_ADDR` atau
`HTTP_BIND_ADDR` bila port hanya boleh tersedia pada interface tertentu.
Biarkan `VITE_API_URL` kosong untuk deployment satu domain; isi hanya jika API
memang berada pada origin yang berbeda.

API berjalan di `http://localhost:8080`. PostgreSQL development menggunakan
port `55432` agar tidak mudah bentrok dengan instalasi PostgreSQL lain.

Dashboard:

```bash
make dashboard-install
make dashboard-dev
```

Gunakan operator `emisell` dan admin API key `dev-emisell` pada development.
Production wajib mempunyai
`ADMIN_API_KEYS` sendiri dan tidak boleh menggunakan key customer.

Master wilayah diimpor dari snapshot Kemendagri 2025 yang dipin ke commit dan
checksum tertentu. Dataset berisi provinsi sampai desa/kelurahan serta mapping
kode pos:

```bash
make region-import
```

Import tidak memakai hit RajaOngkir. Pencarian customer selalu membaca
PostgreSQL lokal. ID lokasi RajaOngkir dibuat otomatis secara lazy ketika
sebuah lokasi pertama kali digunakan untuk cek ongkir.

Redis tidak dijalankan oleh `make db-up`. Saat diperlukan:

```bash
make redis-up
```

### Mengaktifkan RajaOngkir

Provider key tidak perlu dimasukkan ke `.env`. Buka dashboard, pilih
`Kuota provider`, tekan `Tambah key`, lalu tempel key RajaOngkir. Sistem akan:

1. memvalidasi key langsung ke endpoint resmi RajaOngkir;
2. mencatat satu hit validasi pada ledger hari berjalan;
3. mengenkripsi secret dengan AES-256-GCM;
4. menyimpan hanya ciphertext, fingerprint, dan tampilan termasking;
5. langsung memasukkan key ke rotasi tarif dan tracking tanpa restart.

Jika beberapa key aktif, resolver memilih key yang masih berkuota dengan rasio
pemakaian terendah. Key yang telah mencapai limit tidak dipakai lagi sampai
reset kuota Jakarta. Satu key dapat dinonaktifkan dari dashboard tanpa
mengganggu key lain.

Yang tetap harus tersedia di secret manager/environment adalah master key
enkripsi aplikasi, bukan provider key:

```text
PROVIDER_CREDENTIAL_ENCRYPTION_KEY=<base64-32-byte-random-key>
```

Nilai development bawaan hanya untuk lokal. Production wajib memakai nilai
acak yang stabil dan dibackup; kehilangan atau mengganti master key tanpa
proses rotasi membuat credential database tidak dapat didekripsi.

`RAJAONGKIR_API_KEY` masih dikenali sebagai fallback instalasi lama, tetapi
harus dibiarkan kosong pada instalasi baru.

`location-sync` tidak lagi dijalankan sebagai service default. Full sync
RajaOngkir hanya disimpan sebagai alat pemulihan legacy di profile Compose
`legacy-provider-full-sync`.

### Menyiapkan tracking

Tracking tetap nonaktif sampai key enkripsi dan credential provider siap:

```text
TRACKING_ENABLED=true
TRACKING_ENCRYPTION_KEY=<base64-32-byte-key>
RAJAONGKIR_TRACKING_COURIERS=jne,sap,ninja,jnt,tiki,wahana,pos,lion
```

Tambahkan provider key dari dashboard sebelum menyalakan worker. Key enkripsi
tracking dapat dibuat melalui secret manager atau generator kriptografis.
Jangan menyimpannya pada source code. Request tracking dari dashboard cukup
menggunakan nomor resi dan kode ekspedisi.

## Dokumen

1. [Arsitektur sistem](docs/architecture.md)
2. [Model tarif dan aturan berat](docs/rate-and-weight-engine.md)
3. [Kontrak API](docs/api-contract.md)
4. [Katalog ekspedisi dan regulasi layanan](docs/provider-service-catalog.md)
5. [Matriks toleransi dan pembulatan berat](docs/weight-rounding-matrix.md)
6. [Operasional, kuota, dan compliance](docs/operations-and-compliance.md)
7. [Register sumber](docs/source-register.md)
8. [Stack teknologi dan concurrency](docs/technology-stack.md)
9. [Integrasi RajaOngkir V2](docs/rajaongkir-integration.md)
10. [Dashboard admin dan tracking](docs/admin-dashboard-and-tracking.md)
11. [Operasional adapter tracking RajaOngkir](docs/rajaongkir-tracking.md)

## Prinsip implementasi

1. Master lokasi lengkap, master tarif sparse.
2. Tarif diisi berdasarkan rute yang benar-benar dicari.
3. Satu resi unik hanya memiliki satu proses refresh.
4. Semua GET customer membaca PostgreSQL atau cache lokal; Redis digunakan
   setelah diaktifkan pada pre-production.
5. Webhook didahulukan dibanding polling.
6. Setiap aturan provider mempunyai sumber dan masa berlaku.
7. Aturan yang belum dikonfirmasi tidak boleh di-hardcode.

## Status dokumen

- Terakhir diverifikasi: 29 Juli 2026.
- Cakupan awal: 17 kode kurir domestik yang dicontohkan pada endpoint
  multi-courier RajaOngkir.
- Biteship tidak digunakan.
- Karrio OSS dapat digunakan sebagai internal carrier orchestrator.
- Backend utama menggunakan Go dan Echo; dashboard menggunakan Vite, React,
  dan TypeScript.
- Penggunaan dan penggabungan beberapa credential RajaOngkir hanya boleh
  dilakukan setelah mendapatkan persetujuan tertulis.
