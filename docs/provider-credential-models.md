# Model credential provider

API Kurir mengirim `credential_type` dan `credential_fields` pada
`GET /api/v1/integrations/providers`. Main Service atau dashboard Emisell tidak
perlu mempunyai form khusus untuk setiap provider; field dirender dari kontrak
ini ketika seller memilih provider yang mewajibkan credential, baik
managed-upstream maupun partner-hosted BYOK seperti RajaOngkir.

| `credential_type` | Field form |
| --- | --- |
| `none` | Tidak ada credential seller |
| `api_key` | `api_key` |
| `capability_api_keys` | `shipping_api_key`, `delivery_api_key` opsional |
| `bearer_token` | `token` |
| `api_key_secret` | `api_key`, `api_secret` |
| `oauth2_client_credentials` | `client_id`, `client_secret` |

Contoh provider OAuth:

```json
{
  "code": "provider-oauth",
  "requires_credential": true,
  "credential_type": "oauth2_client_credentials",
  "credential_fields": [
    {
      "code": "client_id",
      "label": "Client ID",
      "input_type": "text",
      "secret": false,
      "required": true
    },
    {
      "code": "client_secret",
      "label": "Client secret",
      "input_type": "password",
      "secret": true,
      "required": true
    }
  ]
}
```

Dashboard mengirim nilai sesuai kode field:

```json
{
  "provider_code": "provider-oauth",
  "credentials": {
    "client_id": "merchant-client-id",
    "client_secret": "merchant-client-secret"
  }
}
```

Urutan aktivasi merchant:

1. Emisell membaca `GET /api/v1/integrations/providers`.
2. Saat seller memilih provider dengan `requires_credential: true`, dashboard
   merender `credential_fields`. OAuth otomatis menampilkan `client_id` dan
   `client_secret`.
3. Backend Emisell mengirim bundle ke
   `POST /api/v1/integrations/provider-credentials`.
4. Setelah credential valid, backend mengaktifkan provider melalui
   `POST /api/v1/integrations/providers/{provider_code}/activate`.

API Kurir menolak field tambahan, mengenkripsi seluruh bundle menggunakan
AES-256-GCM, dan tidak pernah mengembalikan nilai secret. Provider baru hanya
boleh dibuat tersedia setelah adapter mempunyai validator untuk model
credential tersebut. Perubahan model ditolak jika provider sudah mempunyai
credential atau merchant aktif.

Field `api_key` pada request lama tetap didukung untuk kompatibilitas
RajaOngkir.

## Credential berdasarkan fungsi

RajaOngkir menggunakan `capability_api_keys`. `shipping_api_key` wajib dan
dipakai untuk `rates:read` serta `tracking:read`. `delivery_api_key` bersifat
opsional selama merchant hanya memakai ongkir/tracking; field ini dipakai untuk
`shipments:write`, `pickup:write`, `labels:read`, dan `shipments:cancel` ketika
fitur fulfillment diaktifkan.

```json
{
  "provider_code": "rajaongkir",
  "credentials": {
    "shipping_api_key": "shipping-key-seller",
    "delivery_api_key": "delivery-key-seller"
  }
}
```

Request lama dengan `api_key` otomatis diperlakukan sebagai
`shipping_api_key`. API Kurir tidak menggunakan shipping key untuk operasi
delivery bila `delivery_api_key` belum tersedia.
