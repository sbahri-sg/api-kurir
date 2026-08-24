# Kontrak API Publik

Dokumen ini hanya untuk Emisell/client yang memakai API Kurir. Lifecycle
credential seller berada di [Provider Account API](provider-account-api-v1.md),
sedangkan kontrak vendor/ekspedisi berada di
[Partner Integration Contract](partner-api-v1.md). Public API tidak menerima
atau mengembalikan secret RajaOngkir, KiriminAja, maupun partner lain.

## 1. Tujuan kompatibilitas

Kontrak publik mengikuti pola RajaOngkir agar integrasi Emisell familiar:
origin, destination, weight, courier, serta daftar hasil per layanan. Ini
adalah kompatibilitas bentuk, bukan pass-through mentah. API Kurir tetap
menambahkan metadata sumber, freshness, dan rincian berat.

Base path dan klasifikasi:

| Kontrak | Base path | Pemanggil | ID | Status |
|---|---|---|---|---|
| RajaOngkir V2 compatible | `/api/v1` | Emisell/SDK RajaOngkir | Integer snapshot RajaOngkir | Public, stable |
| Emisell Legacy | `/regions`, `/shipping` | Modul region-service lama | Integer dump region-service | Legacy compatibility |
| Canonical/Internal | `/v1` | Dashboard dan service internal | `loc_idn_*` | Internal, stable |
| Emisell Gateway | `/api/v1/integrations` | Backend Emisell bertenant | Merchant ID dari signed token | Internal, stable |
| Admin | `/v1/admin` | Operator API Kurir | Canonical ID/UUID | Restricted |

Partner Connector tidak berada pada base path di atas. Vendor menyediakan
`/partner/v1` pada host miliknya sendiri sesuai
[Partner Integration Contract](partner-api-v1.md).

Header:

```http
Authorization: Bearer <api-key>
# atau
key: <api-key>
Content-Type: application/json
X-Request-Id: <uuid-opsional>
Idempotency-Key: <uuid-untuk-request-mutasi>
```

Uang menggunakan integer rupiah. Berat input dan penyimpanan menggunakan gram.
Timestamp menggunakan ISO 8601 UTC.

### Mode kompatibilitas RajaOngkir V2

Kontrak ditentukan oleh base path, bukan oleh bentuk header autentikasi:

- `/api/v1` selalu memberikan kontrak 1:1 RajaOngkir V2;
- `/v1` selalu memberikan kontrak canonical internal API Kurir;
- `/regions` dan `/shipping` selalu memberikan kontrak region-service lama.

Semua path customer menerima `Authorization: Bearer <api-key>` maupun
`key: <api-key>` dengan hasil yang sama. Pada `/api/v1`, ID lokasi diterbitkan
sebagai integer dari snapshot
`region-service-main` RajaOngkir dan request kalkulasi memakai
`application/x-www-form-urlencoded` atau `multipart/form-data`; payload JSON
ditolak dengan HTTP `415`. Client harus selalu mengambil ID dari
endpoint API Kurir karena namespace ID berlaku per level. Sebagai contoh,
provinsi `32` adalah Maluku Utara pada snapshot RajaOngkir dan bukan kode
Kemendagri Jawa Barat. Path `/api/v1` sama dengan base path resmi RajaOngkir
V2. Path `/v1` tetap tersedia untuk dashboard dan client internal dengan
public ID canonical `loc_idn_*`. Kode Kemendagri hanya menjadi metadata
internal untuk pencocokan lintas provider dan tidak menjadi ID yang harus
dikirim SDK Emisell.

## 2. Destination

### `GET /destination/domestic-destination`

Mencari master lokasi lokal, bukan memanggil provider ketika customer
mengetik.

Query:

| Field | Wajib | Keterangan |
|---|---:|---|
| `search` | Ya | Nama wilayah, kode pos, atau alamat mentah; awalan dan tanda baca dinormalisasi |
| `limit` | Tidak | Default 20; maksimum 50 pada `/v1` atau 1.000 pada `/api/v1` |
| `offset` | Tidak | Offset hasil, default 0 |

Contoh:

```http
GET /api/v1/destination/domestic-destination?search=Beji%20Depok
key: <api-key>
```

```json
{
  "meta": {
    "message": "Success Get Domestic Destinations",
    "code": 200,
    "status": "success"
  },
  "data": [
    {
      "id": 25976,
      "label": "Beji, Beji, Kota Depok, Jawa Barat, 16421",
      "province_name": "Jawa Barat",
      "city_name": "Kota Depok",
      "district_name": "Beji",
      "subdistrict_name": "Beji",
      "zip_code": "16421"
    }
  ]
}
```

