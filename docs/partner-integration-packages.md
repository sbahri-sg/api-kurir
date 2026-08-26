# Partner Integration Package dan Review Console

Status: **Partner Portal, review internal, active release, dan rollback tersedia**

## 1. Tujuan dan batas

Partner Package adalah artefak versi yang dipakai staff Emisell untuk menilai
apakah Partner Connector milik vendor sesuai kontrak API Kurir. Alurnya mirip
version submission pada platform extension, tetapi package **bukan plugin yang
dijalankan di server API Kurir**.

```text
Partner source/connector (di-host partner)
              |
              | ZIP review artifact
              v
Partner Portal -> static validation -> API Kurir Admin review -> publish
              |
              +-> tidak mengeksekusi source code partner
```

Vendor tetap meng-host connector sandbox dan production pada infrastrukturnya.
API Kurir membaca URL serta kontrak OpenAPI dari package, lalu pada tahap
sertifikasi memanggil Partner Connector melalui jaringan terkontrol.

## 2. Urutan onboarding MVP

1. Staff membuat identitas provider pada menu **Provider**.
2. Staff membuka aksi **Akses partner** pada provider dan membuat partner access
   key. Secret hanya ditampilkan satu kali.
3. Vendor masuk ke `/partner`. Identitas provider diambil dari access key dan
   dikunci server-side; portal tidak menampilkan dropdown provider.
4. Vendor mengunggah ZIP dengan versi immutable. Request upload tidak menerima
   `provider_code` dari browser.
5. API Kurir memvalidasi keamanan dasar, manifest, dan OpenAPI secara statis.
6. Package yang lulus masuk `technical_review`; package yang gagal langsung
   masuk `changes_requested` dan tetap tersimpan sebagai bukti submission.
7. Reviewer memindahkan status melalui sandbox, security review, UAT, approved,
   dan published.
8. Status `published` menjadikan versi tersebut **active release** provider dan
   versi production sebelumnya otomatis menjadi `superseded`. Admin tetap
   mengaktifkan `available` secara terpisah setelah kontrol bisnis dan
   operasional selesai.
Staff tetap memakai admin API key untuk review. Vendor hanya menerima partner
access key yang terikat ke satu `provider_code`; admin API key dan endpoint
`/v1/admin/*` tidak boleh diberikan kepada vendor. Access key disimpan sebagai
SHA-256 hash, dapat dicabut staff, dan plaintext tidak dapat dibaca ulang.

## 3. Struktur ZIP

Ukuran maksimum compressed adalah 25 MB, maksimum expanded 100 MB, dan maksimum
250 entry. Upload dibatasi 10 percobaan per jam dan satu upload aktif untuk setiap
access key. Kuota penyimpanan awal adalah 25 versi atau total 500 MB per provider.
Dua file berikut wajib berada tepat pada root ZIP:

```text
partner-package.zip
├── emisell-extension.yaml
├── openapi.yaml
├── README.md             # direkomendasikan
├── docs/                # direkomendasikan
├── contract-tests/       # direkomendasikan
└── src/                 # opsional, hanya untuk review; tidak dieksekusi
```

Contoh `emisell-extension.yaml`:

```yaml
schema_version: "1"
provider:
  code: mengantar
  name: Mengantar
connector:
  contract_version: v1
  base_url: https://api.partner.example/partner/v1
capabilities:
  - rates
  - shipments
  - pickup
  - tracking
  - balance
services:
  - regular
  - next_day
  - economy
  - cargo
```

`provider.code` harus sama dengan provider yang terikat pada access key.
`connector.base_url` adalah satu endpoint HTTPS aktif milik connector. API Kurir
menentukan alamat gateway publik berdasarkan environment: domain lokal saat
development dan `api-kurir.emisell.com` saat production. Secret, API key, `.env`, private key,
credential native provider, dan data customer dilarang berada dalam ZIP.

Capability yang diterima pada MVP adalah `rates`, `shipments`, `pickup`,
`tracking`, dan `balance`. Group layanan yang diterima adalah `regular`,
`next_day`, `economy`, dan `cargo`. Nilai lain menghasilkan feedback manifest
dan status `changes_requested`.

