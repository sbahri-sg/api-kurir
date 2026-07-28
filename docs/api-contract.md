# Kontrak API Publik

## 1. Tujuan kompatibilitas

Kontrak publik mengikuti pola RajaOngkir agar integrasi Emisell familiar:
origin, destination, weight, courier, serta daftar hasil per layanan. Ini
adalah kompatibilitas bentuk, bukan pass-through mentah. API Kurir tetap
menambahkan metadata sumber, freshness, dan rincian berat.

Base URL:

```text
https://api-kurir.example.com/v1
```

Header:

```http
Authorization: Bearer <api-key>
Content-Type: application/json
X-Request-Id: <uuid-opsional>
Idempotency-Key: <uuid-untuk-request-mutasi>
```

Uang menggunakan integer rupiah. Berat input dan penyimpanan menggunakan gram.
Timestamp menggunakan ISO 8601 UTC.

## 2. Destination

### `GET /destination/domestic-destination`

Mencari master lokasi lokal, bukan memanggil provider ketika customer
mengetik.

Query:

| Field | Wajib | Keterangan |
|---|---:|---|
| `search` | Ya | Nama kota, kecamatan, kelurahan, atau kode pos |
| `limit` | Tidak | Default 20, maksimum 50 |
| `cursor` | Tidak | Cursor halaman berikutnya |

Contoh:

```http
GET /v1/destination/domestic-destination?search=Beji%20Depok
```

```json
{
  "meta": {
    "request_id": "req_01J...",
    "next_cursor": null
  },
  "data": [
    {
      "id": "loc_id_3276010",
      "label": "Beji, Depok, Jawa Barat, 16421",
      "province": "Jawa Barat",
      "city": "Depok",
      "district": "Beji",
      "subdistrict": null,
      "postal_code": "16421",
      "postal_codes": ["16421"]
    }
  ]
}
```

`id` adalah ID stabil milik API Kurir. ID RajaOngkir atau carrier tidak boleh
diekspos sebagai primary ID.

`postal_code` mempertahankan kompatibilitas dengan client lama.
`postal_codes` berisi seluruh kode pos valid dari dataset wilayah/kode pos
lokal yang versinya dicatat saat import. Nilai kosong, `0`, dan `00000` tidak
diterbitkan.

Pencarian customer hanya menerbitkan lokasi level kelurahan/desa
(`subdistrict`). Provinsi, kota, dan kecamatan disimpan sebagai parent
hierarki, tetapi tidak dipakai sebagai destination ID pada metode direct
search RajaOngkir.

## 3. Daftar kurir

### `GET /couriers`

Query opsional:

- `origin`;
- `destination`;
- `service_type`: `parcel`, `cargo`, `same_day`, atau `instant`;
- `active`: default `true`.

Respons memuat `code`, `name`, provider, kemampuan domestik/internasional/
tracking, tanggal verifikasi katalog, layanan lokal aktif, dan mode
kalkulasinya.

```json
{
  "data": [
    {
      "code": "jne",
      "name": "JNE",
      "provider_code": "rajaongkir",
      "supports_domestic_cost": true,
      "supports_international_cost": true,
      "supports_tracking": true,
      "catalog_source": "https://rajaongkir.com/docs/shipping-cost/getting_started/courier_availability",
      "catalog_verified_at": "2026-07-28",
      "services": [
        {
          "code": "REG",
          "name": "Reguler",
          "service_type": "parcel",
          "calculation_mode": "local_rate_card"
        },
        {
          "code": "JTR",
          "name": "JNE Trucking",
          "service_type": "cargo",
          "calculation_mode": "local_rate_card"
        }
      ]
    }
  ]
}
```

## 4. Cek ongkir

### `POST /calculate/domestic-cost`

Request:

```json
{
  "origin": "loc_id_3273010",
  "destination": "loc_id_3276010",
  "weight": 10800,
  "courier": "jne:tiki:sicepat",
  "dimensions": {
    "length": 40,
    "width": 30,
    "height": 25,
    "unit": "cm"
  },
  "item_value": 1500000,
  "options": {
    "include_insurance": false,
    "include_unverified": false
  }
}
```

Field:

| Field | Wajib | Aturan |
|---|---:|---|
| `origin` | Ya | ID lokasi internal |
| `destination` | Ya | ID lokasi internal |
| `weight` | Ya | Berat aktual dari Emisell dalam gram, harus `> 0` |
| `courier` | Ya | Satu atau beberapa kode dipisah `:` |
| `dimensions` | Tidak | Jika diisi, panjang/lebar/tinggi harus lengkap |
| `item_value` | Tidak | Dibutuhkan jika asuransi dihitung |
| `options` | Tidak | Kebijakan tambahan |

Respons:

