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
menyembunyikan layanan.

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
minimum tagihan. Minimum tagihan hanya diisi jika sumber layanan dapat
diverifikasi.

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
