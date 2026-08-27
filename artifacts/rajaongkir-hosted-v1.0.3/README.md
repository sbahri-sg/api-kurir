# RajaOngkir Hosted Connector

Connector resmi yang dikelola dan di-host oleh API Kurir untuk menerjemahkan
RajaOngkir Shipping Cost dan Shipping Delivery ke kontrak Partner API Kurir v1.

Package versi `1.0.5` ini memakai kode canonical `rajaongkir`. Setelah release connector
dipublikasikan, seluruh rate, tracking, shipment, label, cancel, dan pickup
RajaOngkir dijalankan melalui connector hosted. Credential tetap milik seller
dan diteruskan API Kurir secara terenkripsi sesuai capability.

## Credential

Connector mendukung dua credential resmi yang tidak boleh dimasukkan ke ZIP:

- Shipping Cost melalui header `key` untuk rate dan tracking.
- Shipping Delivery melalui header `x-api-key` untuk order, detail, cancel,
  label, dan pickup.

Partner Portal menyimpan keduanya secara terenkripsi. Source tidak menulis
header, payload pelanggan, atau response provider ke log.

## Endpoint

- `GET /partner/v1/health`
- `GET /partner/v1/capabilities`
- `GET /partner/v1/services`
- `POST /partner/v1/rates`
- `POST /partner/v1/tracking/waybills`
- `POST /partner/v1/shipments`
- `GET /partner/v1/shipments/{partner_shipment_id}`
- `POST /partner/v1/shipments/{partner_shipment_id}/cancel`
- `GET /partner/v1/shipments/{partner_shipment_id}/label`
- `POST /partner/v1/pickups`

## Menjalankan

```bash
npm run check
npm test
npm start
```

Port default adalah `8080`. API Kurir mengirim header
`X-Emisell-Execution-Mode: live|sandbox` pada setiap request connector.
Shipping Cost tetap live pada kedua mode, sedangkan Shipping Delivery otomatis
memilih production atau sandbox. Base URL hanya dapat dioverride dari deployment
melalui `SHIPPING_COST_BASE_URL`, `SHIPPING_DELIVERY_BASE_URL`, dan
`SHIPPING_DELIVERY_SANDBOX_BASE_URL`.

Manifest mendeklarasikan form credential, environment, perilaku capability, dan
billing. Emisell hanya merender deklarasi aman tersebut; URL upstream dan secret
tidak pernah dikirim ke browser.

## Deploy hosted

Manifest hanya menyimpan satu `connector.base_url`. Route publik connector
tetap mengikuti environment API Kurir:

- Official hosted endpoint: `https://api-kurir.emisell.com/connectors/rajaongkir/v1`
- Docker lokal: public URL mengikuti `RAJAONGKIR_HOSTED_PUBLIC_BASE_URL`, sedangkan
  runtime API Kurir memakai `RAJAONGKIR_HOSTED_BASE_URL`.

Container ini harus ditempatkan di belakang reverse proxy, TLS 1.2+, rate
limiter, dan observability API Kurir. Jangan membuka port container langsung ke
internet.

Pada Docker Compose API Kurir, route `/connectors/rajaongkir/v1/*` diteruskan
ke endpoint internal `/partner/v1/*` milik container ini. Provider database
`rajaongkir` tetap `available=false` selama UAT. Provider diaktifkan kembali
setelah release hosted berstatus published; tidak ada provider RajaOngkir
duplikat pada katalog merchant.

## Sumber kontrak upstream

- Shipping Cost: `https://rajaongkir.komerce.id/api/v1/`
- Shipping Delivery production: `https://api.collaborator.komerce.id/`
- Shipping Delivery sandbox: `https://api-sandbox.collaborator.komerce.id/`

Endpoint transaksi tetap dikunci pada Visual API Explorer. Pengujian create,
cancel, label, dan pickup dilakukan pada sandbox melalui proses sertifikasi.