```json
{
  "meta": {
    "request_id": "req_01J...",
    "calculated_at": "2026-07-28T08:15:30Z",
    "currency": "IDR"
  },
  "data": [
    {
      "courier": {
        "code": "jne",
        "name": "JNE"
      },
      "service": {
        "code": "JTR",
        "name": "JNE Trucking",
        "type": "cargo"
      },
      "cost": 45000,
      "etd": {
        "min_days": 3,
        "max_days": 7,
        "text": "3-7 hari"
      },
      "weight": {
        "actual_grams": 10800,
        "volumetric_grams": 6000,
        "chargeable_grams": 10800,
        "rounded_grams": 11000,
        "billing_grams": 11000,
        "minimum_grams": 10000,
        "rounding_profile": "jne-jtr-public-2026"
      },
      "breakdown": {
        "shipping": 45000,
        "surcharge": 0,
        "insurance": 0,
        "tax": 0,
        "total": 45000
      },
      "source": {
        "type": "local_rate_card",
        "provider": "rajaongkir",
        "verification_status": "official_contract",
        "effective_from": "2026-07-20",
        "fetched_at": "2026-07-20T03:10:00Z",
        "is_stale": false
      }
    }
  ],
  "warnings": []
}
```

Catatan:

- `provider` adalah sumber rate card terakhir; kurir tetap berada pada
  `courier.code`.
- Jika dimensi tidak dikirim, kalkulasi hanya dapat dianggap final untuk
  layanan yang tidak memakai berat volumetrik atau bila Emisell sudah
  memastikan berat input adalah chargeable weight.
- Emisell mengirim berat aktual tanpa pembulatan. API Kurir menerapkan profile
  service; tidak ada toleransi 300 gram global.
- Layanan dengan aturan `needs_contract_confirmation` tidak ditampilkan
  kecuali `include_unverified=true`, dan hasilnya diberi warning.
- Urutan default: total termurah, lalu estimasi tercepat.

### Cache dan miss

Jika rate card aktif tersedia, respons dihitung lokal. Jika belum tersedia:

1. API Kurir mencari snapshot exact-match yang masih berlaku;
2. jika tidak ada, quota credential diperiksa dan dikonsumsi secara atomik;
3. API Kurir mengambil quote dari provider yang berizin;
4. hasil dinormalisasi dan disimpan sebagai snapshot;
5. request berikutnya memakai snapshot exact-match selama TTL.

Quote provider memakai `source.type=provider_quote` dan tidak mengklaim aturan
pembulatan, berat minimum, maupun jenis layanan yang tidak diberikan oleh
provider. Untuk respons ini `service.type` bernilai `unknown`, sedangkan rincian
berat hanya menunjukkan input yang dikirim ke provider. Dimensi tetap masuk
fingerprint snapshot, tetapi endpoint RajaOngkir V2 saat ini hanya menerima
`weight`; karena itu berat yang dikirim Emisell harus sudah aman untuk proses
quote provider.

Status `202` hanya digunakan jika provider lambat dan caller memilih mode
asynchronous. Default tetap mencoba memberi hasil sinkron dalam batas timeout.

## 5. Membaca snapshot tarif otomatis

### `GET /admin/rate-snapshots`

Endpoint admin read-only untuk mengamati hasil quote provider yang telah
disimpan, bukan endpoint publik customer dan bukan form master harga.

Query:

- `search`: kurir, layanan, provider, origin, atau destination;
- `limit`;
- `offset`.

Respons memuat rute, kurir/layanan, berat request, harga total provider, ETD,
waktu pengambilan, kedaluwarsa, dan status freshness. Snapshot tidak dianggap
bukti tarif per kilogram, minimum berat, atau aturan pembulatan.

## 6. Tracking resi

### `POST /track/waybill`

Request:

```json
{
  "waybill": "0123456789012",
  "courier": "jne",
  "refresh": "if_stale"
}
```

Dashboard tidak meminta nomor telepon. `last_phone_number` hanya dipertahankan
sebagai field opsional untuk kompatibilitas; jika dikirim, nilainya dienkripsi,
tidak dikembalikan, dan tidak masuk log.

Nilai `refresh`:

- `if_stale`: default, jadwalkan refresh jika perlu;

Versi v0.4 hanya menerima `if_stale`. Mode `never` dan `force` belum menjadi
kontrak aktif.

Respons:

```json
{
  "meta": {
    "request_id": "req_01J..."
  },
  "data": {
    "waybill": "*********9012",
    "courier": "jne",
    "status": "in_transit",
    "status_label": "Dalam perjalanan",
    "summary": {
      "shipper": "JAKARTA",
      "receiver": "DEPOK",
      "origin": "JAKARTA",
      "destination": "DEPOK"
    },
    "events": [
      {
        "code": "departed_hub",
        "description": "Kiriman diberangkatkan dari hub",
        "location": "JAKARTA",
        "occurred_at": "2026-07-28T04:22:00Z"
      }
    ],
    "provider": "carrier_adapter",
    "provider_fetched_at": "2026-07-28T08:10:00Z",
    "next_refresh_at": "2026-07-28T09:10:00Z",
    "is_final": false,
    "refresh_queued": false,
    "last_error_code": ""
  }
}
```

`last_error_code` hanya muncul ketika refresh terakhir gagal. Nilai ini aman
ditampilkan ke client karena tidak memuat respons mentah atau kredensial
provider.

Status normalisasi:

```text
pending_pickup
picked_up
in_transit
out_for_delivery
delivered
delivery_failed
returned
cancelled
unknown
```

