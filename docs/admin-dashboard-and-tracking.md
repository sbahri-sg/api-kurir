# Dashboard Admin dan Tracking

## 1. Scope

Milestone ini menyediakan:

- dashboard operasional Vite dan React;
- admin API dengan credential terpisah;
- daftar snapshot tarif provider untuk observasi;
- pencarian master lokal serta monitoring mapping provider otomatis;
- monitoring quota ledger tanpa menampilkan key;
- tambah, validasi, dan nonaktifkan credential provider dari dashboard;
- alat Cek Ongkir dan Cek Resi yang memakai service operasional yang sama
  dengan endpoint customer;
- generate, daftar, dan revoke API key customer;
- audit log untuk mutasi admin;
- registrasi resi yang terenkripsi dan terdeduplikasi;
- durable tracking queue yang dapat diproses beberapa worker.
- adapter tracking API resmi RajaOngkir yang nonaktif secara default.

Tidak ada scraping website carrier. Worker hanya memanggil endpoint provider
yang dikonfigurasi dan telah disetujui.

### 1.1 Struktur navigasi admin

Sidebar memakai kelompok yang dapat dibuka dan ditutup agar fungsi
observability tidak memenuhi navigasi utama:

```text
Dashboard
Operasional
  - Cek Ongkir
  - Cek Resi
  - Monitor Resi
Master Data
  - Ekspedisi & Service
Integrasi Provider
  - Credential & Kuota
  - Legacy RajaOngkir
      - Snapshot Tarif
      - Mapping Lokasi
Developer
  - Webhook
  - Dokumentasi API
  - API Key
```

Hanya satu kelompok terbuka pada satu waktu dan pilihan kelompok disimpan
selama session browser. `Snapshot Tarif` serta `Mapping Lokasi` tetap
read-only dan diletakkan di bawah `Legacy RajaOngkir`; keduanya bukan input
master untuk partner baru. Pada layar mobile, kelompok tetap berupa accordion
vertikal agar tidak menghasilkan horizontal overflow.

## 2. Autentikasi admin

Endpoint customer memakai:

```text
API_KEYS
```

Endpoint `/v1/admin/*` memakai:

```text
ADMIN_API_KEYS
```

Kedua key tidak saling menggantikan. `API_KEYS` menjadi bootstrap/recovery key.
Admin dapat membuat **Main Service key** dari dashboard tanpa restart; key ini
memiliki scope `gateway:access` dan boleh membawa `X-Emisell-Merchant-ID`.
**Public API key** hanya mengakses ongkir, lokasi, kurir, dan tracking serta
ditolak bila mencoba membawa konteks merchant. Production menolak startup bila
kelompok key environment tidak dikonfigurasi. Dashboard menyimpan admin key
hanya di `sessionStorage`, sehingga key hilang ketika sesi browser ditutup.

Header audit opsional:

```http
X-Admin-Actor: nama-operator
```

Nilainya dicatat sebagai alias. Nilai API key tidak pernah dicatat.

## 3. Endpoint admin

