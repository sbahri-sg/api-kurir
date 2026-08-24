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
