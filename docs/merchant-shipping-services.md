# Pilihan Layanan Checkout per Merchant

Dokumen ini menjadi kontrak implementasi Extension Kurir Emisell untuk memilih
kurir dan layanan yang boleh muncul di checkout. Pola UI mengikuti selector
kurir bertingkat: satu checkbox induk per kurir dan checkbox anak untuk setiap
layanan canonical.

## 1. Kepemilikan konfigurasi

- Preferensi dimiliki `merchant_id` dari header backend Emisell.
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
X-Emisell-Merchant-ID: merchant_123
```

Gunakan dedicated service API key milik Main Service. Merchant ID berasal dari
database Emisell dan header maupun API key tidak pernah dikirim ke browser.

## 5. Membaca katalog dan status pilihan

```http
GET /api/v1/integrations/shipping-services
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
        "rate_provider_code": "rajaongkir",
        "tracking_provider_code": "rajaongkir",
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
PUT /api/v1/integrations/shipping-services
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

1. Backend Emisell membaca merchant ID dari sesi seller.
2. UI memanggil GET melalui backend Emisell, bukan langsung ke API Kurir.
3. Filter grup hanya menyaring tampilan atau melakukan bulk select.
4. Checkbox induk memilih/melepas semua layanan canonical pada kurir tersebut.
5. Backend mengirim seluruh konfigurasi dengan service key dan merchant ID.
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
→ policy minimum/maksimum berat
→ preferensi merchant
→ hasil checkout
```

Syarat sebuah hasil tampil:

```text
kurir diminta
AND layanan tersedia pada rute
AND credential/provider aktif
AND berat final Emisell memenuhi minimum/maksimum layanan
AND layanan lolos preferensi merchant
```

Provider dapat mengirim `REG23`, tetapi jika canonical-nya `REG`, pilihan
`jne:REG` tetap meloloskan hasil tersebut. Bentuk response RajaOngkir V2 tidak
berubah; hanya daftar opsi di dalam `data` yang difilter.

Detail pemisahan minimum penerimaan dan minimum tagihan tersedia pada
[`service-weight-eligibility.md`](service-weight-eligibility.md).

## 9. Error

| HTTP | Kode | Arti |
|---|---|---|
| 400 | `INVALID_SHIPPING_SERVICE_PREFERENCE` | kombinasi mode, groups, atau services tidak valid |
| 400 | `MERCHANT_ID_REQUIRED` / `INVALID_MERCHANT_ID` | header merchant hilang atau tidak valid |
| 403 | `MERCHANT_CONTEXT_FORBIDDEN` | customer key mencoba membawa konteks merchant |
| 401 | `UNAUTHORIZED` | service API key tidak valid |
| 422 | `SHIPPING_SERVICE_NOT_FOUND` | pasangan courier/service tidak ada pada katalog aktif |
| 400/422 | `RATE_NOT_AVAILABLE` | tidak ada hasil rute yang lolos konfigurasi |

## 10. Batas dan keamanan

- Maksimal 200 layanan pada mode `custom`.
- Update disimpan dalam satu transaksi database.
- Service `unknown` tidak dapat dipilih pada konfigurasi baru.
- Secret provider tidak pernah menjadi bagian preference atau response katalog.
- Domain tidak mengubah kepemilikan preference dan diselesaikan oleh Emisell.
- Hasil exact quote tetap terisolasi berdasarkan merchant dan credential
  internal yang dipilih API Kurir.
