# Provider Account API v1

Status: **contract-first**
Audience: **Emisell dan dashboard seller/admin**
OpenAPI: `openapi/provider-account-v1.yaml`

## 1. Tujuan

Provider Account API mengatur aktivasi koneksi seller ke Partner Connector.
API ini bukan Partner Connector API dan tidak boleh dapat diakses partner
eksternal.

Satu seller dapat mengaktifkan beberapa provider dan beberapa produk pada
provider yang sama. API Kurir hanya menerima connection credential yang
diterbitkan partner untuk kontrak canonical. Token API native yang digunakan
partner ke carrier, aggregator, atau sistem internalnya tetap dikelola partner.

## 2. Model akun

Identitas unik akun:

```text
seller_id + provider_code + product_code + environment + account_alias
```

Contoh:

```json
{
  "provider_account_id": "pva_01J...",
  "provider_code": "vendor_x",
  "product_code": "shipping",
  "environment": "live",
  "account_alias": "vendor-x-primary",
  "status": "active",
  "credential_mask": "cnx_...Yzd",
  "capabilities": [
    "rates",
    "shipment_create",
    "pickup",
    "label",
    "tracking",
    "webhook"
  ]
}
```

`account_alias` adalah label teknis, bukan nama pengguna dan bukan identitas
customer.

## 3. Autentikasi dan otorisasi

Request menggunakan customer/admin API key API Kurir. `seller_id` diambil dari
principal terautentikasi dan tidak boleh dipercaya dari body request.

Scope:

| Scope | Fungsi |
|---|---|
| `provider_accounts:read` | melihat konfigurasi dan status |
| `provider_accounts:write` | menambah, mengubah, menonaktifkan akun |
| `provider_accounts:validate` | memvalidasi koneksi canonical partner |
| `provider_accounts:preferences` | mengatur prioritas dan kurir |
| `provider_accounts:balance` | melihat saldo provider jika tersedia |

Admin lintas tenant harus memakai endpoint dan credential admin terpisah.

## 4. Menambah koneksi provider

```http
POST /v1/provider-accounts
Idempotency-Key: 01J...ULID
```

```json
{
  "provider_code": "vendor_x",
  "product_code": "shipping",
  "environment": "live",
  "account_alias": "vendor-x-primary",
  "credential": {
    "type": "partner_connection_token",
    "secret": "connection-token-issued-by-partner"
  }
}
```

Ketentuan:

- secret hanya diterima melalui HTTPS;
- secret harus merupakan credential Partner Connector canonical, bukan token
  native RajaOngkir, KiriminAja, atau carrier lain;
- secret tidak boleh masuk access log, trace, error, atau analytics;
- response tidak pernah mengembalikan secret;
- secret dienkripsi dengan envelope encryption/AES-256-GCM;
- database menyimpan fingerprint untuk mendeteksi credential identik;
- validasi Partner Connector dilakukan asynchronous jika partner lambat;
- akun belum ikut routing sebelum status `active`.

Response:

```json
{
  "provider_account_id": "pva_01J...",
  "provider_code": "vendor_x",
  "product_code": "shipping",
  "environment": "live",
  "status": "pending_validation",
  "credential_mask": "****xYz9",
  "validation": {
    "status": "queued",
    "last_checked_at": null,
    "message": null
  }
}
```

## 5. Validasi koneksi

```http
POST /v1/provider-accounts/{provider_account_id}/validate
Idempotency-Key: 01J...ULID
```

Validasi minimum:

1. connection credential dapat mengakses `health`, `capabilities`, dan
   `services` pada Partner Connector;
2. produk dan environment sesuai;
3. capability diperoleh dari response canonical partner;
4. identitas akun/fingerprint tidak mengungkap secret;
5. quota atau batas provider dicatat bila tersedia.

Validasi tidak boleh membuat order, pickup, payment, atau biaya.

Status:

```text
pending_validation
active
invalid
suspended
disabled
rotating
```

## 6. Rotasi credential

```http
POST /v1/provider-accounts/{provider_account_id}/rotate
Idempotency-Key: 01J...ULID
```

```json
{
  "credential": {
    "type": "partner_connection_token",
    "secret": "new-connection-token"
  }
}
```

Connection credential baru divalidasi sebelum menggantikan credential aktif.
Credential lama dipertahankan dalam grace period terenkripsi hanya jika
Partner Connector mendukung rotasi tanpa downtime. Setelah selesai, ciphertext
lama dihapus sesuai retention policy.

## 7. Menonaktifkan akun

```http
DELETE /v1/provider-accounts/{provider_account_id}
```

Operasi ini bersifat soft-disable:

