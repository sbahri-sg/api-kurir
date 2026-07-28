# Matriks Toleransi dan Pembulatan Berat

## 1. Jawaban singkat untuk paket 1,2 kg

Tidak ada satu jawaban yang berlaku untuk seluruh ekspedisi.

| Provider/service | Hasil 1,20 kg | Status |
|---|---:|---|
| JNE JTR | Dibulatkan 1 kg, tetapi ditagih minimum 10 kg | Resmi publik |
| J&T menurut panduan kalkulasi | 1 kg | Resmi publik |
| ID Express | 1 kg | Resmi publik |
| Ninja menurut halaman bantuan harga | 1 kg | Resmi publik, tetapi bertentangan dengan standard terms |
| Ninja menurut standard delivery terms | 2 kg | Resmi publik, perlu scope kontrak |
| RPX | 2 kg | Resmi publik |
| Sentral Cargo, satu koli | Contoh resmi mengarah ke 1,5 kg per koli lalu total 2 kg | Inferensi dari contoh resmi; konfirmasi kontrak |
| TIKI, SiCepat, Lion domestik, AnterAja, Pos, Wahana, SAP, REX, NCS, STAR, DSE | Jangan diasumsikan; gunakan quote/rate sheet | Belum ada aturan publik yang cukup |

Dimensi tetap dapat mengalahkan berat aktual. Jika paket 1,2 kg memiliki berat
volumetrik 3 kg, angka yang diproses adalah 3 kg, bukan 1,2 kg.

## 2. Istilah yang harus dibedakan

### Berat aktual

Angka dari timbangan setelah kemasan.

### Berat volumetrik

Hasil dimensi:

```text
length_cm * width_cm * height_cm / divisor
```

### Chargeable weight

Nilai terbesar antara berat aktual dan volumetrik.

### Rounded weight

Chargeable weight setelah aturan pembulatan service.

### Billing weight

Berat setelah minimum charge, pembulatan, aturan per koli, dan agregasi. Nilai
inilah yang dikalikan dengan rate card.

Contoh JNE JTR:

```text
actual           = 1,20 kg
volumetric       = 0,90 kg
chargeable       = 1,20 kg
rounded          = 1 kg
minimum JTR      = 10 kg
billing weight   = 10 kg
```

Jadi pernyataan “1,2 kg dihitung 1 kg” benar pada tahap pembulatan JTR, tetapi
ongkir JTR tetap memakai minimum 10 kg.

## 3. Matriks detail

`—` berarti belum boleh dihitung lokal berdasarkan sumber publik.