Endpoint tidak melakukan satu hit provider untuk setiap view customer.
Snapshot resi yang sama digunakan ulang; worker memperbarui berdasarkan status
dan `next_refresh_at`. Nomor resi penuh tidak dikembalikan; database menyimpan
hash, masked value, dan ciphertext AES-256-GCM. Jika tracking dinonaktifkan,
endpoint mengembalikan `503 TRACKING_NOT_CONFIGURED`.

## 7. API admin

Endpoint `/v1/admin/*` menggunakan `ADMIN_API_KEYS`, bukan key customer.
Admin dapat membuat customer key tambahan tanpa restart. Key tersebut disimpan
sebagai hash, tidak memakai nama atau masa berlaku, aktif sampai di-revoke, dan
hanya dapat mengakses endpoint `/v1` customer.

| Method | Endpoint | Keterangan |
|---|---|---|
| GET | `/admin/overview` | Metrik dashboard |
| GET | `/admin/catalog` | Kurir dan layanan (read-only) |
| GET | `/admin/locations` | Search master wilayah lokal (read-only) |
| GET | `/admin/couriers` | Kurir untuk alat operasional dashboard |
| POST | `/admin/calculate/domestic-cost` | Cek ongkir dengan admin key |
| POST | `/admin/track/waybill` | Cek resi dengan admin key |
| GET | `/admin/rate-snapshots` | Snapshot tarif otomatis dari provider |
| GET | `/admin/location-mappings` | Mapping lokasi otomatis (read-only) |
| GET | `/admin/provider-credentials` | Metadata key provider termasking |
| POST | `/admin/provider-credentials` | Validasi dan simpan key provider terenkripsi |
| POST | `/admin/provider-credentials/{id}/disable` | Nonaktifkan key provider |
| GET | `/admin/api-keys` | Metadata dan status customer API key |
| POST | `/admin/api-keys` | Generate customer key; secret tampil sekali |
| POST | `/admin/api-keys/{id}/revoke` | Revoke customer key |
| GET | `/admin/provider-quotas` | Monitoring quota tanpa key |

Dashboard tidak menyediakan mutasi tarif atau mapping. Mapping hanya dibuat
oleh resolver lazy setelah kecocokan wilayah provider lolos validasi tepat
satu kandidat. Snapshot tarif hanya dibuat oleh adapter provider setelah quote
berhasil dan disimpan dengan fingerprint request. Key provider baru langsung
masuk rotasi runtime tanpa restart; secret tidak pernah dikembalikan. Endpoint
cek ongkir dan cek resi admin hanya merupakan permukaan operasional untuk
service yang sama, bukan implementasi tarif atau tracking kedua.

## 8. Error

```json
{
  "error": {
    "code": "RATE_NOT_AVAILABLE",
    "message": "Tarif belum tersedia untuk rute dan layanan ini.",
    "request_id": "req_01J...",
    "details": {
      "courier": "jne",
      "service": "JTR"
    }
  }
}
```

Kode utama:

| HTTP | Code | Keterangan |
|---:|---|---|
| 400 | `INVALID_REQUEST` | Payload atau satuan tidak valid |
| 401 | `UNAUTHORIZED` | API key tidak valid |
| 404 | `LOCATION_NOT_FOUND` | Lokasi tidak ditemukan |
| 404 | `WAYBILL_NOT_FOUND` | Resi belum dikenal provider |
| 409 | `IDEMPOTENCY_CONFLICT` | Key dipakai untuk payload berbeda |
| 409 | `PROVIDER_KEY_EXISTS` | Provider key sudah tersimpan |
| 422 | `INVALID_PROVIDER_KEY` | Key provider ditolak saat validasi |
| 422 | `UNSUPPORTED_SERVICE` | Layanan tidak mendukung paket/rute |
| 422 | `RATE_NOT_AVAILABLE` | Rate card/quote tidak tersedia |
| 422 | `PROVIDER_LOCATION_NOT_MAPPED` | Mapping lokasi provider belum tersedia |
| 429 | `TENANT_RATE_LIMITED` | Batas tenant API Kurir |
| 502 | `PROVIDER_AUTHENTICATION_FAILED` | Credential provider ditolak |
| 502 | `PROVIDER_ERROR` | Provider gagal memberi respons valid |
| 503 | `PROVIDER_QUOTA_EXHAUSTED` | Quota sumber yang dibutuhkan habis |
| 503 | `TRACKING_NOT_CONFIGURED` | Tracking belum diaktifkan |

Jangan terus-menerus retry `400`, `404`, dan `422`. Untuk `429`, `502`, dan
`503`, client memakai exponential backoff dan menghormati `Retry-After` jika
header tersebut tersedia.

## 9. Versioning

- Breaking change menggunakan versi path baru, misalnya `/v2`.
- Penambahan field bersifat backward-compatible.
- Kode layanan provider boleh berubah; gunakan `courier + service + version`.
- Deprecation diumumkan minimal 90 hari untuk client internal.
- Raw payload provider disimpan terenkripsi untuk audit dengan retensi
  terbatas, tetapi tidak menjadi kontrak response.