- akun berhenti menerima quote dan shipment baru;
- shipment berjalan tetap dapat menerima webhook dan tracking;
- credential baru dihapus setelah tidak dibutuhkan untuk rekonsiliasi;
- audit event wajib disimpan.

## 8. Capability

Capability canonical:

```text
rates
domestic_rates
international_rates
external_tracking
shipment_create
shipment_cancel
scheduled_pickup
on_demand_pickup
dropoff
label
batch_label
tracking
webhook
cod
insurance
instant
multi_package
multi_destination
balance
payment_inquiry
```

Capability hasil probe belum otomatis aktif. API Kurir hanya mengaktifkan
capability yang lulus UAT dan diizinkan kontrak seller/provider.

## 9. Preferensi routing

```http
PUT /v1/provider-accounts/{provider_account_id}/preferences
```

```json
{
  "enabled": true,
  "priority": 20,
  "courier_allowlist": ["jne", "sicepat", "gosend"],
  "service_group_allowlist": ["regular", "cargo", "instant"],
  "routing": {
    "rates": true,
    "shipment": true,
    "tracking": true
  }
}
```

Ketentuan:

- allowlist kosong berarti seluruh hasil yang diizinkan provider;
- daftar kurir divalidasi terhadap katalog hasil sinkronisasi;
- preferensi seller tidak mengubah capability aktual;
- prioritas lebih kecil dipilih lebih dahulu jika strategi seller adalah
  `provider_priority`;
- perubahan tidak mengubah shipment yang sudah dibuat.

## 10. Daftar akun

```http
GET /v1/provider-accounts
GET /v1/provider-accounts/{provider_account_id}
```

Response boleh menampilkan:

- metadata akun;
- status validasi;
- capability;
- pemakaian quota;
- saldo jika scope dan provider mendukung;
- waktu sync terakhir;
- credential mask/fingerprint pendek.

Response tidak boleh menampilkan ciphertext, nonce enkripsi, secret lama,
token native partner, PIN, atau master encryption key.

## 11. Saldo dan payment inquiry

```http
GET /v1/provider-accounts/{provider_account_id}/balance
GET /v1/provider-accounts/{provider_account_id}/payments/{payment_id}
```

Endpoint bersifat opsional dan hanya aktif bila capability tersedia. Saldo
provider bukan saldo Emisell dan tidak boleh digabung tanpa ledger.

API Kurir menyimpan:

```text
provider_account_id
provider_transaction_id
shipment_id
transaction_type
amount
currency
provider_status
occurred_at
reconciled_at
```

Jenis transaksi:

```text
debit
refund
adjustment
cod_settlement
topup_observed
```

Jika sistem native provider membutuhkan PIN atau credential tambahan, partner
menanganinya di infrastrukturnya sendiri. Provider Account API tidak menerima
PIN maupun token native tersebut.

## 12. Audit

Event audit minimum:

```text
provider_account.created
provider_account.validation_requested
provider_account.validated
provider_account.validation_failed
provider_account.rotated
provider_account.preference_changed
provider_account.disabled
provider_account.balance_read
```

Audit menyimpan actor, seller, request ID, perubahan metadata, dan waktu.
Secret serta data pribadi tidak boleh dimasukkan ke audit.

## 13. Rate limit

Provider Account API tidak berada pada jalur checkout. Rate limit dibuat lebih
ketat daripada Public API:

- create/rotate: 10 per jam per seller;
- validate: 30 per jam per seller dan tetap tunduk pada limit provider;
- list/detail: 120 per menit per seller;
- balance: cache singkat dan maksimal 30 per menit per akun.

## 14. Error contract

```json
{
  "error": {
    "code": "PROVIDER_CREDENTIAL_INVALID",
    "message": "Credential tidak dapat divalidasi.",
    "retryable": false,
    "request_id": "req_01J...",
    "details": {}
  }
}
```

Kode minimum:

```text
PROVIDER_ACCOUNT_NOT_FOUND
PROVIDER_PRODUCT_UNSUPPORTED
PROVIDER_CREDENTIAL_INVALID
PROVIDER_CREDENTIAL_DUPLICATE
PROVIDER_ENVIRONMENT_MISMATCH
PROVIDER_VALIDATION_RATE_LIMITED
PROVIDER_TEMPORARILY_UNAVAILABLE
PROVIDER_CAPABILITY_NOT_CERTIFIED
PROVIDER_ACCOUNT_HAS_ACTIVE_SHIPMENTS
```

## 15. Status implementasi

Dokumen ini adalah kontrak target. Pembuatan OpenAPI tidak otomatis membuat
route atau migration menjadi aktif.
