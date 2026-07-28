# Katalog Layanan dan Regulasi Provider

## 1. Cara membaca dokumen

Katalog ini membedakan aturan yang dapat dihitung lokal dengan informasi yang
masih harus dikonfirmasi lewat kontrak, rate sheet, atau API merchant.

Status:

- **Siap aturan lokal**: formula publik cukup jelas, tetapi harga rute tetap
  berasal dari rate card resmi;
- **Parsial**: produk terverifikasi, namun divisor, pembulatan, batas, atau
  surcharge belum lengkap;
- **Provider quote**: jangan menghitung harga final secara mandiri;
- **Tahan aktivasi**: identitas/sumber resmi belum cukup jelas.

Nama produk, kode layanan, coverage, SLA, dan tarif dapat berbeda menurut akun,
rute, channel, serta periode. Tabel bukan pengganti kontrak komersial.

## 2. Ringkasan kesiapan

| Kurir | Produk yang terverifikasi dari sumber publik | Aturan berat utama | Kesiapan |
|---|---|---|---|
| JNE | REG, YES, SPS, JTR | JTR min 10 kg; divisor 5.000; pembulatan terverifikasi untuk JTR | Siap JTR/parsial lainnya |
| TIKI | SDS, ONS, REG, ECO, TRC, INT | TRC min 10 kg; formula rinci perlu kontrak | Parsial |
| SiCepat | REGULER, BEST, HALU, GOKIL, COD | GOKIL minimum charge 10 kg | Parsial |
| J&T Express | EZ, ECO, SUPER, HEBOH/HBO, DOC | Divisor 6.000; threshold 1,30 perlu scope service | Siap/parsial |
| ID Express | Lite, Regular, Cargo | Lite <510 g; Cargo >10 kg; pembulatan ambang 0,30 kg | Parsial |
| Ninja Xpress | Regular/Standard, Same Day, Cargo | Maks 30 kg; divisor 6.000; ambang 0,30 kg | Siap/parsial |
| Lion Parcel | Priority, Regular, Light, Economy, Big Package | Light maks 300 g; paket besar mulai 10 kg | Parsial |
| AnterAja | Regular, Same Day, Next Day, Instant, Mini Cargo | Mini Cargo >4 kg | Parsial |
| Pos Indonesia | Same Day, Next Day, Reguler, Ekonomi, Kargo | Pos Reguler maks 50 kg | Provider quote |
| Wahana | NextDay, Express, Ekonomis, Kargo | Parcel divisor 6.000; Kargo divisor 5.000 dan min 10 kg | Siap aturan lokal |
| RPX | SameDay, NextDay, Regular, HWP, Big Helow | HWP min 20 kg; parcel divisor 6.000; Big Helow divisor 4.000 | Parsial |
| Sentral Cargo | Darat, Laut, Udara | Darat/laut divisor 4.000; udara 6.000; minimum tertentu 10 kg | Siap/parsial |
| SAP Express | SDS, ODS, kargo laut/udara, dedicated | Formula publik terbaru belum cukup | Provider quote |
| REX | Perlu katalog merchant terbaru | Belum terverifikasi | Tahan aturan lokal |
| NCS | Identitas carrier harus dikunci | Belum terverifikasi | Tahan aktivasi |
| STAR Cargo | Udara, darat, laut | Formula publik belum cukup | Provider quote |
| DSE | Perlu katalog merchant terbaru | Belum terverifikasi | Tahan aktivasi |

## 3. JNE

Produk:

- `REG`: pengiriman reguler;
- `YES`: layanan dengan target esok hari pada coverage tertentu;
- `SPS`: layanan prioritas/super cepat pada coverage tertentu;
- `JTR`: trucking untuk paket besar;
- varian motor JTR dapat muncul sebagai kategori di bawah/di atas kapasitas
  mesin tertentu; kode aktual harus diambil dari katalog merchant.