| Kurir/service | 1,20 kg | 1,30 kg | 1,31 kg | Aturan | Keputusan engine |
|---|---:|---:|---:|---|---|
| JNE JTR | 1 kg | 2 kg | 2 kg | Fraksi `<0,30` turun; `>=0,30` naik; minimum 10 kg | Aktif hanya untuk JTR |
| JNE REG/YES/SPS | — | — | — | Tidak ditemukan batas resmi publik yang cukup | Provider quote/kontrak |
| TIKI SDS/ONS/REG/ECO | — | — | — | Website menerima berat, tetapi tidak mempublikasikan threshold | Provider quote/kontrak |
| TIKI TRC | Min. 10 kg | Min. 10 kg | Min. 10 kg | Minimum 10 kg; rounding di atas minimum belum publik | Provider quote/kontrak |
| SiCepat REG/BEST/HALU | — | — | — | Terms memuat berat dimensi, tetapi threshold tidak terpapar jelas | Provider quote/kontrak |
| SiCepat GOKIL | Min. 10 kg | Min. 10 kg | Min. 10 kg | Minimum charge 10 kg; rounding berikutnya belum publik | Provider quote/kontrak |
| J&T panduan umum | 1 kg | Batas ambigu | 2 kg | `<1,30` menjadi 1; `>1,30` menjadi 2 | Jangan gunakan tepat 1,30 tanpa profile |
| J&T SUPER | 1 kg | 1 kg | 2 kg | `1–1,30` mengikuti 1 kg; di atas 1,30 mengikuti kg berikutnya | Profile khusus SUPER |
| J&T HEBOH | Tidak eligible | Tidak eligible | Tidak eligible | Berat layanan mulai 2,31 kg menurut terms terkini | Provider catalog |
| ID Express | 1 kg | 1 kg | 2 kg | `0–1,30` menjadi 1; `1,31–2,30` menjadi 2 | Aktif sesuai tabel resmi |
| Ninja help center | 1 kg | 2 kg | 2 kg | Fraksi `<0,30` turun; `>=0,30` naik | Profile channel bantuan |
| Ninja standard terms | 2 kg | 2 kg | 2 kg | Setiap berat aktual dibulatkan ke atas | Provider quote sampai scope dikonfirmasi |
| Lion Parcel domestik | — | — | — | Tidak ada aturan domestik terkini yang cukup | Provider quote/kontrak |
| Lion Parcel internasional | 2 kg | 2 kg | 2 kg | 1,01 kg menjadi 2 kg | Hanya service internasional |
| AnterAja | — | — | — | Basis aktual vs volumetrik dipublikasikan; threshold tidak | Provider quote/kontrak |
| Pos Indonesia domestik | — | — | — | Menggunakan tingkat berat; threshold tidak dipublikasikan cukup | Provider quote/kontrak |
| Wahana parcel | — | — | — | Ditagih dalam kg; metode pembulatan tidak dinyatakan | Provider quote/kontrak |
| Wahana Kargo | Min. 10 kg | Min. 10 kg | Min. 10 kg | Minimum 10 kg; rounding berikutnya tidak dinyatakan | Provider quote/kontrak |
| RPX parcel/Big Helow | 2 kg | 2 kg | 2 kg | Setiap pecahan desimal kg dibulatkan ke atas | Aktif, versi rate sheet |
| RPX HWP | Min. 20 kg | Min. 20 kg | Min. 20 kg | Minimum service 20 kg dan pecahan naik | Aktif, versi rate sheet |
| Sentral Cargo | Contoh mengarah ke 1,5 kg; total dapat 2 kg | Contoh mengarah ke 1,5 kg; total dapat 2 kg | Contoh mengarah ke 1,5 kg; total dapat 2 kg | Fraksi mulai 0,11 naik ke setengah kg, lalu total akhir naik ke kg | Konfirmasi sebelum local final |
| SAP Express | — | — | — | Belum ada ketentuan rounding resmi terkini | Provider quote/kontrak |
| REX | — | — | — | Sumber resmi belum cukup | Nonaktif |
| NCS | — | — | — | Identitas provider dan aturan belum terkunci | Nonaktif |
| STAR Cargo | — | — | — | Formula rounding belum publik | Provider quote/kontrak |
| DSE | — | — | — | Sumber resmi belum cukup | Nonaktif |

## 4. Rincian sumber terverifikasi

### JNE JTR