`meta.message`, `meta.code`, `meta.status`, `province_name`, `city_name`,
`district_name`, `subdistrict_name`, dan `zip_code` mengikuti bentuk respons
RajaOngkir V2. Pada path `/api/v1`, `id` adalah ID snapshot RajaOngkir untuk
lokasi akhir. Pada path `/v1`, field hierarki memakai public ID canonical dan
`postal_codes` berisi seluruh kode pos valid. Nilai kosong, `0`, dan `00000`
tidak diterbitkan.

Search menormalisasi alamat mentah: tanda baca dipisahkan, kata administratif
seperti `jl`, `jalan`, `kec`, `kecamatan`, `kel`, `kelurahan`, `kab`, dan
`provinsi` diabaikan, token duplikat dibuang, dan kode pos lima digit mendapat
sinyal pencocokan tersendiri. Hasil diranking dari jumlah token wilayah yang
cocok, lalu level kelurahan/kecamatan/kota/provinsi.

Pencarian customer hanya menerbitkan lokasi level kelurahan/desa
(`subdistrict`). Provinsi, kota, dan kecamatan disimpan sebagai parent
hierarki, tetapi tidak dipakai sebagai destination ID pada metode direct
search RajaOngkir.

Nama wilayah canonical disimpan dalam format tampilan yang wajar, misalnya
`Jawa Timur`, `Bandung`, dan `Husen Sastranegara`. Singkatan resmi seperti
`DKI`, `DI`, `NTB`, `NTT`, `NAD`, serta angka Romawi tetap dipertahankan.
Normalisasi ini hanya mengubah kapitalisasi nama; ID, hierarki, kode pos, dan
mapping provider tidak berubah. Label mentah provider tetap disimpan pada
`provider_location_mappings` untuk audit.

### Endpoint hierarki RajaOngkir V2

Untuk SDK atau form cascading, API juga menyediakan pola endpoint bertingkat:

| Level | Endpoint | Parent |
|---|---|---|
| Provinsi | `GET /destination/province` | - |
| Kota/kabupaten | `GET /destination/city/{province_id}` | ID provinsi lokal |
| Kecamatan | `GET /destination/district/{city_id}` | ID kota lokal |
| Kelurahan/desa | `GET /destination/sub-district/{district_id}` | ID kecamatan lokal |

Contoh:

```json
{
  "meta": {
    "message": "Success Get City By Province ID",
    "code": 200,
    "status": "success"
  },
  "data": [
    {
      "id": 55,
      "name": "Bandung",
      "zip_code": ""
    }
  ]
}
```

Seluruh endpoint ini membaca dump lokal dan tidak memakai hit RajaOngkir. ID
parent harus diambil dari endpoint level sebelumnya; ID kelurahan/desa dapat
dipakai sebagai `origin` atau `destination` pada endpoint direct search.

Pada path `/api/v1`, bentuk respons endpoint hierarki mengikuti RajaOngkir V2
untuk header `key` maupun Bearer:

```json
{
  "meta": {
    "message": "Success Get District By City ID",
    "code": 200,
    "status": "success"
  },
  "data": [
    {
      "id": 442,
      "name": "Cicendo",
      "zip_code": ""
    }
  ]
}
```

### Façade region-service Emisell

Untuk mengganti service Node lama tanpa mengubah form alamat Emisell, API
Kurir juga menyediakan kontrak berikut:

| Endpoint | Fungsi |
|---|---|
| `GET /regions/provinces` | Daftar provinsi ID lama |
| `GET /regions/provinces/{id}` | Detail provinsi |
| `GET /regions/cities?provinceId={id}` | Kota per provinsi |
| `GET /regions/cities/{id}` | Detail kota |
| `GET /regions/districts?cityId={id}` | Kecamatan per kota |
| `GET /regions/districts/{id}` | Detail kecamatan dan parent |
| `GET /regions/subdistricts?districtId={id}` | Kelurahan dan kode pos |
| `GET /shipping/domestic-cost` | Cek ongkir GET untuk Emisell lama |

Route ini memakai header `key` atau Bearer customer API key. Bentuk `data`
dan nama field camelCase dipertahankan seperti `region-service-main`, termasuk
`serviceName` pada respons ongkir untuk pengelompokan `REG`, `EXPRESS`,
`TRUCKING`, `CARGO`, `MOTOR`, dan kategori lainnya.

## 3. Daftar kurir

### `GET /couriers`

Query opsional:

- `origin`;
- `destination`;
- `service_type`: `parcel`, `cargo`, `same_day`, atau `instant`;
- `active`: default `true`.

