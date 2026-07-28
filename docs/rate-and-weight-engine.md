# Model Tarif dan Aturan Berat

## 1. Prinsip

Berat aktual berasal dari Emisell. API Kurir menentukan berat yang ditagihkan
dan total ongkir berdasarkan regulasi layanan.

```text
actual_weight
      |
      +----------------------+
      |                      |
      v                      v
volumetric_weight        actual_weight
      |                      |
      +---------- max -------+
                  |
                  v
           chargeable_weight
                  |
          minimum + rounding
                  |
                  v
             billing_weight
                  |
                  v
            tariff + surcharge
```

## 2. Rumus dasar

```text
volumetric_weight_kg =
  (length_cm * width_cm * height_cm) / volumetric_divisor

chargeable_weight_kg =
  max(actual_weight_kg, volumetric_weight_kg)

billing_weight_kg =
  apply_rounding(max(chargeable_weight_kg, minimum_weight_kg))
```

Divisor, minimum, dan pembulatan berasal dari service rule; tidak ada satu
nilai global yang berlaku bagi semua kurir.

## 3. Pricing model

Nilai `pricing_model` yang didukung:

| Model | Keterangan |
|---|---|
| `flat` | Satu harga untuk layanan/rentang tertentu |
| `per_kg` | Harga per kg setelah minimum dan pembulatan |
| `base_plus_increment` | Harga berat pertama ditambah berat berikutnya |
| `minimum_then_per_kg` | Minimum charge lalu kenaikan per kg |
| `tiered` | Harga berbeda per rentang berat |
| `distance_based` | Instant/same-day berdasarkan jarak |
| `provider_quote` | Belum aman dihitung lokal; gunakan snapshot quote |

## 4. Data model

```text
couriers
- id
- code
- name
- active

courier_services
- id
- courier_id
- code
- name
- service_type
- transport_mode
- active

rate_cards
- id
- origin_location_id
- destination_location_id
- courier_service_id
- pricing_model
- currency
- base_weight_grams
- base_price
- rate_per_increment
- minimum_weight_grams
- maximum_weight_grams
- weight_increment_grams
- volumetric_divisor
- rounding_threshold_grams
- rounding_mode
- rounding_profile_id
- insurance_rule_id
- effective_from
- effective_until
- verification_status
- source_id

rate_tiers
- id
- rate_card_id
- weight_from_grams
- weight_to_grams
- flat_price
- price_per_increment
- increment_grams

rate_surcharges
- id
- rate_card_id
- surcharge_type
- condition_json
- amount_type
- amount
- taxable

rate_snapshots
- id
- origin_location_id
- destination_location_id
- courier_code
- service_code
- requested_weight_grams
- requested_dimensions_json
- returned_cost
- etd_min
- etd_max
- raw_response_hash
- provider_code
- fetched_at
- expires_at
```

`verification_status`:

- `official_public`: berasal dari halaman/dokumen resmi publik;
- `official_contract`: berasal dari kontrak atau API merchant;
- `observed`: disimpulkan dari beberapa quote;
- `needs_contract_confirmation`: belum boleh dipakai menghitung final;
- `deprecated`: tidak lagi ditawarkan.

Profile pembulatan harus berada pada level service, channel, dan versi
kontrak. Detail boundary seperti perbedaan `>=1,30 kg` dan `>1,30 kg`
didokumentasikan pada
[Matriks toleransi dan pembulatan berat](weight-rounding-matrix.md).

## 5. Strategi inferensi tarif

Sebagian aggregator hanya mengembalikan total harga untuk berat yang diminta,
bukan rate card. Jangan membagi total harga dengan berat lalu menyimpulkannya
sebagai harga per kg.

Probe minimum:

### Parcel/reguler

- 1 kg;
- 2 kg;
- 5 kg.

### Cargo/trucking

- di bawah minimum yang diduga;
- tepat pada minimum;
- minimum + 1 kg;
- berat tier berikutnya.

Contoh JTR:

```text
5 kg, 10 kg, 11 kg, 20 kg
```

Aturan hanya boleh dinaikkan dari `observed` menjadi `official_contract`
setelah dibandingkan dengan rate sheet atau konfirmasi provider.

## 6. Sparse rate population

Master lokasi lengkap; rate card hanya dibuat untuk rute yang digunakan.

```text
hits = origin_count * destination_count * probe_weight_count
```

Endpoint district multi-courier RajaOngkir dapat meminta beberapa kode kurir
dalam satu request. Gunakan kemampuan tersebut agar jumlah kurir tidak
mengalikan jumlah hit.

Strategi:

1. Pre-warm origin gudang dan kota seller utama.
2. Isi destination populer.
3. Lazy load rute baru.
4. Refresh rute populer setiap 7-14 hari.
5. Refresh rute jarang saat dicari.
6. Simpan negative result 6-24 jam.

## 7. Algoritma kalkulasi

```text
function calculate(shipment, rateCard):
  profile = load_rounding_profile(
    rateCard.service,
    shipment.channel,
    shipment.shipped_at
  )

  billing = calculate_billing_weight_by_profile(
    shipment.packages,
    rateCard,
    profile
  )

  subtotal = apply_pricing_model(billing, rateCard)
  surcharge = calculate_surcharges(shipment, billing, rateCard)
  insurance = calculate_insurance(shipment.item_value, rateCard)

  return subtotal + surcharge + insurance
```

## 8. Validation

- Tolak berat `<= 0`.
- Dimensi harus diberikan sebagai satu set lengkap.
- Jangan gunakan divisor default jika provider belum dikonfirmasi.
- Jangan gunakan toleransi pembulatan default lintas provider.
- Jangan menawarkan layanan jika berat melewati maksimum.
- Simpan berat aktual, volumetrik, chargeable, rounded, dan billing dalam
  breakdown.
- Gunakan integer rupiah dan integer gram; hindari floating point untuk uang.