[Halaman resmi JNE JTR](https://www.jne.co.id/jtr-indonesia) menyatakan:

- berat aktual `<1,3 kg` dibulatkan ke bawah menjadi 1 kg;
- berat aktual `>=1,3 kg` dibulatkan ke atas menjadi 2 kg;
- minimum tagihan 10 kg;
- di atas 10 kg berlaku kenaikan per kg sesuai tujuan;
- berat volumetrik memakai divisor 5.000.

Aturan itu hanya terverifikasi untuk JTR. Jangan otomatis menyalinnya ke REG,
YES, atau SPS.

### J&T Express

[Panduan perhitungan J&T](https://help.jet.co.id/web-show/NXRTVktVTE4xOHRoWHJMdDRFRldsdz09)
menyebutkan berat di bawah 1,3 kg menjadi 1 kg dan di atas 1,3 kg menjadi 2
kg. Kalimat tersebut tidak menyelesaikan kasus tepat 1,30 kg.

[Ketentuan J&T SUPER](https://help.jet.co.id/web-show/VGRCM1hBOVNtdVdjclVQWTVTUTFkZz09)
lebih spesifik: 1–1,30 kg mengikuti harga 1 kg dan di atas 1,30 kg mengikuti
harga per kilo. Karena ketentuan dapat berubah dan berlaku per produk, simpan
rule terpisah untuk `SUPER`.

Terms J&T terkini juga memperkenalkan `HEBOH` dengan eligibility 2,31–10,3 kg.
Eligibility bukan pembulatan dan tidak boleh dipakai sebagai rule layanan lain.

### ID Express

[Syarat resmi ID Express](https://idexpress.com/bantuan/syarat-dan-ketentuan)
memuat tabel:

```text
0–1,30 kg     -> 1 kg
1,31–2,30 kg  -> 2 kg
2,31–3,30 kg  -> 3 kg
```

Dengan demikian 1,20 kg dan 1,30 kg menjadi 1 kg; 1,31 kg menjadi 2 kg.
Dokumen memakai resolusi 0,01 kg. Jika provider API mengirim gram, engine
tetap menyimpan gram. Rentang 1.301–1.309 gram tidak dijelaskan oleh tabel;
cara carrier mengubah timbangan ke dua desimal harus dikonfirmasi sebelum
local calculation pada boundary tersebut.

### Ninja Xpress

[Halaman bantuan Ninja](https://www.ninjaxpress.co/id-id/support/shipper-support/pricing-and-coverage/how-to-calculate-and-check-shipping-rates)
menyatakan fraksi di bawah 0,3 kg turun dan sama dengan/di atas 0,3 kg naik.
Contohnya 1,29 kg menjadi 1 kg dan 1,32 kg menjadi 2 kg.

Namun [Standard Delivery Terms Ninja](https://www.ninjaxpress.co/id-id/standard-delivery-terms)
menyatakan berat aktual dibulatkan ke atas, dengan contoh 1,3 kg menjadi 2 kg.
Karena dua sumber resmi mempunyai scope/redaksi berbeda:

1. jangan tetapkan rule global `ninja`;
2. minta konfirmasi rule untuk akun/API merchant;
3. simpan rule per service, channel, dan contract version;
4. sebelum terkonfirmasi, quote provider adalah final.

### Lion Parcel

[Ketentuan internasional Lion Parcel](https://lionparcel.com/pengiriman-internasional)
menyatakan 1,01 kg menjadi 2 kg. Ini tidak boleh dipakai untuk layanan domestik.

Artikel Lion mengenai program lama `SIKAT` memuat pembulatan per 0,5 kg, tetapi
itu adalah promo lama, bukan regulasi domestik universal. Rule tersebut tidak
diaktifkan.

### RPX

[Big Helow RPX](https://rpx.co.id/service/bighelow) dan rate sheet resmi RPX
menyatakan setiap pecahan desimal kilogram dibulatkan ke atas. Maka 1,01,
1,20, dan 1,30 kg menjadi 2 kg.

Rate sheet harus mempunyai tanggal efektif. Formula dapat disimpan, tetapi
harga lama tidak boleh digunakan untuk transaksi baru.

### Sentral Cargo

[Syarat resmi Sentral Cargo](https://sentralcargo.co.id/syarat-dan-ketentuan)
memberi contoh bertahap:

```text
8,11 kg -> 8,50 kg
7,51 kg -> 8,00 kg
total 16,50 kg -> 17 kg
```

Implementasi yang paling konsisten dengan contoh adalah pembulatan per koli ke
increment setengah kilogram setelah toleransi 0,10 kg, kemudian pembulatan
total. Untuk satu koli 1,20 kg, inferensinya:

```text
per-koli rounded = 1,50 kg
aggregate total  = 1,50 kg
final billed     = 2,00 kg
```

Ketentuan pickup juga dapat mengenakan minimum 10 kg. Minimum itu
bersyarat pada skema pickup, bukan otomatis seluruh transaksi. Karena teks
publik hanya memberi contoh dan frasa singkat “0.11 pembulatan ke atas”,
profile ini tetap `needs_contract_confirmation`.

## 5. Provider yang belum mempunyai threshold publik

Untuk TIKI, SiCepat, Lion domestik, AnterAja, Pos, Wahana, SAP, REX, NCS,
STAR, dan DSE, sumber resmi yang ditinjau belum memberikan batas pembulatan
yang cukup jelas untuk local calculation.

Ini tidak berarti semuanya pasti melakukan `ceil`. Kebijakan dapat berupa:

- toleransi 0,30 kg;
- setiap pecahan naik;
- increment 0,50 kg;
- tier per service;
- minimum charge;
- rule khusus kontrak marketplace.

Selama belum dikonfirmasi:

```text
rounding_mode = provider_quote
verification_status = needs_contract_confirmation
```

API Kurir tetap mengirim berat asli dalam gram kepada RajaOngkir/direct API
dan menyimpan hasil quote. Jangan mengubah 1.200 gram menjadi 1.000 atau 2.000
sebelum upstream.

## 6. Data model yang diperlukan

Aturan pembulatan tidak cukup hanya dengan `rounding_threshold_grams`.

```text
weight_rounding_profiles
- id
- courier_service_id
- channel
- contract_version
- basis
- scope
- mode
- increment_grams
- threshold_grams
- threshold_boundary
- minimum_weight_grams
- maximum_weight_grams
- apply_minimum_stage
- aggregate_rounding_mode
- effective_from
- effective_until
- verification_status
- source_id
```

Nilai:

```text
basis:
  actual
  volumetric
  greater_of_actual_or_volumetric

scope:
  per_shipment
  per_koli
  after_aggregate

mode:
  threshold
  ceil_increment
  floor_increment
  tier_table
  provider_quote

threshold_boundary:
  up_when_equal
  down_when_equal
  explicit_ranges
```

Boundary wajib disimpan karena:

```text
JNE JTR    : 1,30 kg -> 2 kg (up_when_equal)
ID Express: 1,30 kg -> 1 kg (down_when_equal/explicit_ranges)
J&T SUPER : 1,30 kg -> 1 kg (down_when_equal)
```

## 7. Urutan kalkulasi

Urutan harus berasal dari profile:

```text
actual_weight
volumetric_weight
      |
      v
select basis
      |
      v
per-koli rounding, jika berlaku
      |
      v
aggregate, jika multikoli
      |
      v
minimum charge pada stage yang ditentukan
      |
      v
final rounding/tier
      |
      v
billing weight
```

Pseudocode:

```text
function calculateBillingWeight(shipment, rule):
  bases = []

  for koli in shipment.packages:
    actual = koli.actual_weight_grams
    volume = calculateVolume(koli, rule.volumetric_divisor)
    basis = selectBasis(actual, volume, rule.basis)

    if rule.scope == "per_koli":
      basis = roundByProfile(basis, rule)

    bases.append(basis)

  total = sum(bases)

  if rule.apply_minimum_stage == "after_aggregate":
    total = max(total, rule.minimum_weight_grams)

  if rule.scope == "after_aggregate":
    total = roundByProfile(total, rule)

  total = applyAggregateRounding(total, rule)
  return total
```

## 8. Perlakuan quote RajaOngkir

Pada cache miss:

```json
{
  "origin": "origin-provider-id",
  "destination": "destination-provider-id",
  "weight": 1200,
  "courier": "jne:tiki:sicepat"
}
```

Simpan:

- requested weight: 1.200 gram;
- returned service dan total;
- source provider;
- timestamp;
- raw response hash;
- rule profile yang digunakan, jika ada.

Jika hanya mempunyai total quote, jangan menyimpulkan threshold dari satu
sampel. Probe minimum untuk kandidat threshold 0,30 kg:

```text
1.000 g
1.200 g
1.299 g
1.300 g
1.301 g
1.310 g
2.000 g
```

Hasil probe berstatus `observed`. Status berubah menjadi `official_contract`
hanya setelah rate sheet/API contract mengonfirmasi scope dan boundary.

## 9. Respons API

Respons cek ongkir harus transparan:

```json
{
  "weight": {
    "actual_grams": 1200,
    "volumetric_grams": 0,
    "chargeable_grams": 1200,
    "rounded_grams": 1000,
    "billing_grams": 10000,
    "minimum_grams": 10000,
    "rounding_profile": "jne-jtr-public-2026"
  }
}
```

Jika aturan belum dikonfirmasi:

```json
{
  "weight": {
    "actual_grams": 1200,
    "billing_grams": 1200,
    "calculated_by": "provider"
  },
  "source": {
    "type": "provider_quote",
    "verification_status": "official_provider_response"
  }
}
```

`billing_grams` pada provider quote adalah nilai dari provider jika tersedia.
Jika provider hanya mengembalikan total harga, jangan mengarang
`billing_grams`; gunakan `null`.

## 10. Aturan implementasi

1. Emisell mengirim berat aktual dan dimensi tanpa pembulatan.
2. API Kurir tidak mempunyai default toleransi global.
3. Rule berada pada level `courier_service + channel + contract_version`.
4. Berat volumetrik dihitung sebelum threshold jika kontrak menyatakan basis
   terbesar aktual/volume.
5. Minimum cargo tidak sama dengan toleransi berat.
6. Exact boundary seperti 1.300 gram harus memiliki test case.
7. Quote provider mengalahkan rule `observed`.
8. Berat hasil timbang provider dapat berbeda dari deklarasi Emisell; simpan
   keduanya untuk rekonsiliasi tagihan.

## 11. Test case minimum

Setiap profile harus lulus:

```text
0 g              -> invalid
1 g              -> minimum/first tier
999 g
1.000 g
1.200 g
1.299 g
1.300 g
1.301 g
1.310 g
9.999 g
10.000 g
10.001 g
```

Tambahkan skenario:

- volumetrik lebih besar dari aktual;
- aktual lebih besar dari volumetrik;
- dua koli;
- minimum cargo;
- maximum service;
- rule sebelum dan sesudah tanggal efektif.
