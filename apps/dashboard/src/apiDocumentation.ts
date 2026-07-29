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
      "province": "Jawa Barat",
      "city": "Kota Bandung",
      "district": "Coblong",
      "subdistrict": "Dago",
      "postal_code": "40135",
      "postal_codes": ["40135"]
    }
  ],
  "meta": {
    "request_id": "req_example",
    "next_cursor": null
  }
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
    path: "/v1/track/waybill",
    title: "Cek resi",
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
