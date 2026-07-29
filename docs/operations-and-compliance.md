# Operasional, Kuota, dan Kepatuhan

## 1. Sasaran operasional

- Cek ongkir cache/rate card lokal: p95 di bawah 150 ms.
- Tracking dari snapshot lokal: p95 di bawah 200 ms.
- Tracking dari snapshot fresh/final tidak memakai hit provider; hanya
  miss/stale yang memakai satu hit dan menyimpan hasilnya.
- Provider outage tidak menjatuhkan seluruh API.
- Tarif dan event selalu memiliki sumber serta waktu pengambilan.

## 2. Populasi tarif secara hemat

Jangan mengisi Cartesian product seluruh wilayah. Contoh 100 origin × 7.000
destination × beberapa berat akan menghabiskan quota tanpa manfaat.

Urutan populasi:

1. origin gudang aktif;
2. destination yang benar-benar dicari;
3. rute dengan transaksi berhasil;
4. rute populer menurut frekuensi 7/30 hari;
5. pre-warm bertahap untuk seller baru.

Untuk satu rute, kirim multi-courier dalam satu request bila endpoint dan
lisensi provider mengizinkan. Probe berat hanya dilakukan bila diperlukan
untuk memahami model tarif:

- parcel: 1, 2, dan 5 kg;
- cargo: di bawah minimum, tepat minimum, minimum+1, dan batas tier.

Setelah formula resmi tersedia, hentikan probe berulang. Refresh hanya harga
dasar dan periode efektif.

TTL awal:

| Jenis data | TTL rekomendasi |
|---|---:|
| Master lokasi | 30 hari, dengan sync versi |
| Rute populer | 7 hari |
| Rute normal | 14 hari |
| Rute jarang | 30 hari atau lazy refresh |
| Rute tidak tersedia | 6–24 jam |
| Quote instant/same-day | Sangat singkat; provider quote |

Refresh menggunakan jitter agar ribuan rate card tidak kedaluwarsa pada detik
yang sama.

## 3. Model kuota RajaOngkir

Asumsi komersial dari rencana awal:

```text
1 akun  = Rp249.000/bulan, 50.000 hit/hari
4 akun  = Rp996.000/bulan, 200.000 hit/hari secara matematis
```

Angka 200.000 adalah kapasitas teoritis, bukan otomatis hak untuk melakukan
credential pooling. Syarat RajaOngkir membatasi penggunaan API untuk membuat
layanan serupa/redistribusi tanpa izin tertulis. Sebelum empat akun dipakai
sebagai satu pool, dapatkan persetujuan tertulis mengenai:

- satu badan usaha dengan beberapa credential;
- penggunaan internal oleh Emisell;
- penyimpanan/cache tarif;
- retensi respons;
- pembagian data ke tenant/customer;
- failover dan rotasi credential.

Jika disetujui, jangan memakai round-robin buta. Gunakan quota ledger per
credential:

```text
credential_id
provider
daily_limit
used_count
reserved_count
reset_at
health_status
last_error_at
```

Contoh budget konservatif dari kapasitas teoritis 200.000 hit/hari:

| Pool | Budget/hari |
|---|---:|
| Tracking refresh | 140.000 |
| Rate miss/refresh | 15.000 |
| Retry teknis terkontrol | 5.000 |
| Cadangan lonjakan/insiden | 40.000 |

Budget bukan target untuk dihabiskan. Alert di 60%, 75%, 85%, dan 95%.

## 4. Tracking tanpa membakar quota

### Subscription, bukan page view

Resi menjadi satu entitas yang berlangganan refresh. Sepuluh ribu customer
membuka resi yang sama tetap membaca satu snapshot dan tidak membuat sepuluh
ribu hit upstream.

Key deduplikasi:

```text
courier_code + normalized_waybill
```

Tambahkan `tenant_id` bila kontrak atau kebijakan privasi tidak mengizinkan
snapshot lintas tenant.

### Jadwal berbasis status

| Status | Interval awal |
|---|---:|
| Belum ditemukan, usia <24 jam | 2–4 jam |
| Pickup/in transit | 1–2 jam |
| Out for delivery | 30–60 menit |
| Delivery failed | 2–4 jam |
| Delivered/returned/cancelled | Hentikan polling |
| Tidak ada perubahan >48 jam | Perlambat menjadi 6–12 jam |

Interval diberi jitter ±10–20%. Prioritaskan resi aktif yang dekat SLA dan
kurangi frekuensi resi tanpa perubahan.

### Deduplication dan lock

- unique index pada identitas resi;
- distributed lock `tracking-refresh:{courier}:{waybill}`;
- satu refresh aktif per resi;
- event di-upsert menggunakan hash timestamp+status+location;
- hasil invalid waybill di-negative-cache 30–120 menit;
- tidak retry invalid AWB.

### Webhook

Jika direct carrier menyediakan webhook:

1. verifikasi signature;
2. simpan payload mentah terenkripsi;
3. normalisasi event secara idempotent;
4. jadikan webhook sumber utama;
5. polling hanya sebagai reconciliation.

## 5. Kebijakan fallback

Urutan:

```text
direct carrier API
  -> aggregator resmi yang masih memiliki quota
  -> snapshot lokal dengan is_stale=true
  -> error terstruktur
```

