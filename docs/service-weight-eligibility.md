# Kelayakan Berat Layanan Checkout

Dokumen ini menjelaskan filter minimum dan maksimum berat sebelum sebuah
layanan ekspedisi ditampilkan di checkout Emisell.

## Pembagian tanggung jawab

Emisell menghitung berat final paket, termasuk keputusan dimensi dan berat
volumetrik. Field `weight` yang dikirim ke API Kurir dianggap sudah siap
digunakan sebagai berat pengiriman. API Kurir tidak memvalidasi batas dimensi
checkout pada policy ini.

API Kurir bertanggung jawab untuk:

1. menolak layanan jika `weight` lebih kecil dari minimum penerimaan;
2. menolak layanan jika `weight` melebihi maksimum penerimaan;
3. menerapkan minimum berat tagihan tanpa mengubah berat input Emisell;
4. menerapkan pembulatan dan tarif layanan;
5. hanya mengembalikan layanan yang lolos policy dan pilihan seller.

## Dua minimum yang berbeda

`minimum_accepted_weight_grams` menentukan apakah layanan boleh muncul.
`minimum_billable_weight_grams` menentukan berat terendah yang ditagihkan.

Contoh layanan cargo dengan minimum diterima 3 kg dan minimum tagihan 5 kg:

| Berat dari Emisell | Ditampilkan | Berat tagihan |
|---:|---|---:|
| 1.000 gram | Tidak | - |
| 2.999 gram | Tidak | - |
| 3.000 gram | Ya | 5.000 gram |
| 4.000 gram | Ya | 5.000 gram |
| 6.000 gram | Ya | 6.000 gram sebelum pembulatan provider |

Untuk parcel reguler, paket 800 gram tetap dapat diterima sementara minimum
tagihannya 1.000 gram. Karena itu minimum tagihan tidak boleh dipakai untuk
menyembunyikan layanan. Saat exact quote diperlukan, API Kurir tetap menyimpan
800 gram sebagai berat aktual dan mengirim 1.000 gram sebagai berat tagih
minimum ke provider. Filter cargo tetap mengevaluasi berat aktual.

## Prioritas policy

Policy khusus `courier_code + canonical_service_code` selalu mengalahkan
policy grup. Policy grup `cargo` menjadi pengaman untuk layanan cargo baru yang
belum mempunyai aturan khusus.

```text
service policy
    ↓ jika tidak ada
service_group policy
    ↓ jika tidak ada
tidak ada filter tambahan; exact quote provider tetap berlaku
```

Default grup cargo menolak berat di bawah 3.000 gram, tetapi tidak menetapkan
minimum tagihan. Grup `regular`, `economy`, dan `next_day` mempunyai pengaman
maksimum 50.000 gram. Policy khusus layanan dapat mengalahkan pengaman grup
apabila kontrak atau sumber layanan memberikan batas yang berbeda.

## Urutan checkout

```text
weight final dari Emisell
→ exact quote/rate card
→ klasifikasi canonical service
→ policy minimum/maksimum berat
→ pilihan layanan seller
→ hasil checkout
```

Layanan yang gagal policy tidak dimasukkan ke `data`. Jika seluruh layanan
gagal, endpoint mengembalikan `RATE_NOT_AVAILABLE`.

Endpoint canonical `/v1/calculate/domestic-cost` menambahkan informasi policy
pada `weight` dan `eligibility`. Endpoint `/api/v1/...` tetap mempertahankan
struktur RajaOngkir V2; policy hanya memfilter isi array `data`.

## Sumber dan lifecycle

Setiap policy menyimpan `source_type`, `source_reference`, `verified_at`,
`effective_from`, dan `effective_until`. Nilai dari dokumen marketplace hanya
berstatus referensi sampai dikonfirmasi melalui provider API, kontrak, atau
dokumen resmi ekspedisi.

Perubahan policy dibuat sebagai versi baru. Data tarif dan policy berat tidak
dicampur: tarif dapat berubah per rute, sedangkan policy berat berlaku pada
layanan sesuai masa efektifnya.

## SOP operasional checkout

1. Emisell menjumlahkan berat barang dan mengirim `weight` final dalam gram.
2. API Kurir mengambil exact quote atau snapshot quote dari provider.
3. API Kurir mencocokkan service code provider ke canonical service dan grup.
4. Policy khusus service diperiksa lebih dahulu, lalu fallback grup.
5. Layanan di bawah minimum atau di atas maksimum tidak dikirim ke checkout.
6. Layanan yang lolos disaring lagi dengan pilihan service milik seller.
7. Harga exact quote provider tidak ditulis ulang oleh policy marketplace.

Contoh hasil yang wajib dijaga oleh regression test:

| Berat final | Hasil checkout |
|---:|---|
| 1 kg | Cargo dengan minimum 3 kg disembunyikan |
| 3 kg | Anteraja Cargo dapat tampil; minimum tagihan referensi 5 kg |
| 80 kg | Regular, economy, dan next day default disembunyikan; hanya cargo yang batas maksimumnya mencukupi |
| Di atas maksimum service | Service tersebut disembunyikan walaupun seller mengaktifkannya |

## Matriks referensi kargo Emisell

| Layanan canonical | Minimum diterima | Minimum tagihan | Maksimum | Status sumber |
|---|---:|---:|---:|---|
| Anteraja `BIG` | 3 kg | 5 kg | 100 kg | Referensi kanal Shopee; perlu konfirmasi kontrak |
| JNE `JTR` | 3 kg | 5 kg | 600 kg | Referensi kanal Shopee; perlu konfirmasi kontrak |
| SiCepat `GOKIL` | 3 kg | 5 kg | 50 kg | Referensi kanal Shopee; perlu konfirmasi kontrak |
| Sentral `DARAT/LAUT/UDARA` | 5 kg | Mengikuti quote | Mengikuti quote | Referensi kanal Shopee; perlu konfirmasi kontrak |
| Wahana `KARGO` | 10 kg | 10 kg | 50 kg | Publik resmi |
| TIKI `TRC` | 10 kg | 10 kg | Mengikuti quote | Publik resmi |
| RPX `HWP` | 20 kg | 20 kg | 50 kg | Publik resmi |
| SAPX `CARGO` | 5 kg | Mengikuti quote | Mengikuti quote | Publik; perlu konfirmasi kontrak |

Shopee menempatkan economy dan cargo dalam kelompok visual “Hemat Kargo”. API
Kurir tetap memisahkan keduanya menjadi grup `economy` dan `cargo`, karena
batas berat, SLA, dan service yang dapat diaktifkan seller berbeda.

`J&T Cargo` pada kanal Shopee bukan otomatis sama dengan canonical service
`J&T HBO` dari katalog RajaOngkir. Service tersebut tidak boleh dibuat hanya
dari kemiripan nama; tunggu sampai code provider tersedia dan terverifikasi.
