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

Emisell bertanggung jawab menghitung berat final paket, termasuk dimensi dan
berat volumetrik. API Kurir menerima field `weight` yang sudah siap digunakan
untuk cek ongkir dan bertanggung jawab menerapkan:

- minimum berat penerimaan layanan;
- maksimum berat penerimaan layanan;
- pembulatan berat;
- minimum berat tagihan;
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
- filter kelayakan layanan berdasarkan minimum diterima, minimum tagihan, dan
  maksimum berat sehingga cargo tidak tampil untuk paket ringan;
- request coalescing dalam satu instance menggunakan `singleflight`;
- memory cache dan PostgreSQL advisory lock untuk fase MVP;
- adapter Redis yang baru aktif jika `REDIS_ENABLED=true`;
- adapter RajaOngkir V2 dengan timeout, quota ledger, dan exact quote snapshot;
- fallback otomatis saat rate card lokal belum tersedia;
- katalog canonical dan pengelompokan layanan otomatis untuk 12 kurir aktif;
- penyimpanan alias kode mentah provider tanpa input mapping manual, termasuk
  varian JNE seperti `REG23`, `CTCYES`, `CTCSPS`, dan `JTR>130`;
- import master wilayah lokal Kemendagri sampai kelurahan/desa dan kode pos;
- compatibility facade region-service Emisell yang mempertahankan ID numerik
  dump RajaOngkir tanpa mencampurnya dengan kode Kemendagri;
- kode pos many-to-many dengan provenance dataset, versi/checksum sumber, dan
  validasi lima digit;
- dashboard operasional Vite untuk snapshot tarif, mapping otomatis, kuota,
  dan lifecycle credential provider;
- menu Cek Ongkir dengan pencarian wilayah lokal, pilihan ekspedisi, serta
  hasil harga/ETD/berat tagihan/sumber tarif;
- menu Cek Resi dengan pilihan ekspedisi, status
  antrean refresh, ringkasan, dan timeline perjalanan;
- menu Ekspedisi & Service dengan pencarian, filter kemampuan, katalog domestik,
  internasional, tracking, serta layanan lokal yang sudah terdaftar;
- tambah, validasi, masking, enkripsi, rotasi, dan nonaktifkan key RajaOngkir
  dari dashboard tanpa restart API atau memasukkan provider key ke environment;
- credential RajaOngkir BYOK terisolasi per merchant Emisell, tenant context
  JWT Ed25519, snapshot tarif tenant-aware, dan ledger kuota per seller;
- menu dokumentasi API dengan daftar endpoint, contoh request/response, Postman
  Collection, dan Postman Environment tanpa credential;
- credential admin terpisah dari API key customer;
- generate, daftar, dan revoke customer API key dari dashboard; secret acak
  256-bit hanya tampil sekali, database hanya menyimpan hash, dan key aktif
  sampai di-revoke tanpa nama atau masa berlaku;
- versioning rate card dan audit log perubahan admin;
- registrasi tracking terdeduplikasi dengan resi plaintext untuk backend Emisell;
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

Komunikasi Main Service Emisell memakai salah satu `API_KEYS` khusus backend
bersama header `X-Emisell-Merchant-ID`. API key customer yang dibuat dari
dashboard tidak dapat mengakses endpoint merchant gateway.

Master wilayah diimpor dari snapshot Kemendagri 2025 yang dipin ke commit dan
checksum tertentu. Dataset berisi provinsi sampai desa/kelurahan serta mapping
kode pos. Untuk development dengan Go lokal:

```bash
make region-import
```

Untuk deployment Docker atau server baru, jalankan bootstrap satu kali setelah
stack database tersedia:

```bash
make region-import-docker
```

Target Docker tersebut membangun app `region-import`, menunggu migrasi selesai,
mengimpor dataset ke volume PostgreSQL yang aktif, lalu menghapus container
bootstrap. Import bersifat idempotent dan dapat dijalankan ulang setelah
deployment.

Import tidak memakai hit RajaOngkir. Pencarian customer selalu membaca
PostgreSQL lokal. ID RajaOngkir untuk Emisell lama diisi dari dump
region-service; mapping provider lain dapat ditambahkan pada lokasi internal
yang sama tanpa mengubah ID yang tersimpan di Emisell.

### Mengimpor ID lama region-service Emisell

Emisell lama memakai ID numerik dari snapshot RajaOngkir yang tersimpan pada
`region-service-main/data/regions.db`. ID tersebut mempunyai namespace berbeda
dari kode Kemendagri; sebagai contoh ID provinsi lama `32` berarti Maluku
Utara, sedangkan kode Kemendagri `32` berarti Jawa Barat.

Import snapshot satu kali setelah migrasi dan import master wilayah selesai:

```bash
make legacy-region-import \
  REGION_DB=/absolute/path/region-service-main/data/regions.db
```

Pada server yang menjalankan Docker:

```bash
make legacy-region-import-docker \
  REGION_DB=/absolute/path/region-service-main/data/regions.db
```

Import bersifat idempotent. ID provider disimpan sebagai string pada
`provider_location_mappings` dengan kombinasi unik provider, level, dan ID.
Karena itu ID provinsi `32`, kota `32`, dan kecamatan `32` dapat hidup
bersamaan tanpa bentrok. Provider lain seperti Mengantar menyimpan ID-nya pada
namespace provider masing-masing dan tetap diarahkan ke lokasi internal yang
sama.

Endpoint kompatibilitas berikut tersedia untuk Emisell lama:

