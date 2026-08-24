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
  `jne:REG`, `jne:YES`, dan `jne:JTR`.

## 2. Grup canonical

| Grup | Contoh |
|---|---|
| `regular` | JNE REG, J&T EZ, TIKI REG |
| `next_day` | JNE YES, TIKI ONS |
| `economy` | JNE OKE, TIKI ECO |
| `cargo` | JNE JTR, TIKI TRC, SiCepat GOKIL |

GET Emisell Gateway hanya mengirim empat grup tersebut. Layanan `express`,
`same_day`, `instant`, `international`, `special`, dan `unknown` tetap boleh ada
di master internal untuk kebutuhan provider, tetapi tidak dikirim ke Emisell,
tidak dapat dipilih melalui PUT, dan tidak lolos ke hasil checkout merchant.

## 3. Mode pilihan

API hanya menerima mode `custom`. Seller memilih setiap layanan secara
eksplisit sehingga layanan baru hasil sinkronisasi provider tidak otomatis
tampil di checkout.

```json
{
  "mode": "custom",
  "services": [
    { "courier_code": "jne", "service_code": "REG" },
    { "courier_code": "jne", "service_code": "YES" },
    { "courier_code": "jnt", "service_code": "EZ" }
  ]
}
```

Field `mode` boleh dihilangkan; server tetap menyimpan `custom`. Payload lama
yang masih mengirim `mode: custom` dan `enabled_groups: []` tetap diterima.
Nilai `all` atau `groups` ditolak agar checkbox seller selalu menjadi sumber
pilihan yang eksplisit.

Merchant yang belum pernah memanggil endpoint PUT mempunyai `configured=false`,
mode `custom`, dan nol layanan terpilih. Cek ongkir tidak menampilkan opsi
hingga seller menyimpan setidaknya satu layanan. Saat deployment, preference
lama `all/groups` dikonversi ke pasangan custom yang ekuivalen agar pilihan
aktif tidak hilang.

## 4. Autentikasi

Semua endpoint memakai dua header:

```http
key: <main-service-api-key>
X-Emisell-Merchant-ID: merchant_123
```