Respons memuat `code`, `name`, URL `logo` permanen, provider utama, `rate_provider_code`,
`tracking_provider_code`, kemampuan domestik/internasional/tracking, tanggal
verifikasi katalog, layanan lokal aktif, dan mode kalkulasinya. Nilai provider
dipisahkan agar Biteship fallback tracking tidak pernah dianggap sebagai
sumber cek ongkir.

Layanan baru yang dikembalikan provider akan diklasifikasikan dan dimasukkan
ke master `courier_services` secara otomatis bila grup serta tipe servicenya
dapat dikenali. Respons yang belum dapat diklasifikasikan tetap dicatat pada
alias/snapshot untuk audit, tetapi tidak langsung dipublikasikan ke seller.

```json
{
  "data": [
    {
      "code": "jne",
      "name": "JNE",
      "logo": "https://api-kurir.emisell.com/courier-logos/jne.webp",
      "provider_code": "rajaongkir",
      "rate_provider_code": "rajaongkir",
      "tracking_provider_code": "rajaongkir",
      "supports_domestic_cost": true,
      "supports_international_cost": true,
      "supports_tracking": true,
      "catalog_source": "https://rajaongkir.com/docs/shipping-cost/getting_started/courier_availability",
      "tracking_catalog_source": "https://www.rajaongkir.com/docs/shipping-cost/getting_started/courier_availability",
      "catalog_verified_at": "2026-07-28",
      "services": [
        {
          "code": "REG",
          "name": "Reguler",
          "group": "regular",
          "service_type": "parcel",
          "calculation_mode": "local_rate_card"
        },
        {
          "code": "JTR",
          "name": "JNE Trucking",
          "group": "cargo",
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

Mode SDK RajaOngkir V2:

```http
POST /api/v1/calculate/domestic-cost
key: <api-key>
Content-Type: application/x-www-form-urlencoded