```text
GET /regions/provinces
GET /regions/provinces/{id}
GET /regions/cities?provinceId={id}
GET /regions/cities/{id}
GET /regions/districts?cityId={id}
GET /regions/districts/{id}
GET /regions/subdistricts?districtId={id}
GET /shipping/domestic-cost
```

Semua endpoint tersebut memakai API key customer melalui header `key` atau
`Authorization: Bearer`. Pada route `/regions` dan `/shipping`, angka polos
selalu ditafsirkan sebagai ID RajaOngkir lama. Endpoint canonical
`/v1/destination/*` tetap memakai `loc_idn_*` dan tidak berubah.

Kontrak tidak bergantung pada bentuk header autentikasi. Path `/api/v1`
selalu memakai ID dan respons RajaOngkir V2, sedangkan `/v1` selalu memakai
public ID canonical internal. Pencarian pada kedua path menerima alamat mentah:
awalan seperti `Jl.`, `Kec.`, dan `Kel.`, tanda baca, nomor bangunan, serta
kode pos dinormalisasi dan hasil diranking berdasarkan token wilayah yang
cocok.

Redis tidak dijalankan oleh `make db-up`. Saat diperlukan:

```bash
make redis-up
```

### Mengaktifkan RajaOngkir

Provider key tidak perlu dimasukkan ke `.env`. Buka dashboard, pilih
`Integrasi Provider > Credential & Kuota`, tekan `Tambah key`, lalu tempel key
RajaOngkir. Sistem akan:

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

Fulfillment (buat shipment, pickup, label, dan pembatalan) memakai key Shipping
Delivery yang berbeda dari key cek ongkir/tracking. Untuk fallback legacy,
server mengenali `RAJAONGKIR_DELIVERY_API_KEY` dan
`RAJAONGKIR_DELIVERY_BASE_URL`; endpoint validasi sandbox dapat dioverride
melalui `RAJAONGKIR_DELIVERY_SANDBOX_BASE_URL`. Instalasi baru sebaiknya menyimpan
`shipping_api_key` dan `delivery_api_key` terenkripsi melalui credential
RajaOngkir di dashboard; key tidak dikirim oleh Main Service pada setiap
request. Kedua jenis key diuji ke endpoint read-only RajaOngkir sebelum
disimpan. Quote, shipment, dan pickup memilih serta mengunci environment
credential secara otomatis tanpa header mode dari Main Service.

`location-sync` tidak lagi dijalankan sebagai service default. Full sync
RajaOngkir hanya disimpan sebagai alat pemulihan legacy di profile Compose
`legacy-provider-full-sync`.

### Menyiapkan tracking

Tracking tetap nonaktif sampai key konteks privat dan credential provider siap:

```text
TRACKING_ENABLED=true
TRACKING_ENCRYPTION_KEY=<base64-32-byte-key>
RAJAONGKIR_TRACKING_COURIERS=jne,sap,ninja,jnt,tiki,wahana,pos,lion
```

Tambahkan provider key dari dashboard sebelum menyalakan worker. Nomor resi
disimpan sebagai plaintext agar dapat dipakai backend Emisell dan webhook.
`TRACKING_ENCRYPTION_KEY` hanya melindungi konteks provider opsional seperti
digit telepon penerima dan dipakai untuk migrasi resi legacy. Jangan menyimpan
key tersebut pada source code.

## Dokumen

Mulai dari [Pusat Dokumentasi API Kurir](docs/README.md). Dokumen tersebut
memisahkan kontrak RajaOngkir V2, Emisell Legacy, Canonical/Internal, Admin,
dan Partner API berdasarkan pemanggil serta base path.

Dokumentasi utama:

1. [Kontrak API publik](docs/api-contract.md)
2. [Peta kontrak dan arah komunikasi](docs/api-surface-map.md)
3. [Integrasi RajaOngkir V2](docs/rajaongkir-integration.md)
4. [Katalog ekspedisi dan regulasi layanan](docs/provider-service-catalog.md)
5. [Partner Integration Contract v1](docs/partner-api-v1.md)
6. [Keamanan dan request signing](docs/security-and-signing.md)
7. [Sertifikasi partner](docs/partner-certification.md)
8. [Partner Portal dan publikasi extension](docs/partner-portal.md)
9. [Partner Integration Package dan Review Console](docs/partner-integration-packages.md)

OpenAPI partner contract-first tersedia di
[`openapi/partner-v1.yaml`](openapi/partner-v1.yaml). Kontrak partner belum
berarti endpoint booking/vendor telah aktif di production.

Vendor mengunggah ZIP melalui `/partner` menggunakan access key yang terikat ke
satu provider; provider tidak dipilih dari form. Console internal pada menu
**Partner Packages** hanya dipakai staff untuk review status. Package tetap
hanya artefak review dan source code partner tidak pernah dijalankan oleh API
Kurir.

OpenAPI untuk aktivasi dan lifecycle akun provider seller tersedia di
[`openapi/provider-account-v1.yaml`](openapi/provider-account-v1.yaml).
Spesifikasi ini juga masih contract-first dan belum menyatakan route runtime
telah aktif.

Onboarding partner baru memakai connector canonical yang disediakan dan
dioperasikan partner. API Kurir tidak membuat adapter native per vendor.
Integrasi RajaOngkir Shipping Cost yang sudah ada dipertahankan sebagai
integrasi legacy untuk rate/tracking dan bukan pola onboarding Partner API.

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
