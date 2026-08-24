# Register Sumber dan Verifikasi

Tanggal peninjauan: **20 Agustus 2026**.

Dokumen ini mencatat sumber publik yang dipakai untuk membangun katalog awal.
Harga rute produksi tetap harus berasal dari API/rate sheet/kontrak yang masih
aktif.

## Master wilayah lokal

| Subjek | Sumber | Snapshot yang dipakai | Catatan |
|---|---|---|---|
| Kode wilayah Kemendagri | [cahyadsn/wilayah](https://github.com/cahyadsn/wilayah) | Commit `f1aad011720e15de4633fefac62b3b14ebb66db8`, SHA-256 `c4c3396d...e954031` | Mirror MIT; importer memvalidasi 91.599 record dan hierarki |
| Mapping kode pos | [cahyadsn/wilayah_kodepos](https://github.com/cahyadsn/wilayah_kodepos) | Commit `ba8497156c5cc9bcbfc527f7b8875d403eda2354`, SHA-256 `fdb972e6...34e898e` | Mirror MIT; 83.762 mapping desa/kelurahan |
| Verifikasi kode pos | [Pos Indonesia](https://kodepos.posindonesia.co.id/) | Pembanding operasional | GitHub adalah bootstrap; audit berkala terhadap sumber resmi tetap diperlukan |

Commit dan checksum lengkap juga disimpan di tabel
`location_dataset_imports`. Mengganti snapshot wajib melalui review jumlah
wilayah, parent-child, kode baru/hilang, dan kode pos berubah.

## Platform dan aggregator

Dokumentasi provider pada bagian ini dipakai untuk memahami fitur dan
menyusun kontrak canonical. Pencantuman sumber tidak berarti API Kurir akan
membuat adapter native. Untuk onboarding baru, provider wajib menyediakan
Partner Connector API dan menangani API native mereka sendiri.

| Subjek | Sumber | Dipakai untuk | Catatan |
|---|---|---|---|
| RajaOngkir | [Pricing](https://rajaongkir.com/pricing) | Paket dan batas hit yang ditampilkan | Verifikasi kembali saat kontrak dibuat |
| RajaOngkir | [Terms & Conditions](https://rajaongkir.com/terms-condition) | Batas penggunaan, layanan serupa, redistribusi | Wajib izin tertulis untuk model bisnis ini |
| RajaOngkir | [Calculate Cost](https://rajaongkir.com/docs/shipping-cost/endpoint-rajaongkir-for-form-base-calculate-cost/calculate-cost) | Bentuk endpoint dan multi-courier | Respons quote bukan otomatis rate per kg |
| RajaOngkir | [Daftar paket dan kurir domestik](https://rajaongkir.com/) | Daftar 17 kode cek ongkir termasuk `anteraja`, `rpx`, `ncs`, `star`, dan `dse` | Lebih baru tetapi tidak konsisten dengan tabel Courier Availability; verifikasi quote per rute |
| RajaOngkir V2 | [Endpoint](https://www.rajaongkir.com/docs/shipping-cost/getting_started/endpoint) | Base URL serta path rate, destination, dan tracking | Base URL aktif `rajaongkir.komerce.id/api/v1` |
| RajaOngkir V2 | [Authorization](https://rajaongkir.com/docs/shipping-cost/getting_started/apikey) | Header autentikasi `key` dan keamanan credential | Key disimpan terenkripsi melalui lifecycle credential atau secret manager fallback |
| RajaOngkir V2 | [Calculate Domestic](https://rajaongkir.com/docs/shipping-cost/endpoint-rajaongkir-for-search-base/calculate-domestic-cost) | Form origin, destination, weight, courier; struktur quote | Endpoint publik tidak mendokumentasikan formula rate per kg |
| RajaOngkir V2 | [Search Destination](https://rajaongkir.com/docs/shipping-cost/endpoint-rajaongkir-for-search-base/search-destination-rajaongkir) | Import lokasi on-demand dan provider ID | Tidak dipanggil saat customer mengetik |
| RajaOngkir V2 | [Courier Availability](https://www.rajaongkir.com/docs/shipping-cost/getting_started/courier_availability) | Kode dan capability kurir | Capability dapat berubah; sinkronkan berkala secara terkontrol |
| RajaOngkir V2 | [Tracking AWB](https://www.rajaongkir.com/docs/shipping-cost/tracking) | Parameter AWB, courier, validasi nomor telepon, struktur summary/manifest | Dipanggil sinkron saat snapshot miss/stale atau oleh worker; raw response tidak disimpan |
| RajaOngkir Web | [Lacak Resi](https://rajaongkir.com/lacak-resi) | Daftar tracking aktual mencakup `anteraja` melalui endpoint katalog halaman publik | Dipakai karena tabel Courier Availability belum sinkron dengan daftar halaman publik |
| Shopee Seller Centre | [Pengaturan Jasa Kirim](https://seller.shopee.co.id/portal/all-settings/shipping/shipping-channel) | Referensi pemisahan minimum diterima, minimum tagihan, maksimum berat, COD, dan metode pickup/drop-off per layanan | Referensi marketplace, bukan aturan authoritative Emisell; angka harus dikonfirmasi ke provider sebelum berstatus resmi |
| AnterAja | [Layanan resmi](https://anteraja.id/id/services), [API service rates](https://developer.anteraja.id/) | Klasifikasi `DOK`, `ECO`, `MIC`, `ND`, `REG`, serta kode layanan lain yang diobservasi dari RajaOngkir | Data master diisi otomatis dari quote; sumber resmi menentukan grup service |
| 21 Express / DSE | [Layanan resmi](https://www.21express.co.id/layanan-kami) | Katalog Regular, Over Night, Same Day, International, dan City Courier | Harga dan coverage tetap berasal dari quote RajaOngkir |
| NCS | [Produk dan layanan resmi](https://ncskurir.com/ncskurir/product-service) | Katalog Regular, Overnight, Same Day, Regular Darat, NFD, dan International | Harga dan coverage tetap berasal dari quote RajaOngkir |
| RPX | [Domestic Express](https://www.rpx.co.id/service/domestic-express-id) | Katalog SDP, MDP, NDP, RGP, HWP, ECP, dan HCP | Harga dan coverage tetap berasal dari quote RajaOngkir |
| STAR Cargo | [Tentang dan moda resmi](https://starcargo.co.id/pages/index/tentang-kami) | Katalog cargo multimoda udara, darat, dan laut | Harga dan coverage tetap berasal dari quote RajaOngkir |
| RajaOngkir Shipping Delivery | [Endpoint](https://rajaongkir.com/docs/delivery-order-api/getting_started/base-url), [Authorization](https://www.rajaongkir.com/docs/delivery-order-api/getting_started/api-key) | Pemisahan sandbox/live, produk, key, rate, order, pickup, cancel, detail, history, label, webhook | Produk/key berbeda dari Shipping Cost dan memerlukan Enterprise/live approval |
| RajaOngkir Shipping Delivery | [Calculate](https://www.rajaongkir.com/docs/delivery-order-api/calculate), [Store Order](https://rajaongkir.com/docs/delivery-order-api/Store_order/store_order) | Rate booking regular/cargo/instant, pinpoint, order, COD/Bank Transfer | Quote Delivery dikunci; tidak memakai harga Shipping Cost untuk booking |
| RajaOngkir Shipping Delivery | [Pickup](https://rajaongkir.com/docs/delivery-order-api/pickup_order), [Label](https://www.rajaongkir.com/docs/delivery-order-api/label_order) | Pickup batch parsial, AWB, label bulk dan layout | Retry per item; PDF/base64 tidak dicatat ke log |
| RajaOngkir Shipping Delivery | [History AWB](https://www.rajaongkir.com/docs/delivery-order-api/history_awb), [Webhook](https://www.rajaongkir.com/docs/delivery-order-api/webhook) | Timeline order dan notifikasi status | Webhook publik belum mendokumentasikan HMAC/delivery ID; wajib rekonsiliasi |
| RajaOngkir Shipping Delivery | [GoSend pricing](https://www.rajaongkir.com/docs/delivery-order-api/Store_order/gosend_pricing) | Dynamic pricing instant dan koreksi biaya | Simpan quoted/booked/actual cost terpisah |
| Biteship | [Retrieve Rates](https://biteship.com/id/docs/api/rates/retrieve), [Maps API](https://biteship.com/id/docs/api/maps/overview) | Fallback tarif Emisell Kurir dan mapping area Biteship otomatis | Area ID dipilih dengan kecocokan provinsi, kota, kecamatan, dan kode pos; hasil tarif disimpan sebagai snapshot provider terpisah |
| Biteship | [Public Tracking](https://biteship.com/id/docs/api/trackings/status), [Overview](https://biteship.com/id/docs/api/trackings/overview) | Fallback tracking Emisell Kurir saat RajaOngkir tidak mencakup kurir atau gagal | Endpoint public tracking berbayar per hit; snapshot dan refresh konservatif wajib dipakai |
| Biteship | [Courier catalog](https://biteship.com/id/docs/api/couriers/overview) | Verifikasi kode kurir dan service fallback tarif/tracking | Instant courier tidak masuk cakupan; hanya alias service yang telah ditinjau yang boleh dikirim ke Emisell |
| Biteship | [Authentication](https://biteship.com/id/docs/api/authentication), [biaya mode testing](https://help.biteship.com/hc/id/articles/58286997471513-Kebijakan-Biaya-Mode-Testing) | Validasi token dan model biaya | Token terenkripsi di database; validasi credential tidak memanggil endpoint tracking |
| KiriminAja | [Mitra API](https://developer.kiriminaja.com/docs/introduction) | Referensi rate, order, pickup, cancel, tracking, COD, dan sandbox | Referensi capability; onboarding tetap melalui connector canonical milik partner |
| KiriminAja | [Webhook Express](https://developer.kiriminaja.com/docs/webhook/event) | Event AWB/status, Bearer callback, dedup AWB + order ID | Belum mendokumentasikan HMAC/timestamp/nonce; perlu compensating controls |
| KiriminAja | [Pricing Express](https://developer.kiriminaja.com/docs/pricing/express), [Courier Detail](https://developer.kiriminaja.com/docs/others/courier-detail) | Cost, COD/asuransi, discount, group, cut-off, volumetric, rounding | Metadata disinkronkan; harga authoritative tetap quote |
| KiriminAja | [Instant Order](https://developer.kiriminaja.com/docs/order/instant), [Instant Tracking](https://developer.kiriminaja.com/docs/order/tracking-instant), [Instant Webhook](https://developer.kiriminaja.com/docs/webhook/event-instant) | GoSend/Grab/Borzo, koordinat, vehicle, driver, live tracking | Webhook utama; pull tracking fallback |
| KiriminAja | [Payment](https://developer.kiriminaja.com/docs/payment), [KA Credit](https://developer.kiriminaja.com/docs/payment/ka-credit), [PIN](https://developer.kiriminaja.com/docs/payment/pin-validation) | QRIS inquiry, saldo, PIN, status payment | PIN write-only dan tidak pernah disimpan |
| KiriminAja | [Status Mapping](https://developer.kiriminaja.com/docs/important-notes/status-mapping) | Attempt, problem, return, lost, damaged, final state | Simpan raw code dan canonical status |
| KiriminAja | [Syarat dan ketentuan](https://kiriminaja.com/syarat-ketentuan), [kebijakan privasi](https://kiriminaja.com/privacy-policy) | Akun, tanggung jawab pengiriman, penggunaan data, dan retensi | Kontrak komersial/DPA wajib sebelum data merchant production diproses |
| Karrio | [Repository resmi](https://github.com/karrioapi/karrio) | Orkestrasi carrier, lisensi core | Multi-tenancy tetap tanggung jawab API Kurir |

## Framework dan infrastruktur

| Subjek | Sumber resmi | Dipakai untuk |
|---|---|---|
| Go | [Effective Go: Concurrency](https://go.dev/doc/effective_go#concurrency) | Model goroutine, channel, dan concurrency |
| Echo | [Dokumentasi Echo](https://echo.labstack.com/) | HTTP framework API Kurir |
| Vite | [Panduan Vite](https://vite.dev/guide/) | Build tool dashboard |
| River | [Dokumentasi River](https://riverqueue.com/docs) | Durable background job berbasis PostgreSQL |
| pgx | [Repository pgx](https://github.com/jackc/pgx) | Driver dan connection pool PostgreSQL untuk Go |
| sqlc | [Dokumentasi sqlc](https://docs.sqlc.dev/) | Go code dari SQL yang tervalidasi |
| OpenTelemetry | [Dokumentasi OpenTelemetry](https://opentelemetry.io/docs/) | Trace, metric, dan context propagation |

Versi runtime dan library dikunci pada manifest serta lockfile repository.
Gunakan versi stabil yang masih menerima security update; jangan mengandalkan
label “latest” di deployment produksi.

## Carrier

| Carrier | Sumber resmi | Data yang diverifikasi | Keterbatasan |
|---|---|---|---|
| JNE | [Produk dan layanan](https://www.jne.co.id/produk-dan-layanan), [JNE Trucking](https://www.jne.co.id/jtr-id) | REG, YES, SPS, JTR dan produk khusus; JTR min 10 kg, divisor 5.000, pembulatan, surcharge berat besar | Kode varian, harga, dan coverage per rute tetap dari quote/rate sheet |
| TIKI | [Produk TIKI](https://www.tiki.id/id/produk), [T15/T25](https://www.tiki.id/id/blog/1512/nggak-mau-ribet-kirim-motor-coba-t15-dan-t25-dari-tiki) | SDS, ONS, REG, ECO, TRC, INT; TRC min 10 kg; T15/T25 varian motor | T60 teramati dari API provider; divisor/rounding belum lengkap |
| SiCepat | [GOKIL](https://ekspres.sicepat.com/services/GOKIL), [ringkasan resmi](https://sahabatsicepat.com/sicepat-ekspres-layanan-ke-semua-segmen/) | Produk dan minimum GOKIL 10 kg | Formula rinci perlu kontrak |
| J&T Express | [Ketentuan layanan](https://jet.co.id/information/terms/jtsuper) | Produk, divisor 6.000, DOC, SUPER, packing | Ada redaksi dimensi ambigu; konfirmasi merchant |
| ID Express | [Situs utama](https://idexpress.com/), [syarat](https://idexpress.com/bantuan/syarat-dan-ketentuan) | Lite/Regular/Cargo, kode `Idtruck` dari quote provider, dan pembulatan | Divisor belum terverifikasi |
| Ninja Xpress | [Standard terms](https://www.ninjaxpress.co/id-id/standard-delivery-terms), [perhitungan ongkir](https://www.ninjaxpress.co/id-id/support/shipper-support/pricing-and-coverage/how-to-calculate-and-check-shipping-rates) | Maks 30 kg, divisor 6.000, pembulatan, Ninja Care | Scope aturan per produk perlu diuji |
| Lion Parcel | [Produk](https://lionparcel.com/product/) | Kategori produk, estimasi, Light 300 g, large mulai 10 kg | Kode/formula akun perlu sinkronisasi |
| AnterAja | [Layanan](https://anteraja.id/id/services) | Produk, estimasi, Mini Cargo >4 kg | Divisor dan minimum charge perlu kontrak |
| Pos Indonesia | [Pos Reguler](https://www.posindonesia.co.id/id/pages/pos-reguler), [syarat domestik](https://www.posindonesia.co.id/id/pages/syarat-dan-ketentuan-kiriman-domestik) | H+2–H+4, maks 50 kg, tracking/asuransi, dangerous/valuable goods | Kode aktual dikonfirmasi melalui quote provider |
| Wahana | [Syarat](https://wahana.com/syarat-ketentuan), [Ekonomis](https://wahana.com/layanan/ekonomis) | Divisor 6.000/5.000, cargo min 10 kg | Pembulatan dan batas detail perlu kontrak |
| RPX | [Domestic Express](https://www.rpx.co.id/service/domestic-express-id), [HWP](https://www.rpx.co.id/service/domestic-express-en/heavy-weight-package-hwp-en-en), [Big Helow](https://www.rpx.co.id/service/bighelow) | SDP, MDP, NDP, RGP, HWP min 20 kg, ECP min 10 kg, HCP; Big Helow divisor 4.000 | Rate sheet publik lama tidak boleh jadi harga produksi |
| Sentral Cargo | [Syarat dan ketentuan](https://sentralcargo.co.id/syarat-dan-ketentuan) | Divisor darat/laut 4.000, udara 6.000, minimum/surcharge | Scope mengikuti cabang/rute |
| SAP Express | [Situs resmi SAPX](https://www.sapx.id/id), [laporan tahunan resmi](https://www.sap-express.id/assets/files/AR%20SAP%202020%20%28FINAL%29.pdf) | Regular, SDS, ODS, kargo, internasional, dedicated; alias `UDRREG`, `UDRONS`, `DRGREG` dari quote | Formula tetap perlu katalog merchant |
| STAR Cargo | [Tentang perusahaan](https://starcargo.co.id/pages/index/tentang-kami) | Moda udara, darat, laut | Formula belum tersedia |
| REX | [Layanan resmi](https://rex.co.id/en/services), [brosur resmi](https://www.rex.co.id/public/files/file/Brosur_REX.pdf) | REX-0, REX-1, express, regular, international, other; `REX-10` dari quote provider | REX-10 dipisahkan sebagai cargo; formula dan coverage tetap provider quote |
| NCS | [Produk dan layanan](https://ncskurir.com/ncskurir/product-service) | Regular, Overnight, Same Day, Regular Darat min 10 kg, NFD, International | Formula per layanan tetap provider quote/kontrak |
| DSE / 21 Express | [Layanan resmi](https://www.21express.co.id/layanan-kami) | Regular, Over Night, Same Day, International, City Courier | Formula dan coverage tetap provider quote/kontrak |

## Sumber khusus pembulatan

| Provider/service | Sumber resmi | Boundary yang dicatat |
|---|---|---|
| JNE JTR | [JTR](https://www.jne.co.id/jtr-indonesia) | `<1,30` turun; `>=1,30` naik; minimum 10 kg |
| J&T umum | [Panduan kalkulasi](https://help.jet.co.id/web-show/NXRTVktVTE4xOHRoWHJMdDRFRldsdz09) | `<1,30` menjadi 1; `>1,30` menjadi 2; tepat 1,30 ambigu |
| J&T SUPER | [Ketentuan SUPER](https://help.jet.co.id/web-show/VGRCM1hBOVNtdVdjclVQWTVTUTFkZz09) | `1–1,30` mengikuti 1 kg |
| ID Express | [Syarat](https://idexpress.com/bantuan/syarat-dan-ketentuan) | `0–1,30` menjadi 1; `1,31–2,30` menjadi 2 |
| Ninja help center | [Perhitungan ongkir](https://www.ninjaxpress.co/id-id/support/shipper-support/pricing-and-coverage/how-to-calculate-and-check-shipping-rates) | Fraksi `<0,30` turun; `>=0,30` naik |
| Ninja standard | [Standard terms](https://www.ninjaxpress.co/id-id/standard-delivery-terms) | Berat aktual dibulatkan ke atas; scope perlu kontrak |
| Lion internasional | [Pengiriman internasional](https://lionparcel.com/pengiriman-internasional) | 1,01 kg menjadi 2 kg; tidak untuk domestik |
| RPX | [Big Helow](https://rpx.co.id/service/bighelow) | Setiap pecahan desimal naik |
| Sentral Cargo | [Syarat](https://sentralcargo.co.id/syarat-dan-ketentuan) | Contoh mengarah ke 0,11 naik ke setengah kg per koli, lalu total naik; perlu konfirmasi |

## Tingkat kekuatan bukti

Urutan dari terkuat:

1. kontrak/rate sheet/API merchant yang aktif;
2. syarat layanan resmi carrier;
3. halaman produk resmi;
4. quote yang diamati;
5. sumber komunitas atau repository scraper.

Level 4 hanya boleh menghasilkan `verification_status=observed`. Level 5 tidak
boleh menjadi dasar harga produksi.

## Bukti yang harus disimpan saat onboarding

Untuk setiap provider:

```text
provider
document_title
document_version
source_url_or_contract_id
retrieved_at
effective_from
effective_until
checksum
approved_by
notes
```

Salinan kontrak dan rate sheet disimpan di penyimpanan privat. Database hanya
menyimpan metadata dan checksum; jangan memasukkan credential ke dokumen.

## Agenda konfirmasi provider

Pertanyaan wajib:

1. Apakah caching tarif dan tracking diizinkan, serta berapa lama?
2. Apakah data dapat digunakan oleh semua tenant Emisell?
3. Apakah ada rate limit per detik, menit, hari, IP, atau credential?
4. Apakah tracking menyediakan webhook?
5. Apa kode service resmi dan lifecycle perubahan kodenya?
6. Bagaimana rumus volumetrik, pembulatan, minimum, dan tier?
7. Apa batas berat, ukuran, nilai barang, dan jenis barang?
8. Bagaimana asuransi, pajak, fuel surcharge, remote area, dan packing?
9. Apakah SLA dalam hari kalender atau hari kerja, dan bagaimana cut-off?
10. Apakah beberapa credential boleh dipool untuk satu aplikasi?
