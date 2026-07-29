# Sertifikasi Partner API Kurir

Status: **wajib sebelum production**

Progres sertifikasi, catatan reviewer, dan pengajuan publikasi ditampilkan
melalui Partner Portal. Partner dapat menyiapkan konfigurasi serta bukti,
tetapi hanya admin API Kurir yang dapat memberikan status `approved`,
`published`, atau `suspended`. Lihat
[Partner Portal dan Publikasi Extension](partner-portal.md).

## 1. Tahapan onboarding

```text
draft
submitted
technical_review
sandbox_testing
security_review
uat
approved
published
suspended
```

Limited production menjadi gate operasional sebelum `approved` untuk
capability atau partner yang memerlukannya. Vendor tidak dapat melompati
security review dan UAT.

## 2. Legal dan komersial

Partner harus mengonfirmasi:

- Partner Connector API dibangun dan dioperasikan partner;
- credential native carrier/downstream tidak diserahkan kepada API Kurir;
- API Kurir boleh digunakan untuk banyak seller Emisell;
- tarif dan status boleh ditampilkan kembali kepada seller/customer;
- hak penggunaan nama, logo, dan katalog layanan;
- penanggung jawab kehilangan, kerusakan, retur, dan klaim;
- struktur biaya, diskon, koreksi, COD, serta settlement;
- SLA, support, maintenance, dan penghentian layanan;
- data processing agreement dan subprocessor;
- retention serta penghapusan data;
- rate limit dan fair-use.

## 3. Katalog capability

Setiap capability diuji terpisah:

| Capability | Skenario minimum |
|---|---|
| rates | normal, tidak terjangkau, timeout, dynamic quote |
| shipment create | success, validation error, duplicate, timeout unknown |
| cancel | sebelum pickup, setelah pickup, duplicate |
| pickup | satu shipment, batch parsial, jadwal tidak valid |
| instant | driver search, allocated, cancel, delivered |
| label | single, batch, format tidak didukung |
| tracking | timeline, empty, delivered, external AWB |
| webhook | retry, duplicate, out-of-order, malformed |
| COD | batas nilai, fee, settlement |
| insurance | forced/optional, batas nilai |
| balance | debit, refund, adjustment |

Capability yang gagal tetap `false` meskipun endpoint provider tersedia.

Contract test hanya menguji Partner Connector canonical. Mapping, retry, dan
credential terhadap API native partner berada dalam tanggung jawab partner.

## 4. Test data

Partner menyediakan:

- lokasi terjangkau dan tidak terjangkau;
- layanan regular/cargo/instant yang relevan;
- credential sandbox;
- nomor order/AWB deterministik;
- skenario delivered, canceled, returned, lost, damaged;
- skenario perubahan biaya;
- webhook test trigger;
- daftar status mentah dan mapping resmi.

Test data tidak boleh berisi data pribadi customer sungguhan.

## 5. Idempotency dan concurrency

Uji wajib:

1. 20 request create identik secara concurrent menghasilkan satu shipment;
2. key sama dan body berbeda menghasilkan conflict;
3. timeout setelah provider menerima request dapat direkonsiliasi;
4. cancel berulang tidak menghasilkan tindakan ganda;
5. pickup batch retry tidak menggandakan item yang sudah berhasil.

## 6. Webhook

Partner harus membuktikan:

- event disimpan melalui outbox;
- event ID stabil pada retry;
- retry mengikuti backoff;
- event dapat tiba tidak berurutan;
- delivered/cancelled tidak saling menimpa;
- signature dan rotasi key bekerja;
- callback menerima `202`;
- reconciliation endpoint tersedia.

Provider legacy yang tidak mempunyai signature harus melewati review risiko dan
limited production lebih lama.

## 7. Performance

Target awal:

| Operasi | p95 sandbox | Timeout API Kurir |
|---|---:|---:|
| health/capabilities | 1 detik | 2 detik |
| rates reguler | 2,5 detik | 4 detik |
| rates instant | 4 detik | 6 detik |
| create shipment | 5 detik | 8 detik |
| tracking/detail | 3 detik | 5 detik |
| label | 8 detik | 10 detik |

Timeout tidak membuktikan kegagalan booking. Partner wajib menyediakan lookup
berdasarkan idempotency key atau merchant reference.

## 8. Security

Checklist:

- HMAC test vector;
- replay protection;
- constant-time secret comparison;
- TLS;
- payload size limit;
- rate limiting;
- secret rotation;
- log redaction;
- tenant isolation;
- SSRF control untuk URL;
- dependency vulnerability scan;
- incident contact 24/7 untuk kebocoran credential.

## 9. Limited production

Limited production memakai:

- allowlist seller;
- batas order/hari;
- alert error dan latency;
- reconciliation lebih sering;
- manual approval untuk capability berisiko;
- rollback dengan menonaktifkan provider account tanpa mematikan API Kurir.

## 10. Bukti sertifikasi

API Kurir menyimpan:

```text
partner_id
contract_version
capability
environment
test_run_id
result
evidence_checksum
certified_at
certified_by
expires_at
notes
```

Sertifikasi diulang ketika partner mengubah major API version, autentikasi,
status mapping, perhitungan tarif, atau alur settlement.
