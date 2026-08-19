# Pilihan Layanan Checkout per Merchant

Dokumen ini menjadi kontrak implementasi Extension Kurir Emisell untuk memilih
kurir dan layanan yang boleh muncul di checkout. Pola UI mengikuti selector
kurir bertingkat: satu checkbox induk per kurir dan checkbox anak untuk setiap
layanan canonical.

## 1. Kepemilikan konfigurasi

- Preferensi dimiliki `merchant_id` Emisell dari claim `sub` tenant token.
- Browser, body, query, dan domain tidak boleh menentukan `merchant_id`.
- Semua domain dalam merchant yang sama memakai preferensi yang sama.
- Credential/provider account tetap dikelola terpisah. Seller memilih layanan
  seperti `JNE REG`, bukan memilih snapshot atau kode mentah provider.
- Kunci layanan stabil adalah `courier_code + canonical service_code`, misalnya
  `jne:REG`, `jne:YES`, dan `jne:SPS`.

## 2. Grup canonical

| Grup | Contoh |
|---|---|
| `economy` | JNE OKE, TIKI ECO |
| `regular` | JNE REG, J&T EZ, TIKI REG |
| `next_day` | JNE YES, TIKI ONS |
| `express` | JNE Super Speed, J&T SUPER |
| `same_day` | TIKI SDS, Pos Same Day |
| `instant` | layanan instant yang tersedia melalui provider aktif |
| `cargo` | JNE JTR, TIKI TRC, SiCepat GOKIL |
| `international` | layanan internasional |
| `special` | dangerous goods, valuable goods, atau layanan khusus lain |

`unknown` tidak dapat diaktifkan oleh konfigurasi baru. Kode provider baru harus
masuk katalog canonical terlebih dahulu. Kode mentah tetap disimpan untuk audit
dan proses klasifikasi otomatis.

## 3. Mode pilihan

### `custom` — direkomendasikan

Seller memilih setiap layanan secara eksplisit. Layanan baru hasil sinkronisasi
provider tidak otomatis tampil di checkout.

```json
{
  "mode": "custom",
  "enabled_groups": [],
  "services": [
    { "courier_code": "jne", "service_code": "REG" },
    { "courier_code": "jne", "service_code": "YES" },
    { "courier_code": "jnt", "service_code": "EZ" }
  ]
}
```

### `groups`

Semua layanan dalam kelompok terpilih diaktifkan. Layanan baru yang kemudian
masuk kelompok tersebut ikut aktif, sehingga mode ini harus menjadi pilihan
sadar seller.

```json
{
  "mode": "groups",
  "enabled_groups": ["regular", "next_day"],
  "services": []
}
```

### `all`

Semua layanan canonical yang sudah dikenali aktif. Layanan `unknown` tetap
ditahan.

```json
{
  "mode": "all",
  "enabled_groups": [],
  "services": []
}
```

Merchant lama yang belum pernah memanggil endpoint PUT mempunyai
`configured=false`. API mempertahankan perilaku allow-all lama agar rollout
tidak memutus checkout. Setelah konfigurasi pertama disimpan, filter baru mulai
berlaku.

## 4. Autentikasi

Semua endpoint memakai dua header:

```http
key: <customer-api-key>
X-Emisell-Tenant-Token: <jwt-eddsa>
```

Scope:

| Scope | Operasi |
|---|---|
| `shipping:read` | membaca katalog/pilihan dan mengambil tarif |
| `shipping:write` | mengganti pilihan layanan checkout |

Tenant token harus berumur pendek dan ditandatangani backend Emisell. Jangan
menerbitkan token dari browser seller.

## 5. Membaca katalog dan status pilihan

```http
GET /v1/integrations/shipping-services
```

Response utama:

```json
{
  "data": {
    "preference": {
      "configured": true,
      "mode": "custom",
      "enabled_groups": [],
      "services": [
        { "courier_code": "jne", "service_code": "REG" },
        { "courier_code": "jne", "service_code": "YES" }
      ],
      "version": 2,
      "updated_at": "2026-08-19T10:00:00Z"
    },
    "groups": [
      { "code": "regular", "name": "Regular" },
      { "code": "next_day", "name": "Next Day" },
      { "code": "express", "name": "Express" }
    ],
    "couriers": [
      {
        "code": "jne",
        "name": "JNE",
        "provider_code": "rajaongkir",
        "supports_domestic_cost": true,
        "supports_international_cost": true,
        "supports_tracking": true,
        "selection_state": "partial",
        "selected_service_count": 2,
        "total_service_count": 9,
        "services": [
          {
            "code": "REG",
            "name": "JNE Regular",
            "group": "regular",
            "service_type": "parcel",
            "calculation_mode": "provider_quote",
            "selected": true
          },
          {
            "code": "SPS",
            "name": "JNE Super Speed",
            "group": "express",
            "service_type": "parcel",
            "calculation_mode": "provider_quote",
            "selected": false
          }
        ]
      }
    ]
  },
  "meta": { "request_id": "req_example" }
}
```

