# Partner Portal dan Publikasi Extension

Status: **rancangan produk dan operasional**

## 1. Tujuan

Partner Portal adalah ruang kerja khusus vendor kurir, aggregator logistik,
dan penyedia last-mile yang ingin terhubung ke Emisell melalui API Kurir.

Portal mempunyai tiga tujuan:

1. membantu partner menyiapkan dan menguji Partner Connector API;
2. memperlihatkan progres validasi sampai integrasi layak production;
3. mengelola informasi extension yang akan tersedia bagi seller Emisell.

Partner Portal bukan panel untuk mengoperasikan sistem internal partner.
Partner tetap mengelola tarif, armada, pickup, AWB, credential native,
mapping internal, dan proses pengiriman pada infrastrukturnya sendiri. Portal
hanya mengontrol koneksi, capability, sertifikasi, dan eksposur layanan
partner melalui API Kurir.

```text
Sistem internal partner
        |
        | connector canonical milik partner
        v
Partner Connector API <----> API Kurir <----> Emisell
                                  |
                                  v
                           Partner Portal
```

## 2. Pemisahan dashboard

Dashboard dipisahkan berdasarkan pengguna dan kewenangannya:

| Permukaan | Pengguna | Fungsi utama |
|---|---|---|
| Emisell Seller | seller | mencari, mengaktifkan, dan mengatur prioritas extension |
| Partner Portal | vendor kurir | onboarding, konfigurasi connector, pengujian, dan pengajuan publikasi |
| API Kurir Admin | operator internal | review, persetujuan, publikasi, monitoring, suspend, dan audit |

Ketiganya dapat memakai design system dan API client yang sama, tetapi harus
mempunyai route, session, role, dan authorization policy yang terpisah.
Credential admin tidak boleh berlaku pada Partner Portal, dan credential
partner tidak boleh memberikan akses ke data partner lain.

## 3. Batas kewenangan

| Aktivitas | Partner | Admin API Kurir |
|---|---:|---:|
| Mengubah profil dan kontak partner | Ya | Review |
| Mengatur URL connector sandbox/production | Ya | Validasi |
| Mengatur webhook dan melakukan rotasi credential | Ya | Audit |
| Mendeklarasikan capability dan service | Ya | Sertifikasi |
| Menjalankan contract test sandbox | Ya | Melihat bukti |
| Mengajukan review atau publikasi | Ya | Memutuskan |
| Mengaktifkan akses production | Tidak | Ya |
| Publish, unpublish, atau suspend extension | Tidak | Ya |
| Melihat data partner lain | Tidak | Sesuai role internal |
| Mengubah sistem native partner | Di sistem partner | Tidak |

Partner boleh menghentikan sementara penerimaan order baru melalui status
maintenance yang terkontrol. Tindakan tersebut tidak menghapus shipment
aktif, AWB, webhook, kewajiban pickup, atau proses rekonsiliasi.

## 4. Lifecycle pengajuan

Status onboarding dan publikasi:

```text
draft
  -> submitted
  -> technical_review
  -> sandbox_testing
  -> security_review
  -> uat
  -> approved
  -> published

published -> suspended
suspended -> technical_review | published
```

| Status | Makna |
|---|---|
| `draft` | Profil atau konfigurasi partner belum lengkap |
| `submitted` | Partner telah mengajukan integrasi untuk diperiksa |
| `technical_review` | Kontrak endpoint, autentikasi, service, dan status mapping sedang diperiksa |
| `sandbox_testing` | Contract test tarif, shipment, pickup, label, cancel, dan tracking dijalankan |
| `security_review` | Signature, replay protection, isolasi tenant, dan perlindungan data diperiksa |
| `uat` | Skenario operasional end-to-end diuji bersama |
| `approved` | Versi connector dan capability yang diuji telah disetujui |
| `published` | Extension dapat ditemukan dan diaktifkan oleh seller Emisell |
| `suspended` | Order baru dihentikan karena insiden, pelanggaran, atau hasil review |

