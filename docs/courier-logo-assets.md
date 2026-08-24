# Aset logo ekspedisi

Logo ekspedisi disimpan oleh API Kurir di path publik
`/courier-logos/{courier_code}.{extension}`. Aplikasi downstream hanya memakai
URL `logo` dari API dan tidak perlu mengetahui sumber aset atau ekstensi file.

## Sumber awal

- JNE, J&T, SiCepat, ID Express, SAP Express, Ninja Xpress, TIKI, Wahana,
  Pos Indonesia, Sentral Cargo, Lion Parcel, RPX, dan AnterAja memakai salinan
  lokal aset WebP publik Biteship yang diverifikasi pada 24 Agustus 2026.
- REX memakai salinan lokal PNG dari situs resmi PT Royal Express Indonesia.
- Kurir tanpa aset terverifikasi memakai `default.svg`.

API Kurir tidak melakukan hotlink pada saat runtime. URL sumber hanya disimpan
sebagai provenance di `couriers.logo_source_url`, sedangkan klien menerima URL
HTTPS permanen milik API Kurir dari `couriers.logo_url`.

Logo adalah merek milik masing-masing perusahaan ekspedisi dan hanya digunakan
untuk identifikasi pilihan pengiriman. Sebelum penggunaan komersial diperluas,
tim operasional perlu menjaga bukti izin atau ketentuan brand dari pemilik merek.

## Kontrak API

- `GET /v1/admin/couriers` mengembalikan `logo` pada setiap kurir.
- `GET /api/v1/integrations/shipping-services` mengembalikan `logo` pada setiap
  kurir agar Emisell dapat merender pilihan tanpa hardcode aset.
- Respons ongkir kompatibel RajaOngkir tidak ditambah field logo.

Jika sumber mengubah identitas visual, unggah aset baru ke path yang sama lalu
perbarui `logo_source_url` dan `logo_verified_at`. Dengan demikian URL yang
dipakai Emisell tetap stabil dan cache browser dapat diatur melalui deployment.
