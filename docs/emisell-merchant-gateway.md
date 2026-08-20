# Emisell Merchant Gateway dan RajaOngkir BYOK

Dokumen ini menjelaskan cara API Kurir bertindak sebagai gateway saja ketika
seller Emisell mengaktifkan RajaOngkir dengan API key miliknya sendiri.

## Prinsip kepemilikan

- Tenant utama adalah `merchant_id` Emisell, bukan domain.
- Satu merchant boleh mempunyai banyak domain dan tetap memakai credential serta
  ledger kuota yang sama.
- `domain_id` hanya menjadi metadata untuk memilih gudang atau konfigurasi
  storefront khusus.
- API key RajaOngkir seller disimpan terenkripsi di API Kurir dan tidak pernah
  dikembalikan setelah request pembuatan.
- Request tenant tidak pernah meminjam credential platform atau seller lain.

## Dua lapisan autentikasi

Setiap request Emisell menggunakan dua bukti:

1. Header `key` atau Bearer customer API key membuktikan pemanggil adalah
   service Emisell yang diizinkan.
2. Header `X-Emisell-Tenant-Token` membuktikan merchant pemilik request.

Tenant token adalah JWT EdDSA yang ditandatangani private key Ed25519 milik
`api-service`. API Kurir hanya menyimpan public key pada
`TENANT_CONTEXT_PUBLIC_KEY`.

Claim minimum:

```json
{
  "iss": "emisell-api",
  "aud": "api-kurir",
  "sub": "merchant_123",
  "integration_id": "11111111-2222-4333-8444-555555555555",
  "domain_id": "domain_abc",
  "scope": ["shipping:read", "tracking:read"],
  "iat": 1787112000,
  "exp": 1787112060,
  "jti": "request_unique_id"
}
```

`exp - iat` tidak boleh melebihi `TENANT_CONTEXT_MAX_TTL`. Nilai default adalah
lima menit. `integration_id` wajib diisi untuk memakai RajaOngkir BYOK. Jika
dikosongkan, request berada pada jalur Emisell Kurir gratis dan tidak boleh
memakai credential berbayar milik seller maupun platform secara implisit.

## Aktivasi seller

1. Dashboard Emisell memvalidasi sesi seller dan menentukan `merchant_id` dari
   database, bukan dari body browser.
2. Seller memasukkan API key RajaOngkir dan batas harian paketnya.
3. Backend Emisell membuat tenant token dengan scope
   `provider-credentials:write`.
4. Backend memanggil `POST /api/v1/integrations/provider-credentials`.
5. API Kurir memvalidasi key ke RajaOngkir, mengenkripsi secret, mencatat satu
   hit validasi, dan mengembalikan UUID credential sebagai `integration_id`.
6. Emisell menyimpan UUID tersebut pada konfigurasi extension seller. Secret
   provider tidak disimpan di browser atau log Emisell.

Seluruh endpoint Emisell Gateway memakai base path canonical `/api/v1`.
Path lama `/v1/integrations` dipertahankan sementara sebagai alias kompatibilitas,
tetapi integrasi baru wajib menggunakan `/api/v1/integrations`.

## Multi-domain

Semua domain berikut memakai claim `sub` yang sama:

```text
domain-a.com ─┐
domain-b.com ─┼─> merchant_123 ─> credential RajaOngkir merchant_123
domain-c.com ─┘
```

Jika satu domain memakai gudang berbeda, Emisell menentukan lokasi asal sebelum
memanggil rate API dan mengirim `domain_id` sebagai metadata audit. Credential
tetap dimiliki merchant dan tidak diduplikasi per domain.

## Rate dan tracking

Rate memakai kontrak kompatibel RajaOngkir V2:

```http
POST /api/v1/calculate/district/domestic-cost
key: <customer-api-key>
X-Emisell-Tenant-Token: <signed-tenant-jwt>
Content-Type: application/x-www-form-urlencoded
```

Tracking memakai header yang sama pada `POST /api/v1/track/waybill`. Tenant dan
credential disimpan bersama shipment tracking sehingga worker lanjutan tetap
menggunakan credential merchant yang benar.

Snapshot exact quote dipisahkan berdasarkan `tenant_id` dan `integration_id`.
Hal ini mencegah tarif kontrak atau diskon akun seller A terbaca oleh seller B.
Ledger kuota juga menyimpan tenant owner dan hanya bertambah ketika benar-benar
terjadi hit provider.

## Scope

| Scope | Kegunaan |
|---|---|
| `provider-credentials:read` | Membaca metadata key milik merchant |
| `provider-credentials:write` | Menambah atau menonaktifkan key merchant |
| `shipping:read` | Mengambil tarif dengan credential merchant |
| `shipping:write` | Mengatur kurir dan layanan yang boleh tampil di checkout |
| `tracking:read` | Melacak AWB dengan credential merchant |
| `tracking:write` | Mendaftarkan fulfillment untuk checkpoint tracking |

Untuk order fulfillment, backend Emisell mendaftarkan AWB melalui
`POST /api/v1/integrations/tracking/subscriptions`, lalu membaca snapshot pada
`GET /api/v1/integrations/tracking/subscriptions/{fulfillment_id}`. Perubahan
status dikirim melalui webhook HMAC. Kontrak lengkap tersedia pada
[`tracking-checkpoint-and-webhooks.md`](tracking-checkpoint-and-webhooks.md).

Kontrak pilihan kurir/layanan dan pola UI bertingkat dijelaskan lengkap pada
[`merchant-shipping-services.md`](merchant-shipping-services.md).

## Error penting

| Kode | Arti |
|---|---|
| `TENANT_CONTEXT_REQUIRED` | Header tenant token tidak dikirim |
| `INVALID_TENANT_CONTEXT` | Signature, issuer, audience, atau masa berlaku salah |
| `INSUFFICIENT_TENANT_SCOPE` | Token tidak memiliki scope operasi |
| `PROVIDER_KEY_EXISTS` | Key yang sama sudah pernah disimpan |
| `INVALID_PROVIDER_KEY` | Key ditolak RajaOngkir |
| `PROVIDER_QUOTA_EXHAUSTED` | Ledger atau provider menyatakan kuota habis |
| `RATE_NOT_AVAILABLE` | Merchant tidak mempunyai credential aktif atau tarif tidak tersedia |

OpenAPI lengkap tersedia pada [`openapi/public.yaml`](../openapi/public.yaml),
sedangkan contoh ringkas dapat dibaca pada menu **Dokumentasi API → Emisell
Gateway** di dashboard.