## 4. Kontrak OpenAPI minimum

`openapi.yaml` wajib memakai OpenAPI 3.x. Path dasar selalu diperiksa:

| Method dan path | Kegunaan |
|---|---|
| `GET /health` | health connector |
| `GET /capabilities` | capability aktual |
| `GET /services` | katalog layanan |
| `POST /rates` | quote tarif |

Path tambahan mengikuti capability manifest:

| Capability | Operation wajib |
|---|---|
| `shipments` | `POST /shipments` |
| `pickup` | `POST /pickups` |
| `tracking` | `POST /tracking/waybills` |
| `balance` | `GET /account/balance` |

Detail request, response, signing, dan idempotency tetap mengikuti
[`openapi/partner-v1.yaml`](../openapi/partner-v1.yaml) dan
[`partner-api-v1.md`](partner-api-v1.md).

Jika provider memakai lebih dari satu API key, setiap key wajib dinyatakan
sebagai security scheme terpisah. Jangan menaruh nilai key di OpenAPI atau ZIP.
Visual API Explorer akan membuat form credential berdasarkan deklarasi ini:

```yaml
components:
  securitySchemes:
    shipping_cost:
      type: apiKey
      in: header
      name: key
      x-emisell-credential-code: shipping_cost
      x-emisell-label: RajaOngkir Shipping Cost
    shipping_delivery:
      type: apiKey
      in: header
      name: x-api-key
      x-emisell-credential-code: shipping_delivery
      x-emisell-label: RajaOngkir Shipping Delivery
paths:
  /rates:
    post:
      security:
        - shipping_cost: []
  /shipments:
    post:
      security:
        - shipping_delivery: []
```

Untuk connector berbasis RajaOngkir, domain upstream resmi berada di source
connector: Shipping Cost memakai `https://rajaongkir.komerce.id/api/v1/`,
sedangkan Shipping Delivery production memakai
`https://api.collaborator.komerce.id/`. URL manifest tetap domain connector
partner karena kontrak canonical API Kurir berbeda dari path native RajaOngkir.

## 5. Validasi keamanan upload

Validator saat ini melakukan pemeriksaan statis berikut:

- central directory ZIP valid;
- path traversal, absolute path, dan symbolic link ditolak;
- native binary seperti `.exe`, `.dll`, `.so`, `.dylib`, dan `.bin` ditolak;
- jumlah file, ukuran expanded, ukuran per-file, dan rasio kompresi dibatasi;
- `.env`, `id_rsa`, private key, serta pola credential tertentu ditolak;
- manifest dan OpenAPI wajib tersedia pada root;
- provider, contract version, HTTPS URL, capability, serta path canonical
  diperiksa.
- percobaan upload dibatasi secara persisten per access key dan upload paralel
  pada key yang sama ditolak;
- kuota jumlah versi dan total penyimpanan diperiksa atomik per provider.

Artefak disimpan di PostgreSQL sebagai `bytea`, hanya dapat dibaca oleh partner
pemilik atau staff, menggunakan `Cache-Control: private, no-store`, dan tidak
pernah dieksekusi. Setiap upload, perubahan status, dan download artifact
menghasilkan audit. Untuk skala besar, penyimpanan dapat dipindahkan ke object storage
private dengan encryption at rest, retention, dan signed URL yang sangat
singkat tanpa mengubah model submission.

Pemeriksaan malware berbasis signature eksternal belum terhubung pada MVP.
Dashboard selalu menampilkan warning ini. Karena itu hasil static scan bukan
jaminan keamanan dan tidak boleh melewati security review, sandbox isolation,
dependency scan, serta UAT.

## 6. Lifecycle versi

```text
technical_review -> sandbox_testing -> security_review -> uat
                 -> changes_requested | rejected

uat -> approved -> published -> suspended
suspended -> published | technical_review
```

- Versi bersifat immutable dan unik per provider.
- Package gagal scan dimulai dari `changes_requested`.
- Provider managed/hosted API Kurir dapat memakai endpoint official yang sama
  selama fase pengujian; sandbox domain terpisah tidak diwajibkan. Operasi
  transaksional tetap terkunci sampai tahap sertifikasi yang sesuai.