Gunakan key bertipe `main_service` hasil generate dashboard dengan scope
`gateway:access`. `API_KEYS` environment hanya menjadi recovery. Merchant ID
berasal dari database Emisell dan kedua header tidak pernah dikirim ke browser.

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
    "limits": {
      "enforced": true,
      "couriers": {
        "maximum": 5,
        "selected": 1,
        "remaining": 4,
        "available": 14
      },
      "services": {
        "maximum": 20,
        "selected": 2,
        "remaining": 18,
        "available": 88
      }
    },
    "groups": [
      { "code": "regular", "name": "Regular" },
      { "code": "next_day", "name": "Next Day" },
      { "code": "economy", "name": "Economy" },
      { "code": "cargo", "name": "Cargo" }
    ],
    "couriers": [
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
        "selection_state": "partial",
        "selectable": true,
        "selected_service_count": 2,
        "total_service_count": 9,
        "services": [
          {
            "code": "REG",
            "name": "JNE Regular",
            "group": "regular",
            "service_type": "parcel",
            "calculation_mode": "provider_quote",
            "selected": true,
            "selectable": true
          },
          {
            "code": "JTR",
            "name": "JNE Trucking",
            "group": "cargo",
            "service_type": "cargo",
            "calculation_mode": "provider_quote",
            "selected": false,
            "selectable": true
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

`limits.couriers` dan `limits.services` memberi nilai `maximum`, `selected`,
`remaining`, dan jumlah `available` pada katalog. Dashboard wajib mematuhi
`selectable`: pilihan yang sudah aktif selalu dapat dilepas, sementara pilihan
baru menjadi nonaktif saat limit terkait habis. Backend tetap melakukan
validasi yang sama sehingga limit tidak dapat dilewati dengan request manual.

## 6. Menyimpan pilihan

```http
PUT /api/v1/integrations/shipping-services
Content-Type: application/json
```

PUT mengganti seluruh preferensi secara atomik. UI harus mengirim seluruh
pilihan terbaru, bukan delta satu checkbox.

```json
{
  "services": [
    { "courier_code": "jne", "service_code": "REG" },
    { "courier_code": "jne", "service_code": "YES" }
  ]
}
```

Payload dengan `"mode": "custom"` tetap valid. `enabled_groups` hanya diterima
bila berupa array kosong untuk kompatibilitas client lama.

Response mengembalikan preferensi yang sudah dinormalisasi dan nomor `version`
baru. Kode kurir dinormalisasi lowercase dan kode layanan uppercase. Duplikasi
dihapus otomatis. Layanan dari grup selain `regular`, `next_day`, `economy`,
dan `cargo` ditolak sebagai layanan yang tidak tersedia pada katalog Emisell.

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

Logo kurir selalu dibaca dari field `couriers[].logo`. Emisell tidak perlu
menyimpan pemetaan gambar per kode kurir dan harus memakai fallback visual bila
URL gagal dimuat. Aset di-host API Kurir agar perubahan sumber eksternal tidak
mengubah kontrak maupun tampilan extension.

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
kurir berasal dari service yang dipilih merchant
AND layanan tersedia pada rute
AND credential/provider aktif
AND berat final Emisell memenuhi minimum/maksimum layanan
AND layanan lolos preferensi merchant
```

Untuk provider bawaan `emisell`, tidak adanya `credential_id` tenant berarti
API Kurir memakai pool credential platform setelah memastikan provider Emisell
aktif untuk merchant tersebut. Main Service tetap hanya mengirim merchant ID,
origin, destination, dan berat. Provider BYOK tetap wajib memakai credential
milik merchant dan tidak pernah meminjam pool platform.

Provider dapat mengirim `REG23`, tetapi jika canonical-nya `REG`, pilihan
`jne:REG` tetap meloloskan hasil tersebut. Bentuk response RajaOngkir V2 tidak
berubah; hanya daftar opsi di dalam `data` yang difilter.

Pada request Main Service dengan `X-Emisell-Merchant-ID`, parameter `courier`
tidak diperlukan dan diabaikan bila dikirim. API Kurir mengambil kumpulan kode
kurir langsung dari konfigurasi service merchant. Public API tanpa Merchant ID
tetap mewajibkan `courier` agar kompatibel dengan RajaOngkir V2.

Detail pemisahan minimum penerimaan dan minimum tagihan tersedia pada
[`service-weight-eligibility.md`](service-weight-eligibility.md).

## 9. Error

| HTTP | Kode | Arti |
|---|---|---|
| 400 | `INVALID_SHIPPING_SERVICE_PREFERENCE` | mode bukan custom, enabled_groups terisi, atau services tidak valid |
| 400 | `MERCHANT_ID_REQUIRED` / `INVALID_MERCHANT_ID` | header merchant hilang atau tidak valid |
| 403 | `MERCHANT_CONTEXT_FORBIDDEN` | public key mencoba membawa konteks merchant |
| 401 | `UNAUTHORIZED` | Main Service key tidak aktif/tidak valid atau tidak memiliki gateway:access |
| 422 | `SHIPPING_SERVICE_NOT_FOUND` | pasangan courier/service tidak ada pada katalog aktif |
| 422 | `SHIPPING_SERVICE_LIMIT_EXCEEDED` | jumlah kurir atau layanan melebihi limit merchant |
| 400/422 | `RATE_NOT_AVAILABLE` | tidak ada hasil rute yang lolos konfigurasi |

## 10. Batas dan keamanan

- Default production adalah maksimal 5 kurir dan 20 layanan per merchant.
- Operator dapat mengubahnya melalui `MERCHANT_SHIPPING_MAX_COURIERS` dan
  `MERCHANT_SHIPPING_MAX_SERVICES`; batas keras layanan adalah 200.
- Update disimpan dalam satu transaksi database.
- Hanya service dalam grup `regular`, `next_day`, `economy`, dan `cargo` yang
  dapat dipilih pada konfigurasi baru.
- Secret provider tidak pernah menjadi bagian preference atau response katalog.
- Domain tidak mengubah kepemilikan preference dan diselesaikan oleh Emisell.
- Hasil exact quote tetap terisolasi berdasarkan merchant dan credential
  internal yang dipilih API Kurir.