| Method | Endpoint | Fungsi |
|---|---|---|
| GET | `/v1/admin/overview` | Ringkasan rate, mapping, snapshot, quota, dan tracking |
| GET | `/v1/admin/catalog` | Kurir, service, dan rounding profile aktif (read-only) |
| GET | `/v1/admin/locations` | Search master lokasi lokal (read-only) |
| GET | `/v1/admin/couriers` | Katalog kemampuan kurir dan layanan untuk menu Ekspedisi & Service |
| POST | `/v1/admin/calculate/domestic-cost` | Cek ongkir dari dashboard |
| POST | `/v1/admin/track/waybill` | Cek resi dari dashboard |
| GET | `/v1/admin/tracking-operations` | Monitor AWB, validasi, revision, snapshot, dan antrean worker |
| DELETE | `/v1/admin/tracking-operations/{id}` | Hapus permanen data tracking dan seluruh turunannya |
| GET | `/v1/admin/rate-snapshots` | Membaca hasil tarif yang tersimpan otomatis |
| GET | `/v1/admin/location-mappings` | Membaca mapping yang dibentuk otomatis |
| GET | `/v1/admin/provider-quotas` | Membaca quota ledger |
| GET | `/v1/admin/provider-credentials` | Membaca metadata key provider termasking |
| POST | `/v1/admin/provider-credentials` | Validasi dan simpan key provider terenkripsi |
| POST | `/v1/admin/provider-credentials/{id}/disable` | Keluarkan key dari rotasi |
| GET | `/v1/admin/api-keys` | Membaca metadata dan status customer API key |
| POST | `/v1/admin/api-keys` | Generate customer API key; secret tampil satu kali |
| POST | `/v1/admin/api-keys/{id}/revoke` | Mencabut satu customer API key |
| GET | `/v1/admin/tracking-webhook` | Membaca URL, status, mask secret, dan hasil test terakhir |
| PUT | `/v1/admin/tracking-webhook` | Menyimpan callback URL dan status aktif |
| POST | `/v1/admin/tracking-webhook/secret` | Generate/rotate secret HMAC; plaintext tampil satu kali |
| POST | `/v1/admin/tracking-webhook/test` | Mengirim event `tracking.test` tanpa data pelanggan |

Tidak ada endpoint mutasi manual rate card maupun mapping. Master wilayah
berasal dari importer dataset wilayah lokal Kemendagri; mapping provider dan
snapshot tarif dibentuk otomatis saat cek ongkir pertama. Dashboard bersifat
observasi dan pencarian, bukan input data tarif atau mapping. Harga tidak dapat
dibuat, diedit, dipromosikan, atau dinonaktifkan oleh operator dashboard.
Mutasi dashboard hanya tersedia untuk lifecycle credential customer dan
provider, serta konfigurasi webhook Emisell; bukan untuk tarif atau mapping.

Menu **Monitor Resi** berbentuk tabel operasional. Data diperbarui setiap 15
detik dari PostgreSQL, bukan dari provider, sehingga halaman admin tidak
mengurangi kuota tracking. Operator dapat memfilter merchant/order, courier,
validasi, dan status job `pending`, `running`, `dead`, `final`, atau `idle`.
Khusus tabel ini AWB didekripsi di memory API dan ditampilkan penuh kepada staff
yang lolos `ADMIN_API_KEYS`. Endpoint customer, webhook, audit, dan log aplikasi
tetap menggunakan AWB termasking.

Staff dapat memakai aksi **Hapus permanen** untuk membersihkan data development
atau data uji. Aksi ini tersedia juga saat backend berstatus production karena
otorisasinya berasal dari admin API key, tetapi dashboard tetap meminta
konfirmasi terlebih dahulu. Penghapusan memakai UUID internal shipment dan
dilakukan dalam satu transaksi. Antrean worker, status history, subscription,
revision, dan webhook outbox ikut terhapus melalui relasi cascade. Audit admin
tetap dipertahankan dengan resi yang sudah termasking, actor, dan request ID.

Menu `Ekspedisi & Service` membaca katalog ini secara read-only. Kemampuan cek
ongkir, internasional, dan tracking berasal dari katalog provider yang
diverifikasi. Service lokal hanya ditampilkan bila sudah terdaftar; service
lain tetap mengikuti respons provider pada rute yang benar-benar dicek.

### 3.1 Manajemen webhook tracking

Menu **Developer → Webhook** menggantikan input manual URL dan secret untuk
deployment baru:

1. operator menyimpan callback URL backend Emisell;
2. backend API Kurir menghasilkan secret CSPRNG 256-bit;
3. plaintext secret hanya dikembalikan sekali untuk disalin ke Emisell;
4. database hanya menyimpan ciphertext AES-256-GCM, fingerprint, dan mask;
5. operator mengirim event `tracking.test` sebelum mengaktifkan delivery;
6. worker membaca perubahan database secara dinamis tanpa rebuild container.

Pada production, URL wajib HTTPS dan literal localhost/private IP ditolak.
Event test tidak membawa merchant, order, AWB, alamat, atau identitas penerima.
Rotate secret langsung membuat secret sebelumnya tidak berlaku dan otomatis
menonaktifkan delivery. Receiver Emisell harus diperbarui sebelum delivery
diaktifkan kembali; event yang menunggu tetap aman di outbox.