`selection_state` digunakan langsung untuk checkbox induk:

- `none`: tidak ada layanan anak dipilih;
- `partial`: sebagian layanan anak dipilih;
- `all`: seluruh layanan anak dipilih.

## 6. Menyimpan pilihan

```http
PUT /v1/integrations/shipping-services
Content-Type: application/json
```

PUT mengganti seluruh preferensi secara atomik. UI harus mengirim seluruh
pilihan terbaru, bukan delta satu checkbox.

```json
{
  "mode": "custom",
  "enabled_groups": [],
  "services": [
    { "courier_code": "jne", "service_code": "REG" },
    { "courier_code": "jne", "service_code": "YES" }
  ]
}
```

Response mengembalikan preferensi yang sudah dinormalisasi dan nomor `version`
baru. Kode kurir dinormalisasi lowercase dan kode layanan uppercase. Duplikasi
dihapus otomatis.

Mode `custom` boleh menyimpan array kosong untuk menonaktifkan seluruh layanan.
Checkout kemudian menerima `RATE_NOT_AVAILABLE` sampai setidaknya satu layanan
diaktifkan kembali.

## 7. Alur UI Extension Kurir

1. Backend Emisell membuat tenant token dengan `shipping:read`.
2. UI memanggil GET melalui backend Emisell, bukan langsung membuat tenant
   token di browser.
3. Filter grup hanya menyaring tampilan atau melakukan bulk select.
4. Checkbox induk memilih/melepas semua layanan canonical pada kurir tersebut.
5. UI mengirim seluruh konfigurasi dengan tenant token `shipping:write`.
6. Setelah sukses, UI memakai response server sebagai state baru.

Badge seperti tracking, domestic cost, atau international cost berasal dari
capability katalog. `AWB Otomatis`, pickup, COD, dan pembayaran hanya boleh
ditampilkan setelah capability provider tersebut tersedia; jangan di-hardcode
berdasarkan nama kurir.

## 8. Enforcement pada cek ongkir

Semua jalur cek ongkir tenant menggunakan filter yang sama, termasuk:

```text
/v1/calculate/domestic-cost
/api/v1/calculate/domestic-cost
/api/v1/calculate/district/domestic-cost
/shipping/domestic-cost
```

Urutan efektif:

```text
exact quote provider
→ normalisasi canonical service
→ preferensi merchant
→ hasil checkout
```

Syarat sebuah hasil tampil:

```text
kurir diminta
AND layanan tersedia pada rute
AND credential/provider aktif
AND layanan lolos preferensi merchant
```

Provider dapat mengirim `REG23`, tetapi jika canonical-nya `REG`, pilihan
`jne:REG` tetap meloloskan hasil tersebut. Bentuk response RajaOngkir V2 tidak
berubah; hanya daftar opsi di dalam `data` yang difilter.

## 9. Error

| HTTP | Kode | Arti |
|---|---|---|
| 400 | `INVALID_SHIPPING_SERVICE_PREFERENCE` | kombinasi mode, groups, atau services tidak valid |
| 401 | `TENANT_CONTEXT_REQUIRED` / `INVALID_TENANT_CONTEXT` | tenant token hilang atau tidak valid |
| 403 | `INSUFFICIENT_TENANT_SCOPE` | scope read/write tidak tersedia |
| 422 | `SHIPPING_SERVICE_NOT_FOUND` | pasangan courier/service tidak ada pada katalog aktif |
| 400/422 | `RATE_NOT_AVAILABLE` | tidak ada hasil rute yang lolos konfigurasi |

## 10. Batas dan keamanan

- Maksimal 200 layanan pada mode `custom`.
- Update disimpan dalam satu transaksi database.
- Service `unknown` tidak dapat dipilih pada konfigurasi baru.
- Secret provider tidak pernah menjadi bagian preference atau response katalog.
- `domain_id` tidak mengubah kepemilikan preference.
- Hasil exact quote tetap terisolasi berdasarkan `tenant_id` dan
  `integration_id`.
