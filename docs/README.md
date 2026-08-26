# Pusat Dokumentasi API Kurir

Halaman ini adalah pintu masuk dokumentasi teknis API Kurir. Pilih dokumen
berdasarkan siapa yang melakukan request. Jangan memilih kontrak berdasarkan
provider yang sedang aktif atau jenis header autentikasi.

## Pilih kontrak

| Kontrak | Pemanggil | Base path | Format ID | Status | Spesifikasi |
|---|---|---|---|---|---|
| RajaOngkir V2 compatible | Emisell/SDK RajaOngkir | `/api/v1` | Integer snapshot RajaOngkir lokal | Stable | [`openapi/public.yaml`](../openapi/public.yaml) |
| Emisell Legacy | Modul region-service lama | `/regions`, `/shipping` | Integer dump region-service | Compatibility | [`openapi/public.yaml`](../openapi/public.yaml) |
| Pilihan layanan merchant | Extension Kurir Emisell | `/api/v1/integrations/shipping-services` | `courier_code:canonical_service_code` | Stable | [`merchant-shipping-services.md`](merchant-shipping-services.md) |
| Fulfillment merchant | Main Service Emisell | `/api/v1/integrations/shipments*` | UUID API Kurir + ID provider terpisah | Beta | [`emisell-fulfillment-gateway.md`](emisell-fulfillment-gateway.md) |
| Provider aktif merchant | Backend Extension Kurir Emisell | `/api/v1/integrations/providers` | `merchant_id` + `provider_code` | Stable | [`merchant-shipping-providers.md`](merchant-shipping-providers.md) |
| Canonical/Internal | Dashboard dan service internal | `/v1` | `loc_idn_*` | Internal | [Kontrak API publik](api-contract.md) |
| Emisell Merchant Gateway | Backend Emisell | `/api/v1` | `merchant_id` + UUID credential | Internal | [Merchant Gateway & RajaOngkir BYOK](emisell-merchant-gateway.md) |
| Tracking fulfillment | Backend Emisell | `/api/v1/integrations/tracking` | `merchant_id` + `fulfillment_id` | Internal | [Checkpoint tracking dan webhook](tracking-checkpoint-and-webhooks.md) |
| Admin | Operator API Kurir | `/v1/admin` | Canonical ID dan UUID | Restricted | [`openapi/public.yaml`](../openapi/public.yaml) |
| Partner Connector | API Kurir ke sistem vendor | `/partner/v1` pada host partner | String canonical partner | Draft | [`openapi/partner-v1.yaml`](../openapi/partner-v1.yaml) |

Header `key` dan `Authorization: Bearer` hanya mengautentikasi customer.
Keduanya menghasilkan struktur yang sama pada path yang sama. ID dari satu
kontrak tidak boleh dikirim ke kontrak lain.

## Integrasi Emisell

Mulai dari dokumen berikut:

1. [Kontrak API publik](api-contract.md) untuk lokasi, ongkir, dan tracking.
2. [Integrasi RajaOngkir V2](rajaongkir-integration.md) untuk kompatibilitas SDK,
   quota, snapshot, dan mapping legacy.
3. [Katalog ekspedisi dan layanan](provider-service-catalog.md) untuk kode kurir,
   layanan, dan kelompok canonical.
4. [Aset logo ekspedisi](courier-logo-assets.md) untuk URL logo stabil dan
   provenance aset yang dirender Emisell.
5. [Model tarif dan aturan berat](rate-and-weight-engine.md) serta
   [matriks pembulatan berat](weight-rounding-matrix.md).
6. [Kelayakan berat layanan checkout](service-weight-eligibility.md) untuk
   minimum diterima, minimum tagihan, maksimum, dan filter cargo.
7. [Merchant Gateway & RajaOngkir BYOK](emisell-merchant-gateway.md) untuk
   tenant authentication, credential seller, kuota, dan multi-domain.
8. [Checkpoint tracking dan webhook](tracking-checkpoint-and-webhooks.md) untuk
   scheduler hemat, validasi AWB, snapshot, dan update fulfillment otomatis.
9. [Strategi provider fulfillment](fulfillment-provider-landscape.md) untuk
   booking, pickup, label, COD, saldo, fallback transaksi, dan perbandingan
   RajaOngkir, Biteship, KiriminAja, Lincah, serta Mengantar.
10. [Model credential provider](provider-credential-models.md) untuk form
    dinamis API key, Bearer, key-secret, dan OAuth milik seller.
11. [Emisell Fulfillment Gateway](emisell-fulfillment-gateway.md) untuk create
    shipment, pickup, label, cancel, registrasi tracking otomatis,
    rekonsiliasi, webhook, dan monitor lifecycle.

Postman Collection pada dashboard memakai placeholder. Isi nilai secret hanya
pada Postman Environment lokal, bukan pada Collection atau repository.

## Integrasi vendor/partner

Vendor baru tidak memakai endpoint customer dan tidak meminta API Kurir
membuat adapter native. Vendor menyediakan Partner Connector yang dipanggil
API Kurir.

1. [Strategi provider fulfillment](fulfillment-provider-landscape.md) untuk
   model built-in dan Certified Partner Connector.
2. [Partner Integration Contract v1](partner-api-v1.md).
3. [Partner Event Webhook v1](partner-webhooks-v1.md).
4. [Keamanan dan request signing](security-and-signing.md).
5. [Sertifikasi partner](partner-certification.md).
6. [Partner Visual API Explorer](partner-visual-api-explorer.md).
7. [Partner Portal dan publikasi extension](partner-portal.md).
8. [Partner Integration Package](partner-integration-packages.md) untuk format
   ZIP, static validation, lifecycle versi, dan console review internal.
9. [Provider Account API](provider-account-api-v1.md) untuk aktivasi koneksi
   seller setelah partner tersedia.

Status `Draft` atau `contract-first` berarti spesifikasi target belum otomatis
tersedia di production. Partner harus lulus sertifikasi sebelum extension
dipublikasikan.

## Operasional dan arsitektur

- [Arsitektur sistem](architecture.md)
- [Peta kontrak dan arah komunikasi](api-surface-map.md)
- [Dashboard admin, tracking, dan manajemen webhook](admin-dashboard-and-tracking.md)
- [Operasional adapter tracking RajaOngkir](rajaongkir-tracking.md)
- [Checkpoint tracking hemat dan webhook Emisell](tracking-checkpoint-and-webhooks.md)
- [Biteship sebagai fallback Emisell Kurir](biteship-tracking-fallback.md)
- [Strategi provider fulfillment dan pickup](fulfillment-provider-landscape.md)
- [Operasional, kuota, dan compliance](operations-and-compliance.md)
- [Stack teknologi dan concurrency](technology-stack.md)
- [Register sumber resmi](source-register.md)

## Aturan dokumentasi

- OpenAPI adalah sumber kebenaran bentuk request dan response.
- Dokumen Markdown menjelaskan arsitektur, kebijakan, dan keputusan operasional.
- Dashboard menyediakan contoh ringkas dan Postman untuk pengujian.
- Contoh wajib memakai `{{api_key}}`, `<partner-access-token>`, atau placeholder
  lain; API key aktif, ciphertext, dan secret provider dilarang dicantumkan.
- Uang memakai integer rupiah, berat memakai integer gram, dan timestamp memakai
  ISO 8601/RFC 3339 UTC.
- Perubahan breaking wajib memakai major version atau base path baru.