Variabel `TRACKING_WEBHOOK_ENABLED`, `EMISELL_TRACKING_WEBHOOK_URL`, dan
`EMISELL_TRACKING_WEBHOOK_SECRET` tetap didukung sebagai fallback migrasi.
Begitu konfigurasi database dibuat, nilai dashboard menjadi sumber utama,
termasuk ketika operator menonaktifkan webhook.

### 3.2 Keamanan provider credential

- form menerima `provider_code=rajaongkir|biteship` dan `api_key`;
- credential Biteship hanya dipakai sebagai fallback tracking, bukan tarif;
- key diuji ke endpoint resmi sebelum disimpan; satu request validasi ikut
  dicatat pada quota ledger;
- secret dienkripsi AES-256-GCM dengan
  `PROVIDER_CREDENTIAL_ENCRYPTION_KEY`;
- database menyimpan ciphertext, fingerprint SHA-256, empat karakter awal yang
  aman, dan empat karakter terakhir;
- response daftar/create hanya mengembalikan tampilan termasking;
- raw key tidak masuk response, log request, audit log, atau Postman
  environment setelah penyimpanan;
- key baru langsung tersedia bagi API dan worker tanpa restart;
- resolver memilih key aktif dengan rasio pemakaian terendah dan tidak
  melewati limit harian lokal;
- fallback legacy environment hanya digunakan bila belum ada key database,
  bukan ketika pool database kehabisan kuota;
- create dan disable dicatat pada audit log admin.

Kontrak target multi-provider tidak memperluas endpoint admin lama dengan
field arbitrary. Seller memakai Provider Account API terpisah:

```text
/v1/provider-accounts
```

Dashboard `Integrasi Provider` membedakan `provider_code`, `product_code`, dan
`environment`. Credential yang dimasukkan adalah connection token canonical
yang diterbitkan partner, bukan token API native partner/carrier. Secret tetap
write-only, sedangkan UI hanya menampilkan mask, status validasi, capability,
quota, serta waktu sync. Spesifikasi target berada di
[Provider Account API v1](provider-account-api-v1.md).

### 3.3 Keamanan customer API key

- secret memakai prefix `ek_live_` dan 32 byte acak dari CSPRNG;
- database hanya menyimpan hash SHA-256, prefix aman, dan empat karakter
  terakhir;
- secret lengkap hanya dikembalikan pada response generate dan hanya
  dipertahankan di memory halaman sampai panel ditutup;
- key hasil generate berlaku pada endpoint customer `/v1`, `/api/v1`,
  `/regions`, dan `/shipping`, tetapi tidak berlaku pada `/v1/admin`;
- generate tidak meminta nama atau masa berlaku; key aktif terus sampai
  di-revoke;
- revoke langsung membersihkan cache pada instance yang memproses request.
  Pada deployment multi-instance, instance lain berhenti menerima key paling
  lambat setelah TTL cache autentikasi 60 detik;
- setiap create dan revoke dicatat ke audit log tanpa secret atau hash.

### 3.4 Dokumentasi dan Postman

Menu `Dokumentasi API` pada dashboard berisi endpoint customer, health, dan
admin dan security beserta parameter serta contoh request/response. File berikut
tersedia langsung dari host dashboard:

```text
/api-kurir.postman_collection.json
/api-kurir.local.postman_environment.json
```

Collection menggunakan `{{api_key}}` untuk endpoint customer dan
`{{admin_api_key}}` untuk endpoint admin. Kedua nilainya kosong pada file
environment dan harus diisi pada Current Value Postman. Request pencarian
origin/destination otomatis menyimpan ID lokasi hasil pertama ke environment
agar contoh cek ongkir dapat langsung dijalankan.

Request `Add RajaOngkir Key` memakai secret sementara
`{{rajaongkir_api_key}}`, menyimpan hanya ID hasil ke
`{{provider_credential_id}}`, dan tidak menyalin provider key ke variable lain.
Hapus Current Value provider key di Postman setelah request berhasil.

