# Biteship sebagai fallback Emisell Kurir

Biteship adalah provider internal kedua di balik extension **Emisell Kurir**.
Seller tetap melihat satu provider bernama Emisell Kurir; pemilihan provider
upstream dilakukan API Kurir.

```text
Emisell Kurir -> RajaOngkir (utama) -> Biteship (fallback)
RajaOngkir BYOK seller -------------> RajaOngkir seller saja
```

Biteship tidak ditampilkan sebagai extension seller, tidak menangani booking,
dan tidak mencakup Grab, GoSend, Borzo, atau kurir instan lain.

## Kebijakan fallback

| Kondisi | RajaOngkir utama | Biteship fallback |
|---|---:|---:|
| Quote/tracking berhasil | Dipakai | Tidak dipanggil |
| Kurir atau rute tidak tersedia | Dicoba | Dicoba setelah gagal |
| Timeout/provider unavailable | Dicoba | Dicoba setelah gagal |
| Kuota/rate limit/key platform bermasalah | Dicoba | Dicoba setelah gagal |
| Seller mengaktifkan RajaOngkir BYOK | Dipakai | Dilarang |
| Emisell Kurir tidak aktif | Tidak dipanggil | Tidak dipanggil |

Fallback dilakukan berurutan, bukan paralel. Untuk cek ongkir multi-kurir,
Biteship hanya menerima kode kurir yang belum mempunyai hasil dari RajaOngkir.
Ini mencegah dua hit untuk kurir yang sudah berhasil dijawab provider utama.

Untuk mencegah antrean panjang saat RajaOngkir mengalami gangguan platform,
API Kurir membuka circuit setelah tiga kegagalan beruntun dan mengarahkan
request Emisell Kurir berikutnya langsung ke Biteship selama 30 detik. Setelah
itu hanya satu request yang menjadi recovery probe; request paralel lain tetap
memakai fallback. Circuit hanya berlaku untuk credential platform Emisell,
tidak pernah untuk credential BYOK merchant. Circuit juga tidak dibuka bila
Biteship belum siap atau tidak menghasilkan quote, sehingga RajaOngkir tetap
dapat dicoba dan tidak tercipta total outage akibat backup yang bermasalah.

Saat Redis belum diaktifkan, deduplikasi request provider memakai lock lokal
proses dan tidak menahan koneksi PostgreSQL selama network call. Ini menjaga
pool database tetap tersedia untuk autentikasi merchant, pemilihan credential,
lokasi, dan kuota ketika banyak customer checkout bersamaan. Deployment dengan
lebih dari satu instance API wajib mengaktifkan Redis agar lock berlaku lintas
instance.

Quote RajaOngkir dari credential platform disimpan sebagai snapshot bersama
antar merchant Emisell Kurir. Data mentah yang dibagikan hanya ditentukan oleh
rute, berat, dimensi, kurir, dan opsi harga; otorisasi serta filter layanan
seller tetap dijalankan terpisah pada setiap checkout. Snapshot yang sering
diakses disimpan lima menit di Redis agar checkout cache-hit tidak selalu
membaca PostgreSQL.

Katalog kurir, nama layanan, policy berat, dan rate card lokal juga dicache
lima menit. Pada cache miss, request identik dalam satu instance digabung
menjadi satu pembacaan database. Dengan demikian lonjakan checkout tidak
mengubah seribu customer menjadi seribu query referensi yang sama.

Kurir yang diketahui lambat atau tidak tersedia pada endpoint tarif
RajaOngkir dapat dilewati melalui `RAJAONGKIR_RATE_BYPASS_COURIERS`. Nilai
default saat ini `tiki`, berdasarkan pengujian produksi: empat kurir lain
selesai sekitar 0,34 detik sedangkan TIKI menunggu sekitar 10 detik lalu
ditolak upstream. Bypass hanya berlaku pada Emisell Kurir dan kekurangannya
diisi Biteship; RajaOngkir BYOK tidak diubah.

## Fallback cek ongkir

API Kurir memakai endpoint Biteship `POST /v1/rates/couriers`. Lokasi lokal
tidak diinput manual:

1. API Kurir mencari mapping `location_public_id -> Biteship area_id` lokal.
2. Saat mapping belum ada, API Kurir mencari area lewat Maps API.
3. Hasil hanya diterima bila provinsi, kota/kabupaten, kecamatan, dan kode pos
   cocok dengan master wilayah lokal.
