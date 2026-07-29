# Keamanan, Credential, dan Request Signing

Status: **wajib untuk production**
Terakhir ditinjau: **29 Juli 2026**

## 1. Trust boundary

API Kurir memisahkan:

- customer API key Public API;
- admin API key dashboard;
- Partner Connector credential untuk koneksi seller;
- Partner Connector access token;
- Partner Connector signing secret;
- partner webhook token;
- partner webhook signing secret;
- master encryption key aplikasi.

Credential dari satu boundary tidak boleh diterima pada boundary lain.

## 2. Penyimpanan secret

Partner Connector secret:

- diterima hanya melalui TLS;
- dienkripsi sebelum ditulis ke database;
- memakai AES-256-GCM atau envelope encryption dari KMS;
- mempunyai nonce unik;
- menyimpan key version untuk rotasi master key;
- tidak pernah ditampilkan kembali;
- hanya didekripsi di worker yang melakukan provider call.

Master encryption key berada di secret manager/environment, bukan database,
source code, Compose, log, atau backup biasa.

## 3. Request signing v1

Semua request mutasi Partner Connector dan seluruh webhook standard memakai
HMAC-SHA256.

Canonical string:

```text
v1
<unix_timestamp>
<nonce>
<HTTP_METHOD_UPPERCASE>
<normalized_path>
<normalized_query>
<lowercase_hex_sha256_body>
```

Contoh:

```text
v1
1785290400
01J8NONCEEXAMPLE
POST
/partner/v1/shipments

8f14e45fceea167a5a36dedd4bea2543...
```

Signature:

```text
base64url_without_padding(
  HMAC-SHA256(signing_secret, canonical_string)
)
```

Aturan normalisasi:

- path memakai percent-encoding RFC 3986;
- query diurutkan berdasarkan key lalu value;
- spasi query menjadi `%20`, bukan `+`;
- body adalah byte persis yang dikirim;
- body kosong memakai SHA-256 dari empty byte array;
- JSON tidak di-parse dan di-serialize ulang sebelum verifikasi.

## 4. Verifikasi

Penerima:

1. mencari secret berdasarkan `X-Partner-Key-Id`;
2. memeriksa token akses secara constant time;
3. menolak timestamp di luar toleransi 300 detik;
4. menolak nonce yang sudah dipakai oleh key tersebut;
5. menghitung body hash dan signature;
6. membandingkan signature secara constant time;
7. baru melakukan parsing dan validasi business schema.

Nonce disimpan sedikit lebih lama dari replay window. Gangguan cache nonce
harus fail-closed untuk endpoint mutasi.

## 5. Rotasi key

Setiap connection mendukung dua key aktif sementara:

```text
current
next
```

Alur rotasi:

1. terbitkan `next` dengan ID berbeda;
2. kedua pihak memverifikasi test vector;
3. partner mulai memakai `next`;
4. monitor minimal satu replay window;
5. revoke `current`;
6. hapus material key lama sesuai retention.

Key ID bukan secret dan tidak boleh memuat nama seller.

## 6. Idempotency

Endpoint create shipment, pickup, cancel, payment, dan batch label wajib
memakai `Idempotency-Key`.

Database menyimpan:

```text
principal_id
endpoint
idempotency_key
request_hash
response_status
response_body_encrypted_or_reference
created_at
expires_at
```

Key sama dan body sama mengembalikan hasil terdahulu. Key sama dengan body
berbeda menghasilkan `409 IDEMPOTENCY_CONFLICT`.

## 7. SSRF dan URL eksternal

URL label, driver photo, POD, dan live tracking:

- hanya `https`;
- DNS di-resolve dan diperiksa ulang setelah redirect;
- blok loopback, link-local, private, multicast, dan metadata IP;
- batasi redirect;
- batasi ukuran dan content type;
- gunakan allowlist host bila provider stabil;
- jangan melakukan fetch dari request customer arbitrary.

Live tracking URL diperlakukan sebagai bearer secret dan tidak boleh masuk log.

## 8. Data pribadi

Data sensitif:

- nama dan telepon pengirim/penerima;
- alamat dan koordinat;
- AWB;
- identitas driver;
- foto/tanda tangan POD;
- isi paket dan nilai barang;
- saldo, payment ID, dan PIN.

Kontrol:

- field-level encryption untuk AWB/telepon bila diperlukan;
- masking pada dashboard;
- akses berbasis seller;
- log redaction;
- audit akses POD;
- retention berdasarkan kebutuhan operasional dan kontrak;
- penghapusan/anonimisasi setelah retention berakhir.

PIN selalu write-only, tidak disimpan, tidak dicatat, dan tidak masuk retry
payload persisten.

## 9. Webhook native

API Kurir hanya menerima Partner Event Webhook canonical dengan HMAC.
Keterbatasan webhook native carrier atau aggregator ditangani partner pada
infrastrukturnya sendiri. Partner bertanggung jawab memverifikasi, melakukan
dedup, merekonsiliasi, lalu menerbitkan ulang event canonical bertanda tangan.

Bearer tanpa signature atau IP allowlist saja tidak memenuhi kontrak Partner
API production.

## 10. Network dan runtime

- TLS 1.2 minimum, TLS 1.3 direkomendasikan.
- Egress worker dibatasi ke host provider terdaftar.
- Dashboard tidak pernah memanggil provider langsung.
- Database dan Redis tidak dipublikasikan ke internet.
- Provider response dibatasi ukuran dan waktu.
- Circuit breaker per provider account, bukan global.
- Production dan sandbox memakai credential serta database namespace berbeda.

## 11. Incident response

Credential dianggap bocor jika muncul di:

- chat;
- issue/PR;
- log;
- screenshot;
- repository Git;
- browser bundle;
- monitoring label.

Tindakan:

1. revoke/rotate Partner Connector credential;
2. revoke connection token dan signing secret;
3. audit pemakaian sejak waktu paparan;
4. invalidasi session terkait;
5. hapus secret dari sumber dan history sesuai prosedur;
6. dokumentasikan insiden tanpa menyalin secret.

## 12. Production gate

Integrasi tidak boleh production sebelum:

- TLS dan DNS tervalidasi;
- test vector HMAC lulus;
- key rotation diuji;
- idempotency concurrency diuji;
- webhook replay ditolak;
- secret redaction diuji;
- tenant isolation diuji;
- DPA/izin pemrosesan data selesai;
- webhook canonical dan endpoint rekonsiliasi aktif.