Jangan menggunakan scraping halaman tracking sebagai fondasi produksi.
Pembatasan “maksimum 10 resi per sesi”, CAPTCHA, cookie, perubahan HTML, dan
syarat penggunaan membuatnya tidak stabil serta berisiko diblokir. Library
GitHub yang melakukan scraping boleh dipelajari sebagai referensi format
status, tetapi bukan sebagai SLA production.

Setiap adapter memiliki:

- timeout;
- circuit breaker;
- retry budget;
- rate limiter;
- credential pool yang disetujui;
- health score.

## 6. RajaOngkir dan Karrio

Karrio dipakai untuk menormalisasi provider dan mengurangi kode integrasi yang
berulang. Ia tidak menghapus limit, kontrak, atau biaya upstream. API Kurir
tetap bertanggung jawab atas:

- tenant dan otorisasi;
- quota ledger;
- cache/rate card;
- rule engine;
- kepatuhan kontrak;
- observability.

Karrio OSS berlisensi LGPL untuk core repository. Tinjau dependensi/plugin dan
cara deployment dengan penasihat legal perusahaan; fitur Enterprise Karrio
tidak diasumsikan tersedia.

## 7. Keamanan dan privasi

- Secret provider disimpan di secret manager, bukan database plaintext.
- Log menyimpan alias credential, bukan API key.
- Masking nama, telepon, alamat, dan nomor resi pada log.
- TLS untuk traffic; enkripsi database dan backup.
- API key per tenant, dapat dirotasi dan dicabut.
- Role terpisah untuk admin tarif, operator tracking, dan reader.
- Audit log untuk perubahan rate card serta manual refresh.
- Retensi raw payload dibatasi sesuai kebutuhan audit/kontrak.
- Nomor resi diperlakukan sebagai data sensitif yang dapat mengungkap
  pergerakan kiriman.

## 8. Data quality

### Rate validation

Setiap import/refresh memeriksa:

- harga tidak negatif;
- tanggal efektif tidak overlap secara ambigu;
- origin/destination terpetakan;
- service aktif;
- minimum tidak melebihi maksimum;
- divisor hanya nilai resmi;
- lonjakan harga di atas threshold masuk review.

Lakukan canary untuk rute referensi:

```text
Jakarta -> Bandung
Jakarta -> Surabaya
Jakarta -> Medan
Surabaya -> Makassar
```

Bandingkan 1 kg, 5 kg, serta minimum cargo. Selisih terhadap provider harus
nol untuk harga final atau berada pada toleransi yang memang terdokumentasi.

### Tracking validation

- event tidak boleh mundur waktu tanpa penanda koreksi;
- terminal status tidak ditimpa status nonterminal kecuali provider mengirim
  koreksi eksplisit;
- simpan status asli dan status normalisasi;
- alert jika provider tidak berubah pada banyak resi aktif sekaligus.

## 9. Monitoring dan alert

Dashboard minimum:

- request per endpoint dan tenant;
- p50/p95/p99 latency;
- cache hit rate ongkir;
- hit provider per credential;
- quota tersisa;
- error/timeout/429 per provider;
- active tracking dan polling due;
- oldest queue age;
- rate card stale per kurir;
- mismatch kalkulasi lokal versus quote canary.

Alert kritis:

- quota tinggal 15%;
- provider error >10% selama 10 menit;
- queue lag >15 menit;
- database unavailable, atau Redis unavailable bila Redis sudah diaktifkan;
- mismatch tarif produksi;
- lonjakan invalid AWB yang dapat menandakan abuse.

## 10. Runbook insiden singkat

### Provider lambat/down

1. Buka circuit breaker.
2. Layani snapshot yang masih aman dengan `is_stale=true`.
3. Turunkan concurrency worker.
4. Alihkan ke provider resmi cadangan bila kontrak mengizinkan.
5. Jangan memperbesar retry storm.

### Quota hampir habis

1. Hentikan pre-warm dan refresh rate nonprioritas.
2. Perlambat resi tanpa perubahan.
3. Pertahankan out-for-delivery dan transaksi checkout.
4. Gunakan credential lain hanya jika pooling disetujui.
5. Tampilkan freshness secara jujur.

### Tarif salah

1. Nonaktifkan rate card/version terkait.
2. Fallback ke provider quote.
3. Identifikasi order terdampak dari `rate_card_id`.
4. Import versi koreksi dengan `effective_from`.
5. Jangan menimpa histori yang dipakai order lama.

## 11. Tahap implementasi

### Fase 1

- lokasi lokal;
- RajaOngkir sebagai sumber tarif miss;
- JNE/TIKI/SiCepat sebagai prioritas;
- tracking snapshot dan polling;
- PostgreSQL, queue, metrics, serta kontrak cache/lock yang dapat memakai Redis.
- Redis runtime belum wajib untuk MVP satu instance, tetapi diaktifkan saat
  pre-production sebelum scaling horizontal.

### Fase 2

- onboarding direct API kurir dengan volume tertinggi;
- webhook bila tersedia;
- local calculation untuk layanan yang sudah terverifikasi;
- admin rate card dan approval.

### Fase 3

- optimasi provider routing;
- canary otomatis;
- forecasting quota;
- kurangi ketergantungan aggregator hingga hanya menjadi fallback.

## 12. Gate sebelum produksi

- izin penggunaan/cache/redistribusi tertulis;
- kontrak direct carrier atau aggregator aktif;
- test matrix semua service aktif;
- load test p95 dan queue;
- drill quota exhaustion;
- drill provider outage;
- backup/restore teruji;
- dashboard dan on-call tersedia;
- legal review untuk ToS serta retensi data.
