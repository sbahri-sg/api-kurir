# Biteship sebagai fallback tracking

Biteship di API Kurir hanya dipakai untuk melacak resi kurir yang belum
tersedia pada adapter tracking RajaOngkir. Integrasi ini **tidak** dipakai
untuk cek ongkir, sinkronisasi katalog kurir, rate card, atau pengiriman
instan.

## Alur pemilihan provider

1. API Kurir mencari snapshot tracking yang masih dapat digunakan.
2. Jika perlu mengambil data provider, router memilih adapter berdasarkan kode
   kurir.
3. Kurir bawaan RajaOngkir tetap memakai RajaOngkir.
4. `sicepat`, `ide`, `rpx`, dan `sentral`
   memakai Biteship.
5. Jika suatu kurir sengaja didaftarkan pada kedua adapter, Biteship hanya
   dicoba saat provider pertama tidak memiliki data, sedang tidak tersedia,
   atau timeout. Error credential, rate limit, dan kuota tidak dibypass.

Daftar fallback dapat diubah melalui:

```dotenv
BITESHIP_TRACKING_COURIERS=ide,rpx,sentral,sicepat
```

Startup akan ditolak bila daftar ini tumpang tindih dengan
`RAJAONGKIR_TRACKING_COURIERS`. Guard tersebut memastikan Biteship hanya
menangani celah tracking dan tidak menggantikan kurir yang sudah dicakup
RajaOngkir.

Grab dan GoSend sengaja ditolak dari daftar ini karena belum termasuk lingkup
fallback tracking reguler.

Halaman Cek Resi membaca `supports_tracking` dan `tracking_provider_code` dari
katalog API Kurir. Paxel dicatat sebagai kurir **tracking-only** Biteship.
AnterAja, IDExpress, RPX, Sentral Cargo, dan SiCepat tetap memakai RajaOngkir
untuk cek ongkir, sedangkan cek resinya memakai Biteship fallback.

J&T Cargo tidak dimasukkan karena kode tersebut belum dapat diverifikasi pada
katalog resmi Biteship yang ditinjau. Master tidak boleh menampilkan kurir
berdasarkan asumsi.

## Matriks provider

| Fungsi | Provider | Kurir |
|---|---|---|
| Cek ongkir | RajaOngkir | 17 kode resmi RajaOngkir |
| Cek resi utama | RajaOngkir | JNE, SAP, Ninja, J&T, TIKI, Wahana, POS, Lion |
| Cek resi fallback | Biteship | AnterAja, IDExpress, Paxel, RPX, Sentral Cargo, SiCepat |

Field katalog membedakan `rate_provider_code` dan
`tracking_provider_code`. Karena itu Biteship tidak akan pernah dipilih oleh
rate engine walaupun kurir tersebut memakai Biteship untuk tracking.

## Menambahkan token

Masuk ke **Kuota provider → Tambah key platform**, pilih
**Biteship · tracking fallback**, lalu tempel token `biteship_live.*` atau
`biteship_test.*`. Token akan:

- diverifikasi melalui katalog kurir Biteship, bukan endpoint tracking
  berbayar;
- dienkripsi dengan `PROVIDER_CREDENTIAL_ENCRYPTION_KEY`;
- tidak pernah ditampilkan kembali;
- dapat dinonaktifkan tanpa restart service.

Token tidak disimpan di `.env` dan tidak boleh dikirim ke frontend seller.

## Endpoint API Kurir

Kontrak publik tidak berubah. Emisell tetap memanggil endpoint kompatibel
RajaOngkir V2:

```http
POST /api/v1/track/waybill
Content-Type: application/x-www-form-urlencoded
key: <api-key-api-kurir>

awb=nomor_resi&courier=sicepat
```

Respons endpoint kompatibilitas tetap 1:1 dengan envelope RajaOngkir V2 dan
tidak menambahkan field provider baru. Sumber aktual dapat diaudit pada ledger
provider dan pada respons tracking internal (`provider=biteship`). Status
provider dinormalisasi menjadi `pending_pickup`, `picked_up`, `in_transit`,
`out_for_delivery`, `delivered`, `returned`, atau `cancelled`.

## Efisiensi biaya

Endpoint public tracking Biteship dikenakan biaya per hit. API Kurir mencatat
hit pada ledger lokal dan menyimpan snapshot hasil:

| Kondisi | Refresh paling cepat |
|---|---:|
| Sedang diantar | 2 jam |
| Dalam perjalanan / sudah dipickup | 12 jam |
| Status awal pertama | 12 jam |
| Status awal berikutnya | 24 jam |
| Final (terkirim, retur, batal) | Tidak di-refresh otomatis |

Permintaan berulang sebelum `next_refresh_at` dilayani dari database dan tidak
memanggil Biteship lagi. Polling berhenti ketika batas default 10 hit per AWB
tercapai.

## Referensi resmi

- [Biteship Public Tracking](https://biteship.com/id/docs/api/trackings/status)
- [Biteship Tracking Overview](https://biteship.com/id/docs/api/trackings/overview)
- [Biteship Authentication](https://biteship.com/id/docs/api/authentication)
- [Biaya mode testing Biteship](https://help.biteship.com/hc/id/articles/58286997471513-Kebijakan-Biaya-Mode-Testing)
