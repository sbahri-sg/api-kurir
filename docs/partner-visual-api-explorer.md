# Partner Visual API Explorer

Status: **official read-only runner tersedia**

Visual API Explorer membantu partner menguji implementation OpenAPI yang sudah
diunggah tanpa menyiapkan Postman secara manual. Platform selalu dikunci dari
Partner Access Key, sedangkan katalog endpoint, parameter, dan contoh body
dibaca dari `openapi.yaml` pada versi package yang dipilih.

## 1. Pemisahan credential

| Credential | Fungsi | Pernah dikembalikan API? |
|---|---|---:|
| Partner Access Key `epk_live_*` | Login portal dan isolasi provider | Hanya saat dibuat admin |
| Official API key provider | Menjalankan endpoint provider melalui Explorer | Tidak |
| Runtime production credential | Komunikasi server-to-server setelah release siap | Tidak |

Official API key disimpan menggunakan AES-256-GCM dan associated data per
provider **serta per profil credential**. Key dilarang berada di ZIP, OpenAPI,
browser storage, URL, audit log, atau contoh kode. Portal hanya menerima
`display_key` tersensor.

Satu provider dapat memiliki beberapa profil credential. RajaOngkir adalah
contoh penting karena credential berikut tidak boleh saling ditukar:

| Profil | Upstream resmi | Header |
|---|---|---|
| `shipping_cost` | `https://rajaongkir.komerce.id/api/v1/` | `key` |
| `shipping_delivery` | `https://api.collaborator.komerce.id/` | `x-api-key` |

Jika provider memiliki environment uji native, alamat tersebut tetap menjadi
detail internal source connector. Manifest API Kurir hanya mendeklarasikan
`connector.base_url`, yaitu endpoint canonical aktif yang mengimplementasikan
kontrak API Kurir.

## 2. Alur

```text
Partner Portal
  -> pilih package yang lulus static scan
  -> hubungkan setiap official API key yang diminta OpenAPI
  -> pilih capability dan endpoint
  -> isi path/query/JSON body
  -> API Kurir controlled test runner
  -> official provider endpoint
  -> response + latency + validasi JSON
  -> run evidence redacted
```

Base URL default memakai `connector.base_url` dari manifest package.
Operation dapat mendeklarasikan server yang lebih spesifik melalui OpenAPI,
tetapi target tetap melewati validasi jaringan. UI wajib menampilkan bahwa
request menggunakan key live dan dapat memakai quota provider.

Response Explorer adalah bukti respons connector/upstream dan dapat tetap
menampilkan alias atau layanan mentah provider. Response tersebut bukan hasil
checkout Emisell. Jalur `/api/v1/calculate/district/domestic-cost` melakukan
normalisasi kode ekspedisi dan menerapkan kebijakan minimum/maksimum berat
secara terpusat sebelum mengembalikan opsi kepada Main Service.

Provider yang di-host oleh API Kurir tidak membutuhkan domain sandbox terpisah.
Explorer memakai public URL dari `RAJAONGKIR_HOSTED_PUBLIC_BASE_URL` untuk
dokumentasi, sedangkan eksekusi server-to-server memakai
`RAJAONGKIR_HOSTED_BASE_URL`. Pada Docker lokal, runtime mengarah langsung ke
service `rajaongkir-hosted`; pada production, public URL wajib HTTPS dan memakai
`https://api-kurir.emisell.com/connectors/rajaongkir/v1`. Override internal ini
hanya berasal dari konfigurasi operator dan tidak dapat ditentukan ZIP partner.

Security scheme menentukan credential yang digunakan operation. Contoh:

```yaml
components:
  securitySchemes:
    shipping_cost:
      type: apiKey
      in: header
      name: key
      x-emisell-credential-code: shipping_cost
      x-emisell-label: RajaOngkir Shipping Cost
    shipping_delivery:
      type: apiKey
      in: header
      name: x-api-key
      x-emisell-credential-code: shipping_delivery
      x-emisell-label: RajaOngkir Shipping Delivery
```

Operation `/rates` dan `/tracking/waybills` memakai `shipping_cost`, sedangkan
`/shipments` dan `/pickups` memakai `shipping_delivery`.

## 3. Endpoint yang dapat dijalankan

MVP mengizinkan:

- `GET`, `HEAD`, dan `OPTIONS`;
- `POST /rates`;
- `POST /tracking/waybills`.

Operation lain tetap ditampilkan tetapi berstatus `transactional_locked`.
Create shipment, pickup, cancel, label, atau mutasi lain baru boleh dibuka
setelah tersedia persetujuan admin, idempotency, test account, serta batas
transaksi. Explorer tidak boleh membuat transaksi live secara tidak sengaja.

## 4. Pengamanan jaringan

- HTTPS wajib dan port hanya `443`;
- redirect tidak diikuti;
- localhost, private IP, link-local, multicast, unspecified, dan carrier-grade
  NAT diblokir;
- hostname di-resolve oleh controlled dialer dan koneksi dilakukan ke IP yang
  sudah lolos validasi;
- timeout request 10 detik;
- request JSON maksimum 64 KB;
- response maksimum 1 MB;
- maksimum 120 pengujian per jam per Partner Access Key;
- hanya header response yang aman yang diteruskan;
- exact credential disensor dari response;
- preview run maksimum 4 KB dan field token, key, alamat, kontak, serta AWB
  disensor.

## 5. Endpoint Partner Portal

Semua endpoint menggunakan:

```http
Authorization: Bearer <partner-access-key>
```

| Method | Endpoint | Fungsi |
|---|---|---|
| `GET` | `/partner/v1/explorer/credential` | status credential lama/default; kompatibilitas |
| `PUT` | `/partner/v1/explorer/credentials/{code}` | menyimpan atau merotasi satu profil key resmi |
| `DELETE` | `/partner/v1/explorer/credentials/{code}` | hard delete satu profil credential |
| `GET` | `/partner/v1/submissions/{id}/explorer` | katalog dari OpenAPI package |
| `POST` | `/partner/v1/submissions/{id}/explorer/execute` | menjalankan operation read-only |
| `GET` | `/partner/v1/submissions/{id}/explorer/runs` | bukti pengujian terbaru |

Kontrak mesin lengkap berada di [`../openapi/public.yaml`](../openapi/public.yaml).

## 6. Batas MVP

Validasi saat ini mencakup HTTP success dan validitas JSON. Profil credential
dan aturan header diturunkan dari OpenAPI serta divalidasi sebelum request.
Full response schema
validation, transactional approval, test data generator, webhook simulator,
dan promotion evidence otomatis ke status sertifikasi merupakan tahap lanjutan.
