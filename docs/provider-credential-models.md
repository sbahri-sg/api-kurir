# Model credential provider

API Kurir mengirim `credential_type` dan `credential_fields` pada
`GET /api/v1/integrations/providers/{provider_code}`. Main Service atau dashboard Emisell tidak
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
| `provider_declared` | Field aman dari manifest release provider |

Untuk provider package, API Kurir juga mengirim `credential_source`,
`environments`, dan `capability_policies`. Emisell tidak menebak model
autentikasi maupun ketersediaan sandbox. Provider mendeklarasikannya, validator
API Kurir membatasi field pada `text`, `password`, `select`, atau `checkbox`,
dan dashboard hanya merender kontrak yang sudah dipublikasikan.

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
  "environment": "live",
  "credentials": {
    "client_id": "merchant-client-id",
    "client_secret": "merchant-client-secret"
  }
}
```

Urutan aktivasi merchant:

1. Emisell membaca `GET /api/v1/integrations/providers` untuk listing ringkas.
2. Saat seller membuka satu provider, Emisell membaca
   `GET /api/v1/integrations/providers/{provider_code}`.
3. Saat seller memilih provider dengan `requires_credential: true`, dashboard
   merender `credential_fields` untuk environment yang dipilih. OAuth otomatis
   menampilkan `client_id` dan `client_secret`.
4. Backend Emisell mengirim bundle ke
   `POST /api/v1/integrations/provider-credentials`.
5. Setelah credential valid, backend mengaktifkan provider melalui
   `POST /api/v1/integrations/providers/{provider_code}/activate`.

API Kurir menolak field tambahan, mengenkripsi seluruh bundle menggunakan
AES-256-GCM, dan tidak pernah mengembalikan nilai secret. Provider package
menjadi tersedia setelah release dipublikasikan. Credential yang dapat
diverifikasi lewat endpoint baca diuji saat penyimpanan; credential transaksi
yang tidak mempunyai endpoint validasi aman diuji pada pemakaian capability
pertama.

Field `api_key` pada request lama tetap didukung untuk kompatibilitas
RajaOngkir.

Jika credential opsional ditambahkan setelah instalasi, gunakan PATCH agar
field lama tidak hilang:

```json
{
  "environment": "live",
  "credentials": {
    "delivery_api_key": "delivery-key-seller"
  }
}
```

Request tersebut dikirim ke
`PATCH /api/v1/integrations/provider-credentials/{provider_code}`. Kesiapan
auto-pickup dan nama field yang sudah tersedia dibaca kembali melalui endpoint
detail provider. Nilai secret tidak pernah dikembalikan.

## Credential berdasarkan fungsi

Package hosted RajaOngkir mendeklarasikan model `provider_declared` dengan pola
field per capability. `shipping_api_key` wajib dan dipakai untuk `rates:read`
serta `tracking:read`. `delivery_api_key` bersifat
opsional selama merchant hanya memakai ongkir/tracking; field ini dipakai untuk
`shipments:write`, `pickup:write`, `labels:read`, dan `shipments:cancel` ketika
fitur fulfillment diaktifkan.

```json
{
  "provider_code": "rajaongkir",
  "environment": "live",
  "credentials": {
    "shipping_api_key": "shipping-key-seller",
    "delivery_api_key": "delivery-live-key-seller"
  }
}
```

Jika merchant juga memakai fulfillment sandbox, kirim request kedua. Jangan
menggabungkan key sandbox ke bundle live:

```json
{
  "provider_code": "rajaongkir",
  "environment": "sandbox",
  "credentials": {
    "delivery_api_key": "delivery-sandbox-key-seller"
  }
}
```

Request lama dengan `api_key` otomatis diperlakukan sebagai
`shipping_api_key`. API Kurir tidak menggunakan shipping key untuk operasi
delivery bila `delivery_api_key` belum tersedia.

## Environment dan biaya provider

Header internal `X-Emisell-Execution-Mode` menerima `live` atau `sandbox` dan
default-nya `live`, sehingga integrasi Emisell lama tetap kompatibel. Credential
live dan sandbox disimpan terpisah. Memindahkan mode tidak pernah menyalin
secret dari environment lain. Field `environment` yang tidak dikirim selalu
berarti `live`; satu request hanya mengonfigurasi satu environment.

| Capability RajaOngkir | Mode | Perilaku | Credential | Implikasi |
|---|---|---|---|---|
| rates/tracking | live | live | live | memakai Shipping Cost dan kuota provider |
| rates/tracking | sandbox | live_read_only | live | tetap live; dapat mengurangi kuota provider |
| shipments/pickup | live | live | live | Shipping Delivery production |
| shipments/pickup | sandbox | provider_sandbox | sandbox | Shipping Delivery sandbox |

Dengan model ini, KiriminAja, Biteship, Mengantar, dan provider berikutnya dapat
mendeklarasikan kebijakan berbeda tanpa perubahan form khusus di Emisell.