Partner tidak dapat mengubah status menjadi `approved`, `published`, atau
`suspended`. Transisi tersebut hanya dilakukan admin API Kurir dan wajib
menghasilkan audit log.

Perubahan besar setelah publikasi, seperti major API version, autentikasi,
status mapping, perhitungan tarif, COD, atau settlement, membuat capability
terkait kembali ke `technical_review`. Versi production lama dapat tetap
melayani shipment aktif sampai proses migrasi disetujui.

## 5. Menu Partner Portal

### 5.1 Overview

Menampilkan:

- status onboarding dan persentase checklist;
- environment sandbox dan production;
- capability yang diajukan, lulus, atau ditolak;
- uptime, latency, error rate, dan webhook lag;
- tugas partner dan catatan reviewer terbaru;
- versi connector yang aktif.

### 5.2 Company Profile

Berisi identitas perusahaan, dokumen legal, kontak bisnis, kontak teknis,
kontak insiden, logo, deskripsi, wilayah layanan, dan alamat kebijakan
privasi. Dokumen sensitif hanya dapat dilihat role reviewer yang berwenang.

### 5.3 API Connection

Partner mengatur:

- base URL sandbox dan production;
- metode autentikasi canonical;
- public key atau key ID untuk request signing;
- webhook configuration;
- IP allowlist opsional;
- jadwal rotasi credential;
- status maintenance dan kontak insiden.

Portal tidak menerima token RajaOngkir, KiriminAja, carrier, atau sistem native
partner. API Kurir hanya menyimpan credential koneksi canonical yang
diterbitkan partner.

### 5.4 Capabilities

Partner mendeklarasikan fitur yang didukung, antara lain:

- rate dan coverage;
- shipment create dan cancel;
- scheduled/on-demand pickup dan dropoff;
- label dan batch label;
- tracking internal maupun external AWB;
- regular, cargo, same-day, dan instant delivery;
- COD, insurance, multi-package, balance, dan payment inquiry.

Capability baru berstatus `declared` sampai contract test dan sertifikasi
selesai. Deklarasi partner tidak otomatis membuat capability tersedia bagi
seller.

### 5.5 Services

Menampilkan katalog service yang dikirim Partner Connector API, misalnya
regular, express, cargo, trucking, same-day, atau instant. Data bersifat
read-only dari respons partner dan tidak menjadi form input tarif manual.

API Kurir menyimpan kode service mentah untuk audit serta hasil normalisasi
canonical. Partner bertanggung jawab atas mapping dari kode canonical menuju
kode internalnya.

### 5.6 Sandbox

Partner dapat menjalankan contract test dan melihat:

- test case, request ID, waktu, latency, dan hasil;
- perbedaan schema atau signature;
- idempotency dan concurrency test;
- duplicate serta out-of-order webhook;
- skenario timeout dan rekonsiliasi;
- test data yang aman tanpa data customer production.

Secret, nomor telepon, alamat lengkap, dan payload sensitif harus disamarkan.

### 5.7 Certification

Berisi checklist legal, teknis, keamanan, UAT, performance, dan operasional.
Setiap item memiliki status, catatan reviewer, bukti, waktu pengujian, serta
penanggung jawab. Partner dapat memperbaiki kekurangan dan mengajukan ulang.

### 5.8 Shipments and Logs

Partner hanya dapat melihat request yang menuju koneksi miliknya sendiri:

- shipment dan pickup reference;
- AWB termasking;
- status canonical;
- response code dan latency;
- retry, webhook, dan reconciliation result;
- correlation/request ID untuk investigasi.

Log bukan sumber untuk mengambil credential atau data pribadi mentah.
Retention mengikuti kebijakan keamanan dan perjanjian pemrosesan data.

### 5.9 Extension Listing

Partner menyiapkan materi yang akan dilihat seller:

- nama, logo, dan deskripsi extension;
- capability dan service yang telah disertifikasi;
- wilayah layanan;
- SLA dan jam operasional;
- biaya atau persyaratan komersial;
- tautan bantuan, kebijakan privasi, dan syarat layanan;
- release note dan versi connector.