4. Mapping disimpan sehingga request berikutnya tidak mengulang Maps API.
5. Quote disimpan sebagai snapshot platform `provider_code=biteship`, terpisah
   dari snapshot RajaOngkir dan dapat dipakai ulang lintas merchant Emisell.

Kurir fallback tarif yang telah dipetakan adalah AnterAja, IDExpress, JNE,
J&T, Lion Parcel, Ninja, POS Indonesia, RPX, SAPX, Sentral Cargo, SiCepat,
TIKI, dan Wahana. Hanya service alias yang telah ditinjau yang dikirim ke
Emisell, dan hasilnya dibatasi ke empat group:

- `regular`
- `next_day`
- `economy`
- `cargo`

Same day, express non-next-day, international, special, dan instant diabaikan.
Respons Main Service tidak berubah; `source_provider=biteship` hanya menjadi
metadata audit internal pada snapshot/rate result API Kurir.

## Fallback tracking

Router tracking menyatukan coverage RajaOngkir dengan daftar Biteship. Karena
kurir yang sama boleh ada di kedua adapter, RajaOngkir selalu dicoba lebih
dahulu dan Biteship baru dipanggil jika hasil utama tidak tersedia. Daftar
tambahan default:

```dotenv
BITESHIP_TRACKING_COURIERS=ide,rpx,sentral,sicepat
```

API dan worker menggunakan aturan yang sama. Status Biteship dinormalisasi
menjadi `pending_pickup`, `picked_up`, `in_transit`, `out_for_delivery`,
`delivered`, `returned`, atau `cancelled` sebelum snapshot/webhook dikirim ke
Emisell.

## Konfigurasi

Masuk ke **Kuota provider -> Tambah key platform**, pilih
**Biteship - fallback Emisell Kurir**, lalu tempel token `biteship_live.*` atau
`biteship_test.*`. Token diverifikasi melalui katalog resmi, dienkripsi, tidak
ditampilkan kembali, dan dapat diganti tanpa restart.

```dotenv
BITESHIP_BASE_URL=https://api.biteship.com/
BITESHIP_TIMEOUT=5s
BITESHIP_RATE_FALLBACK_ENABLED=true
BITESHIP_RATE_SNAPSHOT_TTL=336h
BITESHIP_TRACKING_COURIERS=ide,rpx,sentral,sicepat
RAJAONGKIR_HOSTED_TIMEOUT=15s
RAJAONGKIR_HOSTED_TIMEOUT_MS=15000
RAJAONGKIR_RATE_CIRCUIT_FAILURE_THRESHOLD=3
RAJAONGKIR_RATE_CIRCUIT_OPEN_DURATION=30s
RAJAONGKIR_RATE_BYPASS_COURIERS=tiki
REDIS_ENABLED=true
RATE_SNAPSHOT_CACHE_TTL=5m
```

Menonaktifkan `BITESHIP_RATE_FALLBACK_ENABLED` hanya mematikan fallback tarif;
tracking tetap mengikuti konfigurasi daftar kurir dan keberadaan credential.
Token tidak boleh disimpan di frontend atau dikirim oleh Main Service.

## Efisiensi biaya

Tarif dan tracking memakai snapshot PostgreSQL. Request yang cocok dan masih
fresh dibaca dari database tanpa memanggil provider. Mapping area Biteship juga
disimpan permanen sampai operator melakukan remapping.

| Data | Kebijakan default |
|---|---|
| Tarif Biteship | Snapshot 14 hari |
| Tracking sedang diantar | Refresh paling cepat 2 jam |
| Tracking dalam perjalanan | Refresh paling cepat 12 jam |
| Status final | Tidak di-refresh otomatis |
| Batas siklus tracking | Maksimal 10 hit provider per AWB |

Semua hit Maps, Rates, dan Tracking dicatat pada ledger Biteship agar biaya dan
fallback dapat diaudit.

## Kontrak API Emisell

Tidak ada endpoint baru untuk Main Service. Endpoint cek ongkir dan tracking
tetap memakai kontrak kompatibel RajaOngkir V2. Pemilihan upstream bersifat
internal dan tidak menambah field wajib pada request Emisell.

## Referensi resmi

- [Biteship Retrieve Rates](https://biteship.com/id/docs/api/rates/retrieve)
- [Biteship Maps API](https://biteship.com/id/docs/api/maps/overview)
- [Biteship Courier Catalog](https://biteship.com/id/docs/api/couriers/overview)
- [Biteship Public Tracking](https://biteship.com/id/docs/api/trackings/status)
- [Biteship Authentication](https://biteship.com/id/docs/api/authentication)
