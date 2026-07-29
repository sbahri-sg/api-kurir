export type ApiDocumentationScope = "customer" | "admin";
export type ApiDocumentationMethod = "GET" | "POST";

export type ApiDocumentationEndpoint = {
  scope: ApiDocumentationScope;
  method: ApiDocumentationMethod;
  path: string;
  title: string;
  description: string;
  authentication: string;
  parameters?: string[];
  request?: string;
  response: string;
};

export const API_DOCUMENTATION: ApiDocumentationEndpoint[] = [
  {
    scope: "customer",
    method: "GET",
    path: "/health/live",
    title: "Liveness API",
    description:
      "Memastikan proses API sedang berjalan. Endpoint ini tidak memeriksa koneksi database.",
    authentication: "Tanpa autentikasi",
    response: `{
  "status": "ok"
}`,
  },
  {
    scope: "customer",
    method: "GET",
    path: "/health/ready",
    title: "Readiness API",
    description:
      "Memastikan API dan PostgreSQL siap menerima request customer.",
    authentication: "Tanpa autentikasi",
    response: `{
  "status": "ready"
}`,
  },
  {
    scope: "customer",
    method: "GET",
    path: "/v1/destination/domestic-destination",
    title: "Cari lokasi domestik",
    description:
      "Mencari master wilayah lokal sampai kelurahan/desa dan kode pos. Gunakan ID hasil endpoint ini untuk origin dan destination.",
    authentication: "Bearer customer API key",
    parameters: [
      "search — wajib, minimal 2 karakter; mendukung nama wilayah atau kode pos",
      "limit — opsional, 1–50; default 20",
    ],
    request: `GET {{base_url}}/v1/destination/domestic-destination?search=Dago Bandung&limit=20`,
    response: `{
  "data": [
    {
      "id": "loc_idn_32_73_02_1004",
      "label": "Dago, Coblong, Kota Bandung, Jawa Barat",
      "province_id": "loc_idn_32",
      "city_id": "loc_idn_32_73",
      "district_id": "loc_idn_32_73_02",
      "subdistrict_id": "loc_idn_32_73_02_1004",
      "province_name": "Jawa Barat",
      "city_name": "Kota Bandung",
      "district_name": "Coblong",
      "subdistrict_name": "Dago",
      "zip_code": "40135",
      "province": "Jawa Barat",
      "city": "Kota Bandung",
      "district": "Coblong",
      "subdistrict": "Dago",
      "postal_code": "40135",
      "postal_codes": ["40135"]
    }
  ],
  "meta": {
    "message": "Success Get Domestic Destinations",
    "code": 200,
    "status": "success",
    "request_id": "req_example",
    "next_cursor": null
  }
}`,
  },
  {
    scope: "customer",
    method: "GET",
    path: "/api/v1/destination/domestic-destination",
    title: "Cari lokasi SDK RajaOngkir V2",
    description:
      "Direct search wilayah lokal dengan path, header, query, ID numerik, dan respons RajaOngkir V2.",
    authentication: "Header key: customer API key",
    parameters: [
      "search — wajib, minimal 2 karakter",
      "limit — opsional, 1–1.000; default 20",
      "offset — opsional, default 0",
    ],
    request: `GET {{base_url}}/api/v1/destination/domestic-destination?search=Husein Sastranegara&limit=20&offset=0
key: {{api_key}}`,
    response: `{
  "meta": {
    "message": "Success Get Domestic Destinations",
    "code": 200,
    "status": "success"
  },
  "data": [
    {
      "id": 3273061001,
      "label": "Husein Sastranegara, Cicendo, Kota Bandung, Jawa Barat, 40174",
      "province_name": "Jawa Barat",
      "city_name": "Kota Bandung",
      "district_name": "Cicendo",
      "subdistrict_name": "Husein Sastranegara",
      "zip_code": "40174"
    }
  ]
}`,
  },
  {
    scope: "customer",
    method: "GET",
    path: "/api/v1/destination/province",
    title: "Daftar provinsi",
    description:
      "Membaca daftar provinsi dari master Kemendagri lokal dengan bentuk respons hierarki RajaOngkir V2.",
    authentication: "Header key: customer API key (mode SDK RajaOngkir V2)",
    request: `GET {{base_url}}/api/v1/destination/province`,
    response: `{
  "meta": {
    "message": "Success Get Province",
    "code": 200,
    "status": "success"
  },
  "data": [
    {
      "id": 32,
      "name": "Jawa Barat"
    }
  ]
}`,
  },
  {
    scope: "customer",
    method: "GET",
    path: "/api/v1/destination/city/{province_id}",
    title: "Kota per provinsi",
    description:
      "Membaca kota/kabupaten berdasarkan ID provinsi lokal. ID hasil berbeda dari ID provinsi.",
    authentication: "Header key: customer API key (mode SDK RajaOngkir V2)",
    parameters: [
      "province_id — wajib, ID dari endpoint daftar provinsi",
    ],
    request: `GET {{base_url}}/api/v1/destination/city/32`,
    response: `{
  "meta": {
    "message": "Success Get City By Province ID",
    "code": 200,
    "status": "success"
  },
  "data": [
    {
      "id": 3273,
      "name": "Kota Bandung",
      "zip_code": ""
    }
  ]
}`,
  },
  {
    scope: "customer",
    method: "GET",
    path: "/api/v1/destination/district/{city_id}",
    title: "Kecamatan per kota",
    description:
      "Membaca kecamatan berdasarkan ID kota/kabupaten lokal.",
    authentication: "Header key: customer API key (mode SDK RajaOngkir V2)",
    parameters: ["city_id — wajib, ID dari endpoint daftar kota"],
    request: `GET {{base_url}}/api/v1/destination/district/3273`,
    response: `{
  "meta": {
    "message": "Success Get District By City ID",
    "code": 200,
    "status": "success"
  },
  "data": [
    {
      "id": 327306,
      "name": "Cicendo",
      "zip_code": ""
    }
  ]
}`,
  },
  {
    scope: "customer",
    method: "GET",
    path: "/api/v1/destination/sub-district/{district_id}",
    title: "Kelurahan per kecamatan",
    description:
      "Membaca kelurahan/desa berdasarkan ID kecamatan lokal sampai kode pos.",
    authentication: "Header key: customer API key (mode SDK RajaOngkir V2)",
    parameters: ["district_id — wajib, ID dari endpoint daftar kecamatan"],
    request: `GET {{base_url}}/api/v1/destination/sub-district/327306`,
    response: `{
  "meta": {
    "message": "Success Get Sub District By District ID",
    "code": 200,
    "status": "success"
  },
  "data": [
    {
      "id": 3273061001,
      "name": "Husein Sastranegara",
      "zip_code": "40174"
    }
  ]
}`,
  },
  {
    scope: "customer",
    method: "GET",
    path: "/v1/couriers",
    title: "Daftar kurir",
    description:
      "Membaca katalog kurir, kemampuan domestik/internasional/tracking, layanan lokal aktif, dan mode kalkulasi.",
    authentication: "Bearer customer API key",
    response: `{
  "data": [
    {
      "code": "jne",
      "name": "JNE",
      "provider_code": "rajaongkir",
      "supports_domestic_cost": true,
      "supports_international_cost": true,
      "supports_tracking": true,
      "catalog_source": "https://rajaongkir.com/docs/shipping-cost/getting_started/courier_availability",
      "catalog_verified_at": "2026-07-28",
      "services": [
        {
          "code": "REG",
          "name": "Reguler",
          "group": "regular",
          "service_type": "parcel",
          "calculation_mode": "provider_quote"
        }
      ]
    }
  ]
}`,
  },
  {
    scope: "customer",
    method: "POST",
    path: "/v1/calculate/domestic-cost",
    title: "Cek ongkir domestik",
    description:
      "Mengambil tarif berdasarkan rute, berat, dan pilihan kurir. Snapshot yang masih berlaku dipakai ulang tanpa hit provider.",
    authentication: "Bearer customer API key",
    parameters: [
      "origin dan destination — ID lokasi lokal dari endpoint pencarian",
      "weight — berat yang ditangani Emisell dalam gram",
      "courier — satu kode atau beberapa kode dipisahkan tanda titik dua",
      "dimensions, item_value, dan options — opsional",
    ],
    request: `{
  "origin": "{{origin_id}}",
  "destination": "{{destination_id}}",
  "weight": 1200,
  "courier": "jne",
  "dimensions": {
    "length": 30,
    "width": 20,
    "height": 15,
    "unit": "cm"
  },
  "item_value": 250000,
  "options": {
    "include_insurance": false,
    "include_unverified": false
  }
}`,
    response: `{
  "meta": {
    "request_id": "req_example",
    "calculated_at": "2026-07-28T05:20:00Z",
    "currency": "IDR"
  },
  "data": [
    {
      "courier": {
        "code": "jne",
        "name": "Jalur Nugraha Ekakurir (JNE)"
      },
      "service": {
        "code": "REG23",
        "name": "JNE Regular",
        "canonical_code": "REG",
        "group": "regular",
        "type": "parcel",
        "variant_code": "REG23"
      },
      "cost": 12000,
      "etd": {
        "min_days": 1,
        "max_days": 2,
        "text": "1-2 hari"
      },
      "source": {
        "type": "provider_quote",
        "provider": "rajaongkir",
        "verification_status": "observed",
        "is_stale": false
      }
    }
  ],
  "warnings": []
}`,
  },
  {
    scope: "customer",
    method: "POST",
    path: "/api/v1/calculate/domestic-cost",
    title: "Cek ongkir SDK V2 · kelurahan",
    description:
      "Endpoint form-urlencoded RajaOngkir V2 untuk origin dan destination hasil direct search atau endpoint sub-district.",
    authentication: "Header key: customer API key",
    parameters: [
      "origin dan destination — ID integer hasil endpoint sub-district atau domestic-destination",
      "weight — berat gram",
      "courier — kode kurir dipisahkan titik dua",
      "price — lowest atau highest, opsional",
    ],
    request: `POST {{base_url}}/api/v1/calculate/domestic-cost
Content-Type: application/x-www-form-urlencoded
key: {{api_key}}

origin=3273061001&destination=3212122001&weight=1000&courier=jne&price=lowest`,
    response: `{
  "meta": {
    "message": "Success Calculate Domestic Shipping cost",
    "code": 200,
    "status": "success"
  },
  "data": [
    {
      "name": "Jalur Nugraha Ekakurir (JNE)",
      "code": "jne",
      "service": "REG",
      "description": "Layanan Reguler",
      "cost": 20000,
      "etd": "3"
    }
  ]
}`,
  },
  {
    scope: "customer",
    method: "POST",
    path: "/api/v1/calculate/district/domestic-cost",
    title: "Cek ongkir SDK V2 · kecamatan",
    description:
      "Drop-in endpoint form-urlencoded untuk SDK RajaOngkir V2. Gunakan /api/v1/calculate/domestic-cost dengan bentuk request yang sama untuk ID subdistrict.",
    authentication: "Header key: customer API key",
    parameters: [
      "origin dan destination — ID integer hasil endpoint district",
      "weight — berat gram",
      "courier — kode kurir dipisahkan titik dua",
      "price — lowest atau highest, opsional",
    ],
    request: `POST {{base_url}}/api/v1/calculate/district/domestic-cost
Content-Type: application/x-www-form-urlencoded
key: {{api_key}}

origin=327306&destination=321212&weight=1000&courier=jne&price=lowest`,
    response: `{
  "meta": {
    "message": "Success Calculate Domestic Shipping cost",
    "code": 200,
    "status": "success"
  },
  "data": [
    {
      "name": "Jalur Nugraha Ekakurir (JNE)",
      "code": "jne",
      "service": "REG",
      "description": "Layanan Reguler",
      "cost": 20000,
      "etd": "3"
    }
  ]
}`,
  },
  {
    scope: "customer",
    method: "POST",
    path: "/api/v1/track/waybill",
    title: "Cek resi SDK RajaOngkir V2",
    description:
      "Drop-in tracking sinkron dengan envelope dan field RajaOngkir V2. Hasil disimpan sebagai snapshot agar pengecekan berulang tidak selalu memakai hit provider.",
    authentication: "Header key: customer API key",
    parameters: [
      "awb — nomor resi 6–40 karakter",
      "courier — jne, sap, ninja, jnt, tiki, wahana, pos, atau lion",
      "last_phone_number — opsional; 5 digit terakhir nomor penerima hanya jika provider memintanya",
    ],
    request: `POST {{base_url}}/api/v1/track/waybill?awb={{waybill}}&courier={{courier}}
key: {{api_key}}`,
    response: `{
  "meta": {
    "message": "Success Tracking AWB",
    "code": 200,
    "status": "success"
  },
  "data": {
    "delivered": true,
    "summary": {
      "courier_code": "wahana",
      "courier_name": "Wahana Prestasi Logistik",
      "waybill_number": "MT685U91",
      "service_code": "",
      "waybill_date": "2024-10-09",
      "shipper_name": "",
      "receiver_name": "FIKRI EL SARA",
      "origin": "JAKARTA",
      "destination": "SUKABUMI",
      "status": "DELIVERED"
    },
    "details": {
      "waybill_number": "MT685U91",
      "waybill_date": "2024-10-09",
      "waybill_time": "",
      "weight": "",
      "origin": "JAKARTA",
      "destination": "SUKABUMI",
      "shipper_name": "",
      "shipper_address1": "",
      "shipper_address2": "",
      "shipper_address3": "",
      "shipper_city": "",
      "receiver_name": "FIKRI EL SARA",
      "receiver_address1": "",
      "receiver_address2": "",
      "receiver_address3": "",
      "receiver_city": ""
    },
    "delivery_status": {
      "status": "DELIVERED",
      "pod_receiver": "FIKRI EL SARA",
      "pod_date": "2024-10-11",
      "pod_time": "09:26:00"
    },
    "manifest": []
  }
}`,
  },
  {
    scope: "customer",
    method: "POST",
    path: "/v1/track/waybill",
    title: "Cek resi asynchronous (legacy)",
    description:
      "Membaca snapshot tracking yang tersimpan atau mengantrikan refresh. Cukup kirim nomor resi dan ekspedisi.",
    authentication: "Bearer customer API key",
    parameters: [
      "waybill — nomor resi 6–40 karakter",
      "courier — jne, sap, ninja, jnt, tiki, wahana, pos, atau lion",
      "refresh — gunakan if_stale",
    ],
    request: `{
  "waybill": "{{waybill}}",
  "courier": "{{courier}}",
  "refresh": "if_stale"
}`,
    response: `{
  "data": {
    "courier": "jne",
    "waybill": "*********9012",
    "status": "in_transit",
    "status_label": "Dalam perjalanan",
    "summary": {},
    "events": [],
    "provider": "rajaongkir",
    "provider_fetched_at": "2026-07-28T05:20:00Z",
    "next_refresh_at": "2026-07-28T06:20:00Z",
    "is_final": false,
    "refresh_queued": false,
    "last_error_code": ""
  },
  "meta": {
    "request_id": "req_example"
  }
}`,
  },
  {
    scope: "admin",
    method: "GET",
    path: "/v1/admin/overview",
    title: "Ringkasan operasional",
    description:
      "Membaca jumlah master wilayah, mapping, snapshot tarif, kuota, dan antrean tracking.",
    authentication: "Bearer admin API key",
    response: `{
  "data": {
    "official_locations": 91599,
    "location_mappings": 2,
    "fresh_quote_snapshots": 7,
    "quota_used_today": 126,
    "quota_limit_today": 50000,
    "tracking_pending_jobs": 0,
    "tracking_shipments": 0
  },
  "meta": {
    "request_id": "req_example"
  }
}`,
  },
  {
    scope: "admin",
    method: "GET",
    path: "/v1/admin/couriers",
    title: "Kurir untuk alat operasional",
    description:
      "Membaca katalog kemampuan dan layanan untuk menu Daftar Ekspedisi serta pilihan pada alat operasional.",
    authentication: "Bearer admin API key",
    response: `{
  "data": [
    {
      "code": "jne",
      "name": "JNE",
      "provider_code": "rajaongkir",
      "supports_domestic_cost": true,
      "supports_international_cost": true,
      "supports_tracking": true,
      "catalog_source": "https://rajaongkir.com/docs/shipping-cost/getting_started/courier_availability",
      "catalog_verified_at": "2026-07-28",
      "services": [
        {
          "code": "REG",
          "name": "Reguler",
          "group": "regular",
          "service_type": "parcel",
          "calculation_mode": "provider_quote"
        }
      ]
    }
  ]
}`,
  },
  {
    scope: "admin",
    method: "POST",
    path: "/v1/admin/calculate/domestic-cost",
    title: "Cek ongkir dari dashboard",
    description:
      "Menjalankan rate engine yang sama dengan endpoint customer menggunakan autentikasi admin. Snapshot aktif tetap dipakai tanpa hit provider.",
    authentication: "Bearer admin API key",
    parameters: [
      "origin dan destination — ID lokasi lokal dari pencarian admin",
      "weight — berat paket dalam gram",
      "courier — kode ekspedisi",
    ],
    request: `{
  "origin": "{{origin_id}}",
  "destination": "{{destination_id}}",
  "weight": 1200,
  "courier": "jne"
}`,
    response: `{
  "data": [
    {
      "courier": {"code": "jne", "name": "JNE"},
      "service": {
        "code": "JTR>130",
        "name": "JNE Trucking",
        "canonical_code": "JTR",
        "group": "cargo",
        "type": "cargo",
        "variant_code": "JTR>130"
      },
      "cost": 12000,
      "etd": {"min_days": 1, "max_days": 2, "text": "1-2 hari"},
      "source": {
        "type": "provider_quote",
        "provider": "rajaongkir",
        "verification_status": "observed",
        "is_stale": false
      }
    }
  ],
  "warnings": []
}`,
  },
  {
    scope: "admin",
    method: "POST",
    path: "/v1/admin/track/waybill",
    title: "Cek resi dari dashboard",
    description:
      "Membaca snapshot tracking atau mengantrikan refresh ke worker. Nomor resi disimpan terenkripsi dan ditampilkan termasking.",
    authentication: "Bearer admin API key",
    parameters: [
      "waybill — nomor resi 6–40 karakter",
      "courier — jne, sap, ninja, jnt, tiki, wahana, pos, atau lion",
    ],
    request: `{
  "waybill": "{{waybill}}",
  "courier": "jne",
  "refresh": "if_stale"
}`,
    response: `{
  "data": {
    "courier": "jne",
    "waybill": "*********9012",
    "status": "in_transit",
    "status_label": "Dalam perjalanan",
    "events": [],
    "provider": "rajaongkir",
    "refresh_queued": false,
    "is_final": false,
    "last_error_code": ""
  },
  "meta": {"request_id": "req_example"}
}`,
  },
  {
    scope: "admin",
    method: "GET",
    path: "/v1/admin/provider-credentials",
    title: "Daftar key provider",
    description:
      "Membaca key RajaOngkir yang tersimpan secara termasking. Secret asli dan ciphertext tidak pernah dikembalikan.",
    authentication: "Bearer admin API key",
    response: `{
  "data": [
    {
      "id": "be4c62aa-a700-4e23-bfc0-02072791b59e",
      "provider_code": "rajaongkir",
      "credential_alias": "rajaongkir-6c8f212a",
      "display_key": "rk_d••••2026",
      "daily_limit": 50000,
      "active": true,
      "validation_status": "valid",
      "last_validated_at": "2026-07-28T08:20:00Z"
    }
  ],
  "meta": {
    "request_id": "req_example"
  }
}`,
  },
  {
    scope: "admin",
    method: "POST",
    path: "/v1/admin/provider-credentials",
    title: "Tambah key provider",
    description:
      "Memvalidasi key ke RajaOngkir, mengenkripsinya, dan langsung mengaktifkannya untuk resolver runtime tanpa restart.",
    authentication: "Bearer admin API key",
    parameters: [
      "provider_code — saat ini rajaongkir",
      "api_key — secret provider; tidak pernah dikembalikan",
    ],
    request: `{
  "provider_code": "rajaongkir",
  "api_key": "{{rajaongkir_api_key}}"
}`,
    response: `{
  "data": {
    "id": "be4c62aa-a700-4e23-bfc0-02072791b59e",
    "provider_code": "rajaongkir",
    "credential_alias": "rajaongkir-6c8f212a",
    "display_key": "rk_d••••2026",
    "daily_limit": 50000,
    "active": true,
    "validation_status": "valid"
  },
  "meta": {
    "request_id": "req_example"
  }
}`,
  },
  {
    scope: "admin",
    method: "POST",
    path: "/v1/admin/provider-credentials/{id}/disable",
    title: "Nonaktifkan key provider",
    description:
      "Mengeluarkan satu key dari rotasi. Key lain tetap aktif dan resolver otomatis memilih credential yang tersedia.",
    authentication: "Bearer admin API key",
    parameters: ["id — UUID credential provider"],
    request: `POST {{base_url}}/v1/admin/provider-credentials/{{provider_credential_id}}/disable`,
    response: `HTTP 204 No Content`,
  },
  {
    scope: "admin",
    method: "GET",
    path: "/v1/admin/catalog",
    title: "Katalog kurir internal",
    description:
      "Membaca kurir, layanan, dan profil aturan aktif untuk kebutuhan observasi.",
    authentication: "Bearer admin API key",
    response: `{
  "data": {
    "couriers": [],
    "rounding_profiles": []
  },
  "meta": {
    "request_id": "req_example"
  }
}`,
  },
  {
    scope: "admin",
    method: "GET",
    path: "/v1/admin/locations",
    title: "Cari master wilayah admin",
    description:
      "Pencarian read-only pada master wilayah lokal dengan kontrak respons yang sama seperti endpoint customer.",
    authentication: "Bearer admin API key",
    parameters: ["search — wajib", "limit — opsional, maksimum 50"],
    request: `GET {{base_url}}/v1/admin/locations?search=40135&limit=20`,
    response: `{
  "data": [
    {
      "id": "loc_idn_32_73_02_1004",
      "label": "Dago, Coblong, Kota Bandung, Jawa Barat",
      "postal_codes": ["40135"]
    }
  ],
  "meta": {
    "request_id": "req_example"
  }
}`,
  },
  {
    scope: "admin",
    method: "GET",
    path: "/v1/admin/rate-snapshots",
    title: "Snapshot tarif otomatis",
    description:
      "Membaca quote provider yang tersimpan otomatis. Endpoint ini read-only.",
    authentication: "Bearer admin API key",
    parameters: [
      "search — opsional; kurir, layanan, provider, atau wilayah",
      "limit — opsional, maksimum 200",
      "offset — opsional",
    ],
    response: `{
  "data": [
    {
      "origin_label": "Kebon Jeruk, Andir, Kota Bandung, Jawa Barat",
      "destination_label": "Dago, Coblong, Kota Bandung, Jawa Barat",
      "courier_code": "jne",
      "service_code": "CTC",
      "requested_weight_grams": 1000,
      "returned_cost": 8000,
      "provider_code": "rajaongkir",
      "fresh": true
    }
  ],
  "meta": {
    "limit": 50,
    "offset": 0,
    "request_id": "req_example"
  }
}`,
  },
  {
    scope: "admin",
    method: "GET",
    path: "/v1/admin/location-mappings",
    title: "Mapping lokasi otomatis",
    description:
      "Membaca hubungan ID lokasi lokal dengan ID provider yang dibentuk otomatis. Endpoint ini read-only.",
    authentication: "Bearer admin API key",
    parameters: [
      "search — opsional; wilayah atau ID provider",
      "provider — opsional",
      "limit dan offset — opsional",
    ],
    response: `{
  "data": [
    {
      "location_public_id": "loc_idn_32_73_02_1004",
      "provider_code": "rajaongkir",
      "provider_location_id": "4917",
      "granularity": "subdistrict",
      "active": true
    }
  ],
  "meta": {
    "limit": 50,
    "offset": 0,
    "request_id": "req_example"
  }
}`,
  },
  {
    scope: "admin",
    method: "GET",
    path: "/v1/admin/provider-quotas",
    title: "Ledger kuota provider",
    description:
      "Membaca penggunaan setiap credential alias tanpa pernah mengembalikan API key.",
    authentication: "Bearer admin API key",
    parameters: ["limit — opsional, 1–365"],
    response: `{
  "data": [
    {
      "provider_code": "rajaongkir",
      "credential_alias": "rajaongkir-50k-01",
      "daily_limit": 50000,
      "used_count": 126,
      "remaining_count": 49874,
      "usage_percentage": 0.25,
      "health_status": "healthy"
    }
  ],
  "meta": {
    "request_id": "req_example"
  }
}`,
  },
  {
    scope: "admin",
    method: "GET",
    path: "/v1/admin/api-keys",
    title: "Daftar customer API key",
    description:
      "Membaca metadata dan status key customer. Secret lengkap dan hash tidak pernah dikembalikan.",
    authentication: "Bearer admin API key",
    parameters: [
      "limit — opsional, 1–200; default 50",
      "offset — opsional; default 0",
    ],
    response: `{
  "data": [
    {
      "id": "74e79c7b-1f57-45ad-bcb4-43d8d92f22c5",
      "display_key": "ek_live_Rm8jdK2p••••x7Qn",
      "scopes": ["shipping:read", "tracking:read"],
      "active": true,
      "last_used_at": null,
      "created_by": "bahri",
      "created_at": "2026-07-28T05:20:00Z"
    }
  ],
  "meta": {
    "limit": 50,
    "offset": 0,
    "request_id": "req_example"
  }
}`,
  },
  {
    scope: "admin",
    method: "POST",
    path: "/v1/admin/api-keys",
    title: "Generate customer API key",
    description:
      "Membuat secret acak 256-bit tanpa nama dan tanpa masa berlaku. Key aktif sampai di-revoke. Secret lengkap hanya tersedia pada response pertama.",
    authentication: "Bearer admin API key",
    request: `POST {{base_url}}/v1/admin/api-keys`,
    response: `{
  "data": {
    "api_key": {
      "id": "74e79c7b-1f57-45ad-bcb4-43d8d92f22c5",
      "display_key": "ek_live_Rm8jdK2p••••x7Qn",
      "active": true,
      "scopes": ["shipping:read", "tracking:read"]
    },
    "secret": "ek_live_SECRET_HANYA_TAMPIL_SEKALI"
  },
  "meta": {
    "request_id": "req_example"
  }
}`,
  },
  {
    scope: "admin",
    method: "POST",
    path: "/v1/admin/api-keys/{id}/revoke",
    title: "Revoke customer API key",
    description:
      "Mencabut satu key customer. Key lain, admin key, dan key recovery dari environment tidak berubah.",
    authentication: "Bearer admin API key",
    parameters: ["id — UUID customer API key yang akan dicabut"],
    request: `POST {{base_url}}/v1/admin/api-keys/{{customer_api_key_id}}/revoke`,
    response: `HTTP 204 No Content`,
  },
];