- `approved` dan `published` hanya dapat dicapai bila static scan lulus.
- Ketika versi baru menjadi `published`, versi published sebelumnya otomatis
  menjadi `superseded`.
- Versi `superseded` dapat dipublikasikan kembali sebagai rollback. Versi yang
  sedang aktif ditandai `is_active_release=true` pada API dan dashboard.
- Capability manifest diturunkan menjadi `required_scopes`, misalnya
  `rates:read`, `shipments:write`, `pickups:write`, dan `tracking:read`.
- Saat merchant mengaktifkan provider, release ID dan scope yang dipakai dipin
  pada instalasi merchant agar perubahan versi dapat diaudit.
- Semua perubahan status mencatat actor, request ID, status sebelumnya, status
  baru, dan catatan review pada audit admin.
- Revisi dilakukan dengan version baru, bukan menimpa ZIP lama.

## 7. Endpoint internal admin

| Method | Endpoint | Fungsi |
|---|---|---|
| `GET` | `/v1/admin/partner-submissions` | list dan filter submission |
| `GET` | `/v1/admin/partner-submissions/{id}` | detail scan dan manifest |
| `GET` | `/v1/admin/partner-submissions/{id}/artifact` | download ZIP karantina |
| `PUT` | `/v1/admin/partner-submissions/{id}/status` | review lifecycle |
| `GET` | `/v1/admin/shipping-providers/{code}/partner-access-keys` | list key termasking |
| `POST` | `/v1/admin/shipping-providers/{code}/partner-access-keys` | generate key satu kali |
| `POST` | `/v1/admin/shipping-providers/{code}/partner-access-keys/{id}/revoke` | cabut akses |

Semua endpoint menggunakan `Authorization: Bearer <admin-api-key>` dan
`X-Admin-Actor`. Kontrak request/response lengkap berada di
[`openapi/public.yaml`](../openapi/public.yaml).

## 8. Endpoint Partner Portal

| Method | Endpoint | Fungsi |
|---|---|---|
| `GET` | `/partner/v1/me` | identitas provider dari access key |
| `GET` | `/partner/v1/starter-package` | starter ZIP dengan identitas provider otomatis |
| `GET` | `/partner/v1/submissions` | versi milik provider terautentikasi |
| `POST` | `/partner/v1/submissions` | upload `version` + `package`, tanpa provider code |
| `GET` | `/partner/v1/submissions/{id}` | detail milik provider sendiri |
| `GET` | `/partner/v1/submissions/{id}/artifact` | download package milik provider sendiri |

Semua endpoint memakai `Authorization: Bearer <partner-access-key>`. Backend
mengabaikan konteks provider dari browser dan selalu memakai provider yang
terikat pada key. Percobaan membaca ID submission provider lain menghasilkan
404 agar tidak membocorkan keberadaan data.

## 9. Yang belum dikerjakan

- akun personal, MFA, serta role anggota tim partner; fase awal memakai access
  key provider yang dapat dirotasi;
- external malware/dependency scanner;
- official read-only runner untuk health, rates, dan tracking sudah tersedia;
  runner transaksi shipment/pickup/cancel, retry, full response schema, dan
  performance masih tahap berikutnya;
- evidence upload per test case dan approval dua pihak;
- object storage, retention policy otomatis, dan deletion workflow;
- workflow approval dua pihak dan rollout bertahap per persentase merchant.

`README.md`, `SECURITY.md`, `CHANGELOG.md`, source code, dan SBOM belum menjadi
hard requirement pada fase awal. File tersebut tetap direkomendasikan agar
review teknis lebih cepat. Keamanan MVP dipusatkan pada isolasi provider,
pengamanan arsip, pembatasan penggunaan, secret scan, kontrak, audit, dan
larangan mengeksekusi source partner.

Daftar ini sengaja eksplisit agar status `published` pada console MVP tidak
dianggap sebagai izin menjalankan kode partner di API Kurir.