origin=4911&destination=25976&weight=1000&courier=jne&price=lowest&include_group=true
```

Form `multipart/form-data` juga diterima dengan field yang sama. Payload JSON
tidak termasuk kontrak RajaOngkir V2 dan memperoleh HTTP `415`. Parameter
`price=lowest` mengurutkan seluruh layanan dari termurah ke termahal;
`price=highest` mengurutkan seluruh layanan dari termahal ke termurah. Parameter
ini tidak memfilter hasil menjadi satu layanan.

Parameter extension `include_group=true` menambahkan `canonical_service`,
`service_group`, dan `service_type` pada setiap opsi tarif. Tanpa parameter ini
respons tetap 1:1 dengan field RajaOngkir V2. Field `service` selalu menyimpan
kode mentah provider, sedangkan `canonical_service` menormalkan varian seperti
`JTR>130` menjadi `JTR` agar Emisell mudah melakukan pengelompokan.

Khusus pemanggilan Main Service yang membawa `X-Emisell-Merchant-ID`, field
`courier` tidak dikirim. API Kurir mengambil kurir dari konfigurasi service
merchant dan mengabaikan nilai `courier` dari client lama. Pemanggilan public
tanpa Merchant ID tetap wajib mengirim `courier` seperti kontrak RajaOngkir V2.

Kalkulasi berdasarkan kecamatan tersedia pada:

```http
POST /api/v1/calculate/district/domestic-cost
```

Endpoint district memakai ID dari
`GET /api/v1/destination/district/{city_id}` dan meneruskan quote miss ke endpoint
district resmi provider. Mode SDK mengembalikan field flat
`name`, `code`, `service`, `description`, `cost`, dan `etd`, serta tiga field
pengelompokan opsional saat `include_group=true`.

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
| `weight` | Ya | Berat final/chargeable dari Emisell dalam gram, harus `> 0` |
| `courier` | Ya | Satu atau beberapa kode dipisah `:` |
| `dimensions` | Tidak | Field kompatibilitas; eligibility checkout tetap memakai `weight` final |
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
        "code": "JTR>130",
        "name": "JNE Trucking",
        "canonical_code": "JTR",
        "group": "cargo",
        "type": "cargo",
        "variant_code": "JTR>130"
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
        "minimum_accepted_grams": 10000,
        "minimum_billable_grams": 10000,
        "maximum_accepted_grams": null,
        "rounding_profile": "jne-jtr-public-2026"
      },
      "eligibility": {
        "eligible": true,
        "weight_basis": "provided",
        "evaluated_weight_grams": 10800,
        "minimum_accepted_weight_grams": 10000,
        "minimum_billable_weight_grams": 10000,
        "maximum_accepted_weight_grams": null,
        "source_type": "official_public",
        "verification_status": "official_public",
        "verified_at": "2026-08-20"
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
- `service.code` selalu mempertahankan kode mentah dari provider. Gunakan
  `canonical_code` untuk pengelompokan bisnis, `group` untuk kelas layanan,
  dan `variant_code` untuk varian provider seperti `REG23`, `CTCYES`, atau
  `JTR>130`.
- Jika layanan baru belum dikenali, API tidak menebak: `group` dan `type`
  bernilai `unknown`, kode mentah tetap dikembalikan, dan alias observasi
  disimpan untuk ditinjau.
- Emisell mengirim berat final/chargeable tanpa pembulatan provider. API Kurir
  menerapkan minimum penerimaan, minimum tagihan, maksimum, dan profile
  pembulatan service; tidak ada toleransi 300 gram global.
- Layanan yang tidak memenuhi policy berat tidak dimasukkan ke `data`. Jika
  seluruh layanan gagal, response menjadi `RATE_NOT_AVAILABLE`.
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

### Filter layanan checkout per merchant

Sebelum memilih kurir dan service, backend Emisell dapat mengelola extension
provider merchant melalui:

| Method | Endpoint | Scope |
|---|---|---|
| GET | `/api/v1/integrations/providers` | `shipping:read` |
| POST | `/api/v1/integrations/providers/{provider_code}/activate` | `shipping:write` |
| POST | `/api/v1/integrations/providers/{provider_code}/deactivate` | `shipping:write` |

Merchant dapat mempunyai beberapa provider terpasang, tetapi maksimal satu
provider efektif aktif. Tenant baru tidak mempunyai provider aktif; seller harus
mengaktifkan Emisell Kurir atau provider eksternal. Saat tidak aktif, endpoint
rate bertenant mengembalikan `SHIPPING_DISABLED`. Detail state, optimistic
concurrency, dan lifecycle credential dijelaskan pada
[`merchant-shipping-providers.md`](merchant-shipping-providers.md).

Setiap item katalog provider membawa `logo` berupa URL HTTPS permanen dan
`description` berupa teks biasa. Metadata presentasi tersebut dikelola satu kali
di master API Kurir dan tidak diduplikasiasikan per merchant.

Request bertenant dapat mengatur layanan yang boleh muncul melalui
`GET/PUT /api/v1/integrations/shipping-services`. Rate engine menormalisasi kode
mentah provider ke canonical service terlebih dahulu, lalu menerapkan pilihan
merchant tanpa mengubah envelope RajaOngkir V2. Merchant tanpa preference
mulai dari nol layanan dan harus menyimpan pilihan custom terlebih dahulu.

Kontrak request custom-only, limit kurir/layanan, scope, error, dan pola
checkbox kurir bertingkat tersedia pada
[`merchant-shipping-services.md`](merchant-shipping-services.md).

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

### Mode SDK RajaOngkir V2 — `POST /api/v1/track/waybill`

Gunakan header customer API key:

```http
key: <customer-api-key>
```

Parameter dapat dikirim melalui query string seperti SDK RajaOngkir V2:

```http
POST /api/v1/track/waybill?awb=MT685U91&courier=wahana
```

atau sebagai `application/x-www-form-urlencoded`:

```text
awb=MT685U91&courier=wahana
```

`last_phone_number` bersifat opsional dan berisi lima digit terakhir nomor
telepon penerima. Parameter ini hanya perlu dikirim apabila ekspedisi/provider
meminta validasi tambahan. API Kurir tidak mewajibkannya untuk pengecekan resi
normal.

Respons sukses mengikuti kontrak RajaOngkir V2:

```json
{
  "meta": {
    "message": "Success Tracking AWB",
    "code": 200,
    "status": "success"
  },
  "data": {
    "delivered": true,
    "summary": {
      "courier_code": "wahana",
      "courier_name": "Wahana Prestasi Logistik",
      "waybill_number": "MT685U91",
      "service_code": "",
      "waybill_date": "2024-10-09",
      "shipper_name": "",
      "receiver_name": "FIKRI EL SARA",
      "origin": "JAKARTA",
      "destination": "SUKABUMI",
      "status": "DELIVERED"
    },
    "details": {
      "waybill_number": "MT685U91",
      "waybill_date": "2024-10-09",
      "waybill_time": "",
      "weight": "",
      "origin": "JAKARTA",
      "destination": "SUKABUMI",
      "shipper_name": "",
      "shipper_address1": "",
      "shipper_address2": "",
      "shipper_address3": "",
      "shipper_city": "",
      "receiver_name": "FIKRI EL SARA",
      "receiver_address1": "",
      "receiver_address2": "",
      "receiver_address3": "",
      "receiver_city": ""
    },
    "delivery_status": {
      "status": "DELIVERED",
      "pod_receiver": "FIKRI EL SARA",
      "pod_date": "2024-10-11",
      "pod_time": "09:26:00"
    },
    "manifest": [
      {
        "manifest_code": "",
        "manifest_description": "Diterima oleh FIKRI EL SARA (Penerima Langsung)",
        "manifest_date": "2024-10-11",
        "manifest_time": "09:26:00",
        "city_name": "SUKABUMI"
      }
    ]
  }
}
```

Respons error juga memakai envelope RajaOngkir `meta` dan `data: null`.
Request pertama untuk resi yang belum tersimpan akan mengambil data provider
dan menyimpan snapshot. Request berikutnya memakai snapshot selama masih
fresh atau status sudah final. Request bersamaan untuk resi yang sama pada
proses API yang sama digabung menjadi satu hit provider.

### Mode API Kurir lama — `POST /v1/track/waybill`

Mode ini tetap tersedia untuk dashboard dan integrasi lama. Autentikasi
menggunakan `Authorization: Bearer <customer-api-key>` dan responsnya
asynchronous.

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
    "waybill": "0123456789012",
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
    "next_refresh_at": "2026-07-28T20:10:00Z",
    "is_final": false,
    "refresh_queued": false,
    "last_error_code": "",
    "validation_status": "valid",
    "provider_hit_count": 3,
    "provider_hit_limit": 10,
    "polling_stopped": false
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

Untuk fulfillment Emisell gunakan endpoint tenant-aware berikut agar request
checkout/customer hanya membaca snapshot dan tidak memicu provider:

| Method | Endpoint | Konteks merchant |
|---|---|---|
| POST | `/api/v1/tracking/verify` | Header merchant opsional |
| POST | `/api/v1/integrations/tracking/subscriptions` | Header merchant wajib |
| GET | `/api/v1/integrations/tracking/subscriptions/{fulfillment_id}` | Header merchant wajib |
| PUT | `/api/v1/integrations/tracking/subscriptions/{fulfillment_id}` | Header merchant wajib |
| DELETE | `/api/v1/integrations/tracking/subscriptions/{fulfillment_id}` | Header merchant wajib; hapus relasi tanpa membuang snapshot |

Kebijakan checkpoint, negative cache AWB, batas hit, dan webhook HMAC dijelaskan
pada [`tracking-checkpoint-and-webhooks.md`](tracking-checkpoint-and-webhooks.md).

## 7. API admin

Endpoint `/v1/admin/*` menggunakan `ADMIN_API_KEYS`. Admin dapat membuat dua
jenis key tanpa restart: `public` untuk API ongkir/tracking dan `main_service`
untuk backend Emisell Gateway. Keduanya disimpan sebagai hash, tidak memakai
nama atau masa berlaku, serta aktif sampai di-revoke. Hanya `main_service` yang
mendapat scope `gateway:access` dan boleh membawa `X-Emisell-Merchant-ID`.

| Method | Endpoint | Keterangan |
|---|---|---|
| GET | `/admin/overview` | Metrik dashboard |
| GET | `/admin/catalog` | Kurir dan layanan (read-only) |
| GET | `/admin/locations` | Search master wilayah lokal (read-only) |
| GET | `/admin/couriers` | Kurir untuk alat operasional dashboard |
| POST | `/admin/calculate/domestic-cost` | Cek ongkir dengan admin key |
| POST | `/admin/track/waybill` | Cek resi dengan admin key |
| GET | `/admin/tracking-operations` | Monitor AWB, revision, snapshot, dan antrean worker |
| DELETE | `/admin/tracking-operations/{id}` | Hapus permanen shipment tracking dan data turunannya |
| GET | `/admin/rate-snapshots` | Snapshot tarif otomatis dari provider |
| GET | `/admin/location-mappings` | Mapping lokasi otomatis (read-only) |
| GET | `/admin/provider-credentials` | Metadata key provider termasking |
| POST | `/admin/provider-credentials` | Validasi dan simpan key provider terenkripsi |
| POST | `/admin/provider-credentials/{id}/disable` | Nonaktifkan key provider |
| GET | `/admin/api-keys` | Metadata dan status customer API key |
| POST | `/admin/api-keys` | Generate public/main_service key; secret tampil sekali |
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
| 429 | `PROVIDER_RATE_LIMITED` | Provider membatasi request sementara |
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