Partner dapat menyimpan draft dan mengajukan publikasi. Admin API Kurir
melakukan persetujuan akhir. Hanya capability yang lulus sertifikasi yang
boleh tampil pada listing.

### 5.10 Incidents and Support

Partner dapat mengumumkan maintenance, melaporkan insiden, membaca alert, dan
berkomunikasi dengan operator API Kurir. Insiden kritis tetap mengikuti jalur
eskalasi serta SLA, bukan hanya notifikasi di portal.

## 6. Syarat publikasi

Extension hanya dapat dipublikasikan ketika:

1. profil legal dan kontak wajib telah diverifikasi;
2. connector production dapat dijangkau melalui HTTPS;
3. autentikasi, HMAC, replay protection, dan rotasi secret lulus;
4. capability yang akan ditampilkan lulus contract test;
5. webhook, retry, idempotency, timeout, dan rekonsiliasi lulus;
6. UAT dan limited production selesai sesuai risiko;
7. listing extension dan kebijakan data disetujui;
8. tidak ada finding keamanan kritis yang masih terbuka;
9. admin API Kurir memberikan persetujuan eksplisit.

Publish bukan persetujuan tanpa batas waktu. API Kurir dapat meminta
sertifikasi ulang, menonaktifkan satu capability, membatasi seller, atau
men-suspend extension tanpa mematikan partner lainnya.

## 7. Pengalaman seller setelah publish

Setelah extension berstatus `published`, seller Emisell dapat:

1. menemukan extension pada katalog integrasi;
2. melihat capability, service, SLA, dan wilayah layanan;
3. mengaktifkan koneksi melalui activation flow yang disetujui;
4. memilih service serta prioritas routing;
5. melihat kesehatan koneksi tanpa melihat credential partner;
6. menonaktifkan extension untuk order baru.

Seller tidak mengelola connector, webhook canonical, mapping native, atau
credential internal partner. Semua request tetap mengikuti jalur:

```text
Emisell -> API Kurir -> Partner Connector API -> sistem internal partner
```

Quote atau shipment harus menyimpan `provider_account_id`, `partner_id`, dan
versi connector yang digunakan. Setelah booking berhasil, API Kurir tidak
boleh memindahkan shipment tersebut ke partner lain secara otomatis.

## 8. Keamanan minimum

- autentikasi portal memakai akun personal, bukan shared API key;
- MFA wajib untuk production administrator partner;
- role minimum: `partner_owner`, `partner_developer`, `partner_support`, dan
  `partner_viewer`;
- authorization selalu diperiksa pada server berdasarkan `partner_id`;
- secret hanya tampil satu kali, dienkripsi saat disimpan, dan dapat dirotasi;
- perubahan endpoint, credential, capability, dan status menghasilkan audit;
- perubahan kritis dapat memerlukan approval dua pihak;
- log dan export menyamarkan data pribadi serta AWB;
- session production mempunyai idle timeout dan dapat dicabut;
- rate limit, CSRF protection, secure cookie, CSP, dan TLS wajib;
- sandbox dan production memakai credential serta data yang terpisah.

## 9. Data inti

Implementasi portal minimal membutuhkan:

```text
partners
partner_members
partner_connections
partner_connection_versions
partner_capability_submissions
partner_service_snapshots
partner_certification_runs
partner_certification_evidence
partner_review_notes
partner_extension_listings
partner_publication_events
partner_incidents
partner_audit_logs
```

Tabel publikasi menyimpan versi listing dan capability yang disetujui agar
perubahan draft partner tidak langsung mengubah extension yang sedang aktif.

## 10. Status implementasi

Dokumen ini mendefinisikan rancangan produk dan kontrol operasional. Partner
Portal, endpoint onboarding, serta workflow publikasi belum dianggap tersedia
di production hanya karena telah didokumentasikan. Implementasi harus
mengikuti Partner Integration Contract, OpenAPI, sertifikasi, dan kebijakan
keamanan yang terkait.
