# Partner Portal dan Publikasi Extension

Status: **portal dan submission/review tersedia**

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
| Mengatur satu URL connector HTTPS aktif | Ya | Validasi |
| Mengatur webhook dan melakukan rotasi credential | Ya | Audit |
| Mendeklarasikan capability dan service | Ya | Sertifikasi |
| Menjalankan contract test | Ya | Melihat bukti |
| Menjalankan official read-only API Explorer | Ya | Melihat bukti dan audit |
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
technical_review | sandbox_testing | security_review | uat
  -> changes_requested | rejected
published --(versi baru published)--> superseded
superseded --(rollback admin)--> published
```

| Status | Makna |
|---|---|
| `draft` | Profil atau konfigurasi partner belum lengkap |
| `submitted` | Partner telah mengajukan integrasi untuk diperiksa |
| `technical_review` | Kontrak endpoint, autentikasi, service, dan status mapping sedang diperiksa |
| `sandbox_testing` | Nama status kompatibilitas untuk contract test; tidak mewajibkan domain sandbox terpisah |
| `security_review` | Signature, replay protection, isolasi tenant, dan perlindungan data diperiksa |
| `uat` | Skenario operasional end-to-end diuji bersama |
| `approved` | Versi connector dan capability yang diuji telah disetujui |
| `published` | Extension dapat ditemukan dan diaktifkan oleh seller Emisell |
| `suspended` | Order baru dihentikan karena insiden, pelanggaran, atau hasil review |
| `changes_requested` | Versi ini perlu diperbaiki; partner mengirim versi immutable baru |
| `rejected` | Versi ditolak dan tidak dapat dipublikasikan |
| `superseded` | Versi published lama telah digantikan versi yang lebih baru |

Partner tidak dapat mengubah status menjadi `approved`, `published`, atau
`suspended`. Transisi tersebut hanya dilakukan admin API Kurir dan wajib
menghasilkan audit log.

Provider dibedakan menjadi tiga jenis. `built_in` adalah Emisell Kurir,
`managed_upstream` adalah adapter langsung yang dikelola API Kurir, sedangkan
`partner_hosted` adalah connector yang mengikuti lifecycle package. RajaOngkir
menggunakan `partner_hosted` canonical yang di-host API Kurir dan tetap memakai
credential BYOK seller per capability.
Partner access key dan Partner Portal hanya tersedia untuk `partner_hosted`.
Distribusi bukan pilihan partner maupun admin. Provider eksternal selalu
bernilai `merchant` dan hanya tampil pada katalog merchant Emisell yang
terautentikasi. Provider bawaan tetap memakai nilai `built_in`.

Perubahan besar setelah publikasi, seperti major API version, autentikasi,
status mapping, perhitungan tarif, COD, atau settlement, membuat capability
terkait kembali ke `technical_review`. Versi production lama dapat tetap
melayani shipment aktif sampai proses migrasi disetujui.

Provider uji coba yang belum pernah dipublikasikan dan belum mempunyai data
merchant dapat dihapus permanen oleh admin. Penghapusan ini sekaligus
membersihkan submission package, Partner Access Key, credential Explorer,
hasil test, dan katalog service provider. Provider bawaan, active release,
riwayat published, credential seller, atau data operasional selalu dilindungi.

## 5. Menu Partner Portal

### 5.1 Overview

Menampilkan:

- status onboarding dan persentase checklist;
- endpoint connector aktif dan status release;
- capability yang diajukan, lulus, atau ditolak;
- uptime, latency, error rate, dan webhook lag;
- tugas partner dan catatan reviewer terbaru;
- versi connector yang aktif.

Versi yang ditetapkan untuk production ditandai sebagai **active release**.
Publishing versi baru dan rollback ke versi lama selalu atomik, tercatat pada
audit, dan tidak mengubah file ZIP versi mana pun.

### 5.2 Company Profile

Berisi identitas perusahaan, dokumen legal, kontak bisnis, kontak teknis,
kontak insiden, logo, deskripsi, wilayah layanan, dan alamat kebijakan
privasi. Dokumen sensitif hanya dapat dilihat role reviewer yang berwenang.

### 5.3 API Connection

Partner mengatur:

- satu base URL connector HTTPS aktif;
- metode autentikasi canonical;
- public key atau key ID untuk request signing;
- webhook configuration;
- IP allowlist opsional;
- jadwal rotasi credential;
- status maintenance dan kontak insiden.

Credential resmi sistem native partner boleh dimasukkan hanya melalui Visual
API Explorer untuk pengujian read-only. Credential tersebut dienkripsi,
terpisah dari ZIP dan Partner Access Key, serta tidak otomatis menjadi runtime
production credential.

### 5.4 Integration Package

Partner mengirim versi connector sebagai ZIP review artifact dengan manifest
dan OpenAPI. Package tidak dipasang atau dijalankan pada API Kurir; connector
tetap di-host pada infrastruktur partner. Hasil static validation, SHA-256,
status review, dan catatan reviewer ditampilkan per versi.

Upload mandiri tersedia melalui `/partner` dengan access key yang diterbitkan
staff dari master Provider. Access key terikat ke satu provider, sehingga portal
tidak mempunyai dropdown provider dan request upload tidak menerima
`provider_code`. Developer dapat mengunduh starter ZIP yang identitas
providernya sudah terisi melalui `GET /partner/v1/starter-package`. Menu
**Partner Packages** pada API Kurir Admin hanya digunakan
staff untuk review. Format package, endpoint, batas keamanan, dan kekurangan
fase awal dijelaskan pada [Partner Integration Package](partner-integration-packages.md).

### 5.4.1 Visual API Explorer

Setelah package lulus static scan, partner dapat memilih versi, capability,
dan endpoint dari `openapi.yaml`, lalu menjalankan request read-only melalui
controlled test runner. Base URL menggunakan production/official URL karena
tidak semua provider menyediakan sandbox key. Portal menampilkan peringatan
bahwa request dapat memakai quota resmi provider.

Endpoint transaksi tetap terlihat tetapi terkunci sampai tersedia approval,
test account, idempotency, serta batas transaksi. Detail keamanan dan endpoint
portal berada pada [Partner Visual API Explorer](partner-visual-api-explorer.md).

### 5.5 Capabilities

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

Untuk MVP, manifest upload hanya menerima capability `rates`, `shipments`,
`pickup`, `tracking`, dan `balance`. Group layanan yang dapat diteruskan ke
Emisell dibatasi pada `regular`, `next_day`, `economy`, dan `cargo`. Capability
atau group lain ditambahkan kemudian melalui versi kontrak yang eksplisit,
bukan melalui string bebas pada manifest.

### 5.6 Services

Menampilkan katalog service yang dikirim Partner Connector API, misalnya
regular, express, cargo, trucking, same-day, atau instant. Data bersifat
read-only dari respons partner dan tidak menjadi form input tarif manual.

API Kurir menyimpan kode service mentah untuk audit serta hasil normalisasi
canonical. Partner bertanggung jawab atas mapping dari kode canonical menuju
kode internalnya.

Katalog setiap partner disimpan terpisah dari katalog first-party Emisell
Kurir. Setelah sinkronisasi connector dan sertifikasi selesai, hanya pasangan
kurir/service milik provider tersebut yang diteruskan oleh
`GET /api/v1/integrations/shipping-services` ketika seller mengaktifkan
provider itu. Pergantian provider tidak mencampur katalog maupun pilihan
service seller. Emisell Kurir tetap menggunakan katalog internal dan tidak
perlu mengunggah Partner Package.

### 5.7 Pengujian kontrak

Partner dapat menjalankan contract test dan melihat:

- test case, request ID, waktu, latency, dan hasil;
- perbedaan schema atau signature;
- idempotency dan concurrency test;
- duplicate serta out-of-order webhook;
- skenario timeout dan rekonsiliasi;
- test data yang aman tanpa data customer production.

Secret, nomor telepon, alamat lengkap, dan payload sensitif harus disamarkan.

### 5.8 Certification

Berisi checklist legal, teknis, keamanan, UAT, performance, dan operasional.
Setiap item memiliki status, catatan reviewer, bukti, waktu pengujian, serta
penanggung jawab. Partner dapat memperbaiki kekurangan dan mengajukan ulang.

### 5.9 Shipments and Logs

Partner hanya dapat melihat request yang menuju koneksi miliknya sendiri:

- shipment dan pickup reference;
- AWB termasking;
- status canonical;
- response code dan latency;
- retry, webhook, dan reconciliation result;
- correlation/request ID untuk investigasi.

Log bukan sumber untuk mengambil credential atau data pribadi mentah.
Retention mengikuti kebijakan keamanan dan perjanjian pemrosesan data.

### 5.10 Extension Listing

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

### 5.11 Incidents and Support

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
partner_integration_submissions
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

Console internal untuk static validation, versioning, review, active release,
rollback, audit, dan download karantina sudah diimplementasikan. Partner Portal
MVP berbasis access key juga sudah tersedia dan mengunci satu provider pada
server. Akun personal/MFA, dynamic sandbox runner, external malware scan,
provisioning credential runtime, dan rollout bertahap belum dianggap tersedia
di production. Implementasi berikutnya tetap harus mengikuti Partner
Integration Contract, OpenAPI, sertifikasi, dan kebijakan keamanan yang terkait.