Aturan JTR yang dapat dimodelkan:

```text
minimum_weight_kg = 10
volumetric_divisor = 5000
chargeable_weight = max(actual, volumetric, 10 kg)
```

Halaman resmi JTR menyatakan kiriman di bawah 10 kg tetap dikenakan tarif
minimum 10 kg, dan berat berikutnya dihitung per kg menurut rute. Pembulatan
berat yang dipublikasikan khusus pada halaman JTR:

```text
fraksi < 0,3 kg  -> turun ke kg sebelumnya
fraksi >= 0,3 kg -> naik ke kg berikutnya
```

Contoh: 1,20 kg menjadi 1 kg; 1,30 kg menjadi 2 kg. Untuk kargo per koli di
atas 250 kg terdapat surcharge; angka surcharge harus berasal dari rate sheet
aktif.

Implementasi:

- simpan REG/YES/SPS sebagai service terpisah walaupun rutenya sama;
- JTR tidak boleh dihitung dengan formula REG;
- aturan pembulatan JTR tidak boleh otomatis dipakai pada REG/YES/SPS;
- jangan menyimpulkan tarif per kg hanya dari satu total quote;
- coverage dan SLA tetap dibaca dari quote/rate sheet.

Sumber: [JNE Trucking](https://www.jne.co.id/jtr-indonesia).

## 4. TIKI

Produk resmi:

- `SDS`: Same Day Service, estimasi hingga 6 jam pada coverage tertentu;
- `ONS`: Over Night Service;
- `REG`: reguler, estimasi yang ditampilkan sekitar tiga hari;
- `ECO`: ekonomis;
- `TRC`: trucking, minimum charge 10 kg;
- `INT`: internasional;
- layanan khusus meliputi `FROOZY`, `SRP`, `DAT`, dan `TRX`.

Yang belum boleh diasumsikan:

- divisor volumetrik;
- ambang pembulatan;
- maksimum berat/dimensi per service;
- formula surcharge dan asuransi.

Karena itu TIKI diberi `calculation_mode=provider_quote` sampai rate sheet
merchant mengisi field tersebut. Minimum 10 kg TRC boleh disimpan sebagai
aturan publik, tetapi belum cukup untuk menghitung total harga.

Sumber: [Produk TIKI](https://www.tiki.id/id/produk).

## 5. SiCepat

Produk yang muncul pada kanal resmi:

- `REGULER`;
- `BEST`, layanan lebih cepat dengan target sekitar satu hari pada coverage;
- `HALU`, produk ekonomis melalui channel/e-commerce tertentu;
- `GOKIL`, pengiriman kargo;
- `COD`;
- `H3LO`, produk berbasis paket 3 kg pada channel tertentu.

GOKIL memiliki minimum charge 10 kg dan estimasi yang dipublikasikan 2–5 hari.
Minimum tersebut bukan berarti berat aktual harus 10 kg; kiriman di bawah
minimum dapat tetap ditagih sebagai 10 kg apabila rute/channel menerimanya.

Divisor volumetrik, pembulatan fraksi, maksimum ukuran, dan surcharge tidak
cukup jelas pada sumber publik yang ditinjau. Semua field itu wajib berasal
dari kontrak/API merchant.

Sumber:

- [Layanan GOKIL](https://ekspres.sicepat.com/services/GOKIL)
- [Ringkasan layanan SiCepat](https://sahabatsicepat.com/sicepat-ekspres-layanan-ke-semua-segmen/)

## 6. J&T Express

Produk pada syarat resmi:

- `EZ`: reguler, sekitar 2–3 hari kerja;
- `ECO`: ekonomis;
- `SUPER`: prioritas;
- `HEBOH`/`HBO`: bulky/heavy item; kode aktual mengikuti katalog akun;
- `DOC`: dokumen.

Aturan:

```text
volumetric_weight_kg = length_cm * width_cm * height_cm / 6000
billing_base = max(actual_weight, volumetric_weight)
DOC: minimum 0,5 kg, maksimum 3 kg
```

Panduan umum menyatakan `<1,30 kg` mengikuti 1 kg dan `>1,30 kg` menjadi
2 kg, tetapi tidak menjelaskan tepat 1,30 kg. Ketentuan khusus SUPER menyatakan
`1–1,30 kg` mengikuti 1 kg. HEBOH pada terms terkini mempunyai eligibility
2,31–10,3 kg. Simpan semua rule pada level service dan versi kontrak.

Syarat resmi juga menerangkan wooden packing menambah berat perhitungan 30%;
jika nilai tambah itu di bawah 3 kg, komponen packing dibulatkan menjadi 3 kg.
Aturan ini harus dibuat sebagai surcharge/derived weight terpisah dan
divalidasi dengan akun merchant sebelum produksi.

Teks batas dimensi SUPER pada halaman publik memiliki format yang ambigu.
Jangan menerjemahkannya menjadi batas teknis tanpa konfirmasi tertulis.

Sumber:

- [Ketentuan layanan J&T Express](https://jet.co.id/information/terms/jtsuper)
- [Panduan kalkulasi berat J&T](https://help.jet.co.id/web-show/NXRTVktVTE4xOHRoWHJMdDRFRldsdz09)
- [Ketentuan J&T SUPER](https://help.jet.co.id/web-show/VGRCM1hBOVNtdVdjclVQWTVTUTFkZz09)

## 7. ID Express

Produk:

- `Lite`: paket ringan di bawah 0,51 kg;
- `Regular`: estimasi publik 2–10 hari, tergantung tujuan;
- `Cargo`: paket di atas 10 kg.

Pembulatan resmi:

```text
0,00–1,30 kg -> 1 kg
1,31–2,30 kg -> 2 kg
2,31–3,30 kg -> 3 kg
dan seterusnya
```

Artinya 1,20 kg dan tepat 1,30 kg menjadi 1 kg; 1,31 kg menjadi 2 kg.

Implementasi sebaiknya menyimpan threshold sebagai field, bukan hardcode
khusus ID Express. Divisor volumetrik dan aturan paket tepat 510 gram/10 kg
perlu diuji terhadap respons API merchant karena teks marketing menggunakan
kata “di bawah” atau “di atas”.

Sumber:

- [ID Express](https://idexpress.com/)
- [Syarat dan ketentuan ID Express](https://idexpress.com/bantuan/syarat-dan-ketentuan)

## 8. Ninja Xpress

Produk utama yang ditampilkan adalah pengiriman regular/standard, same day,
dan cargo. Ketentuan standard menyatakan batas berat paket 30 kg.

Kalkulasi:

```text
volumetric_weight_kg = length_cm * width_cm * height_cm / 6000
chargeable_weight = max(actual_weight, volumetric_weight)
fraksi < 0,3 kg  -> turun
fraksi >= 0,3 kg -> naik
```

Halaman dukungan memuat ambang 0,3 kg, sedangkan standard terms juga
menyebutkan pembulatan berat ke atas. Karena redaksinya dapat diterapkan pada
produk/channel berbeda, policy pembulatan wajib diberi scope per service dan
divalidasi menggunakan quote sampel.

Ninja Care yang dipublikasikan:

- nilai barang sampai Rp1.000.000: biaya tetap Rp2.500;
- di atas Rp1.000.000: 0,25% dari nilai barang;
- komponen pajak tidak boleh di-hardcode karena dapat berubah.

Batas dimensi pada syarat publik menggunakan penulisan rentang jumlah
panjang+lebar+tinggi. Simpan sebagai `max_dimension_sum_cm` hanya setelah
konfirmasi akun.

Sumber:

- [Standard Delivery Terms](https://www.ninjaxpress.co/id-id/standard-delivery-terms)
- [Cara menghitung ongkir](https://www.ninjaxpress.co/id-id/support/shipper-support/pricing-and-coverage/how-to-calculate-and-check-shipping-rates)

## 9. Lion Parcel

Kategori produk resmi yang terlihat pada saat peninjauan:

- priority: sekitar 1–2 hari;
- regular: sekitar 2–3 hari;
- light: maksimum 300 gram;
- economy: sekitar 2–7 hari;
- large package: mulai 10 kg, sekitar 6–9 hari;
- large/faster: di atas 10 kg, sekitar 2–3 hari;
- international.

Kode yang dikenal seperti `REGPACK`, `BIGPACK`, atau `BIGPACK FAST` tidak
boleh dianggap stabil selamanya. Simpan nama marketing dan `provider_code`
secara terpisah, lalu sinkronkan katalog akun.

Divisor volumetrik, rounding, batas ukuran, dan minimum charge per rute belum
cukup untuk local calculation dari sumber publik. Gunakan provider quote atau
rate sheet aktif.

Sumber: [Produk Lion Parcel](https://lionparcel.com/product/).

## 10. AnterAja

Produk:

- `Regular`: sekitar 1–2 hari Jabodetabek/Jawa, 2–4 hari antarprovinsi,
  dan 5–9 hari untuk jangkauan nasional yang ditampilkan;
- `Same Day`: target diterima paling lambat pukul 22.00 sesuai ketentuan;
- `Next Day`: target H+1;
- `Instant`: akses melalui integrasi/API dan coverage tertentu;
- `Mini Cargo`: untuk berat di atas 4 kg.

Estimasi adalah target layanan, bukan jaminan universal. Hari libur, cut-off,
coverage, dan kondisi operasional tetap mengikuti respons provider.

Divisor, pembulatan, serta minimum charge Mini Cargo tidak boleh diturunkan
hanya dari pernyataan “di atas 4 kg”. Gunakan rate sheet/API merchant.

Sumber: [Layanan AnterAja](https://anteraja.id/id/services).

## 11. Pos Indonesia

Kelompok layanan mencakup same day, next day, Pos Reguler, ekonomi, dan kargo.
Pos Reguler dipublikasikan dengan estimasi H+2 sampai H+4, maksimum 50 kg,
tracking, dan opsi jaminan/asuransi sesuai ketentuan.

Pos menyatakan harga dapat bergantung pada berat atau volume, tetapi faktor
volumetrik dan pembulatan tidak lengkap pada sumber publik yang ditinjau.
Gunakan `provider_quote` sampai rate sheet resmi memberikan formula.

Sumber: [Pos Reguler](https://www.posindonesia.co.id/id/pages/pos-reguler).

## 12. Wahana

Produk: `NextDay`, `Express`, `Ekonomis`, dan `Kargo`.

Rumus publik:

```text
NextDay/Express/Ekonomis:
  volumetric_weight = L * W * H / 6000

Kargo:
  volumetric_weight = L * W * H / 5000
  minimum_weight = 10 kg
```

Layanan ekonomis memuat minimum 1 kg. Harga dan coverage tetap berasal dari
rate card/provider. Pastikan policy pembulatan fraksi serta maksimum dimensi
diisi dari kontrak sebelum local calculation diaktifkan penuh.

Sumber:

- [Syarat dan ketentuan Wahana](https://wahana.com/syarat-ketentuan)
- [Layanan ekonomis Wahana](https://wahana.com/layanan/ekonomis)

## 13. RPX

Produk mencakup same day, next day, regular, Heavy Weight Package (`HWP`), dan
Big Helow.

Aturan yang ditemukan pada sumber resmi:

```text
HWP minimum = 20 kg
parcel volumetric divisor = 6000
Big Helow volumetric divisor = 4000
billing base = max(actual, volumetric)
fraksi kg pada rate sheet parcel = dibulatkan ke atas
```

Big Helow memakai tier/rute; jangan mengubahnya menjadi satu harga per kg.
Rate sheet parcel resmi yang tersedia pada peninjauan bertanggal lama, sehingga
formula dapat digunakan sebagai bahan onboarding, bukan harga produksi.
PPN dan surcharge harus menjadi konfigurasi efektif-berlaku, bukan konstanta.

Sumber:

- [Heavy Weight Package](https://www.rpx.co.id/service/domestic-express-en/heavy-weight-package-hwp-en-en)
- [Big Helow](https://www.rpx.co.id/service/bighelow)

## 14. Sentral Cargo

Moda: darat, laut, dan udara.

Rumus pada syarat resmi:

```text
darat/laut volumetric_weight = L * W * H / 4000
udara       volumetric_weight = L * W * H / 6000
```

Syarat pickup menyebut minimum 10 kg pada kondisi tertentu dan kiriman di
bawahnya dapat dikenai minimum 10 kg. Untuk kargo udara per koli di atas 50 kg
terdapat surcharge. Scope minimum dan nilai surcharge harus mengikuti cabang,
rute, dan rate sheet aktif.

Pembulatan diproses bertahap. Sumber resmi memberi contoh 8,11 kg menjadi
8,50 kg dan 7,51 kg menjadi 8 kg, kemudian total 16,50 kg menjadi 17 kg.
Contoh tersebut mengarah pada pembulatan per-koli lalu agregat, tetapi profile
final harus dikonfirmasi lewat rate sheet/kontrak.

Sumber: [Syarat dan ketentuan Sentral Cargo](https://sentralcargo.co.id/syarat-dan-ketentuan).

## 15. SAP Express

Sumber resmi perusahaan mencantumkan same day (`SDS`), one day (`ODS`),
kargo laut/udara, dan dedicated courier. Dokumen yang ditemukan bukan katalog
tarif merchant terkini, sehingga belum cukup untuk menentukan divisor,
pembulatan, minimum, serta SLA per rute.

Aktifkan adapter hanya dengan provider quote atau kontrak. Jangan menyalin
formula dari carrier lain.

Sumber: [Laporan tahunan SAP Express](https://www.sap-express.id/assets/files/AR%20SAP%202020%20%28FINAL%29.pdf).

## 16. REX, NCS, STAR Cargo, dan DSE

### REX

Belum ditemukan sumber publik resmi yang cukup untuk memastikan katalog dan
formula terkini. Wajib meminta dokumentasi merchant.

### NCS

Nama/akronim NCS berisiko mengarah ke perusahaan berbeda. Kunci terlebih
dahulu legal entity, domain, kode aggregator, dan kontak integrasinya. Jangan
aktifkan hanya berdasarkan string `ncs`.

### STAR Cargo

Situs resmi menerangkan transportasi udara, darat, dan laut, tetapi formula
tarif publik belum cukup. Gunakan provider quote.

Sumber: [Tentang STAR Cargo](https://starcargo.co.id/pages/index/tentang-kami).

### DSE

Belum ada sumber publik resmi yang cukup terverifikasi. Simpan kode sebagai
disabled sampai identitas provider, produk, dan dokumen merchant disetujui.

## 17. Syarat aktivasi layanan

Sebuah service hanya boleh berstatus `active=true` jika:

1. kode provider dan nama produk sudah cocok dengan API/rate sheet;
2. coverage origin-destination tersedia;
3. pricing model dan minimum charge diketahui;
4. divisor serta pembulatan diketahui atau dinyatakan tidak berlaku;
5. maksimum berat/dimensi diketahui;
6. surcharge, asuransi, dan pajak memiliki periode efektif;
7. sedikitnya lima quote uji cocok dengan kalkulasi lokal;
8. sumber dan tanggal verifikasi tercatat.

Jika salah satu aturan yang memengaruhi harga belum diketahui, gunakan
`calculation_mode=provider_quote`, bukan nilai default global.