## 4. Penyimpanan resi

Nomor resi dianggap data sensitif operasional. Database menyimpan:

```text
courier_code
waybill_hash
waybill_masked
waybill_ciphertext
provider_context_ciphertext
```

- `waybill_hash`: SHA-256 untuk deduplikasi;
- `waybill_masked`: hanya empat karakter terakhir untuk response publik,
  webhook, audit, dan log;
- `waybill_ciphertext`: AES-256-GCM dengan nonce acak;
- `provider_context_ciphertext`: konteks provider opsional untuk kompatibilitas,
  juga terenkripsi;
- plaintext hanya tersedia sesaat di memory worker ketika memanggil adapter dan
  di memory API ketika membentuk response **Monitor Resi** untuk staff admin.

Key:

```text
TRACKING_ENCRYPTION_KEY=<base64 yang decode menjadi tepat 32 byte>
```

Key tidak disimpan di database, source, dashboard, log, atau arsip.

## 5. Deduplikasi dan concurrency

Constraint:

```text
UNIQUE (courier_code, waybill_hash)
```

Partial unique index memastikan hanya satu job aktif:

```text
UNIQUE (shipment_id) WHERE status IN ('pending', 'running')
```

Worker mengambil job dengan:

```text
FOR UPDATE OF job SKIP LOCKED
```

Karena itu beberapa instance worker dapat memproses antrean yang sama tanpa
dua worker mengambil job identik. Kegagalan memakai exponential backoff dan
job berpindah ke `dead` setelah `max_attempts`.

## 6. Endpoint tracking

```http
POST /v1/track/waybill
Authorization: Bearer <customer-api-key>
Content-Type: application/json
```

```json
{
  "waybill": "0123456789012",
  "courier": "jne",
  "refresh": "if_stale"
}
```

Dashboard hanya meminta nomor resi dan ekspedisi. Field
`last_phone_number` tetap diterima sebagai konteks provider opsional untuk
kompatibilitas API lama; jika dikirim, nilainya terenkripsi dan tidak muncul
pada response, log, audit, atau dashboard.

Respons pertama `202`:

```json
{
  "data": {
    "courier": "jne",
    "waybill": "*********9012",
    "status": "unknown",
    "status_label": "",
    "summary": {},
    "events": [],
    "provider": "",
    "provider_fetched_at": null,
    "next_refresh_at": null,
    "is_final": false,
    "refresh_queued": true,
    "last_error_code": ""
  }
}
```

Request berulang untuk resi yang sama tidak membuat shipment atau job aktif
baru. Setelah adapter mengisi snapshot, endpoint dapat mengembalikan `200`.
Dashboard melakukan polling singkat otomatis saat `refresh_queued=true`. Jika
worker gagal, `last_error_code` ditampilkan sebagai penjelasan yang aman tanpa
membocorkan respons provider.

## 7. Adapter dan batas operasional

- `TRACKING_ENABLED=false` merupakan default aman.
- Kurir AWB default: `jne,sap,ninja,jnt,tiki,wahana,pos,lion`.
- SiCepat, IDExpress, dan Sentral Cargo memakai Biteship fallback karena
  matriks resmi RajaOngkir saat ini tidak menandai capability AWB mereka.
- AnterAja dan RPX tetap memakai RajaOngkir untuk tarif, tetapi tracking-nya
  memakai Biteship fallback. Paxel tersedia sebagai tracking-only.
- REX, NCS, STAR, dan DSE tetap rate-only sampai ada adapter tracking yang
  terverifikasi.
- Worker memakai pool credential RajaOngkir database ketika
  `TRACKING_ENABLED=true`; tambahkan setidaknya satu key valid sebelum
  mengaktifkan tracking.
- Worker tidak melakukan scraping website JNE atau carrier lain.
- Adapter memakai quota ledger yang sama dengan cek ongkir untuk credential
  alias yang sama.
- Redis tetap opsional; durability dan locking fase ini menggunakan
  PostgreSQL.

Detail interval polling, error, dan aktivasi ada di
[Operasional adapter tracking RajaOngkir](rajaongkir-tracking.md).
