export type ApiDocumentationScope = "customer" | "admin";
export type ApiDocumentationMethod = "GET" | "POST" | "PUT" | "DELETE";
export type ApiDocumentationContract =
  | "rajaongkir-v2"
  | "emisell-legacy"
  | "canonical"
  | "gateway"
  | "admin";

export type ApiDocumentationContractDefinition = {
  id: ApiDocumentationContract;
  label: string;
  classification: "Public" | "Legacy" | "Internal" | "Admin";
  status: "Stable" | "Compatibility" | "Restricted";
  audience: string;
  basePath: string;
  idFormat: string;
  authentication: string;
  description: string;
};

export type ApiDocumentationEndpoint = {
	contract?: ApiDocumentationContract;
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

export const API_DOCUMENTATION_CONTRACTS: ApiDocumentationContractDefinition[] = [
  {
    id: "rajaongkir-v2",
    label: "RajaOngkir V2",
    classification: "Public",
    status: "Stable",
    audience: "Emisell dan SDK yang sudah memakai RajaOngkir V2",
    basePath: "/api/v1",
    idFormat: "Integer dari snapshot RajaOngkir lokal",
    authentication: "Header key atau Bearer customer API key",
    description:
      "Kontrak publik utama untuk Emisell: pencarian wilayah, cek ongkir, dan tracking dengan bentuk request serta respons kompatibel RajaOngkir V2. Gunakan path ini untuk checkout dan SDK seller.",
  },
  {
    id: "emisell-legacy",
    label: "Emisell Legacy",
    classification: "Legacy",
    status: "Compatibility",
    audience: "Modul Emisell yang masih memakai region-service lama",
    basePath: "/regions dan /shipping",
    idFormat: "Integer dari dump region-service RajaOngkir",
    authentication: "Header key atau Bearer customer API key",
    description:
      "Façade kompatibilitas untuk mengganti region-service lama tanpa mengubah form alamat dan alur ongkir Emisell secara besar.",
  },
  {
    id: "canonical",
    label: "Canonical/Internal",
    classification: "Internal",
    status: "Stable",
    audience: "Dashboard dan service internal API Kurir",
    basePath: "/v1",
    idFormat: "Public ID canonical loc_idn_*",
    authentication: "Header key atau Bearer customer API key",
    description:
      "Kontrak khusus dashboard dan service internal. Jangan gunakan ID loc_idn_* ini pada SDK RajaOngkir V2 atau checkout Emisell.",
  },
  {
    id: "gateway",
    label: "Emisell Gateway",
    classification: "Internal",
    status: "Stable",
    audience: "Backend Emisell yang membawa konteks merchant terverifikasi",
    basePath: "/api/v1",
    idFormat: "merchant_id Emisell yang stabil",
    authentication:
      "Main Service API key (gateway:access) + X-Emisell-Merchant-ID",
    description:
      "Kontrak backend-to-backend untuk menyimpan key provider milik seller dan memastikan tarif, tracking, snapshot, serta kuota tidak bercampur antar-merchant. API Kurir memilih credential internal secara otomatis.",
  },
  {
    id: "admin",
    label: "Admin & Security",
    classification: "Admin",
    status: "Restricted",
    audience: "Operator API Kurir yang terautentikasi",
    basePath: "/v1/admin",
    idFormat: "Canonical ID dan UUID internal",
    authentication: "Bearer admin API key",
    description:
      "Endpoint operasional untuk credential, kuota, API key, katalog, snapshot, dan observasi data internal.",
  },
];

export function getApiDocumentationContract(
  endpoint: ApiDocumentationEndpoint,
): ApiDocumentationContract {
  if (endpoint.contract) return endpoint.contract;
  if (endpoint.scope === "admin") return "admin";
  if (endpoint.path.startsWith("/api/v1")) return "rajaongkir-v2";
  if (
    endpoint.path.startsWith("/regions") ||
    endpoint.path.startsWith("/shipping")
  ) {
    return "emisell-legacy";
  }
  return "canonical";
}

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
      "Mencari master wilayah lokal sampai kelurahan/desa dan kode pos. Alamat mentah dinormalisasi otomatis. Gunakan ID hasil endpoint ini untuk origin dan destination.",
    authentication: "Header key atau Bearer customer API key",
    parameters: [
      "search — wajib, minimal 2 karakter; mendukung alamat mentah, nama wilayah, atau kode pos",
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
    path: "/v1/destination/province",
    title: "Daftar provinsi canonical",
    description:
      "Membaca provinsi lokal dengan public ID canonical untuk service internal.",
    authentication: "Header key atau Bearer customer API key",
    request: `GET {{base_url}}/v1/destination/province`,
    response: `{
  "meta": {
    "message": "Success Get Province",
    "code": 200,
    "status": "success",
    "request_id": "req_example"
  },
  "data": [
    { "id": "loc_idn_32", "name": "Jawa Barat" }
  ]
}`,
  },
  {
    scope: "customer",
    method: "GET",
    path: "/v1/destination/city/{province_id}",
    title: "Kota per provinsi canonical",
    description: "Membaca kota/kabupaten dengan ID canonical parent.",
    authentication: "Header key atau Bearer customer API key",
    parameters: ["province_id — wajib, ID canonical dari daftar provinsi"],
    request: `GET {{base_url}}/v1/destination/city/loc_idn_32`,
    response: `{
  "meta": {
    "message": "Success Get City By Province ID",
    "code": 200,
    "status": "success",
    "request_id": "req_example"
  },
  "data": [
    { "id": "loc_idn_32_73", "name": "Kota Bandung", "zip_code": "" }
  ]
}`,
  },
  {
    scope: "customer",
    method: "GET",
    path: "/v1/destination/district/{city_id}",
    title: "Kecamatan per kota canonical",
    description: "Membaca kecamatan dengan ID canonical parent.",
    authentication: "Header key atau Bearer customer API key",
    parameters: ["city_id — wajib, ID canonical dari daftar kota"],
    request: `GET {{base_url}}/v1/destination/district/loc_idn_32_73`,
    response: `{
  "meta": {
    "message": "Success Get District By City ID",
    "code": 200,
    "status": "success",
    "request_id": "req_example"
  },
  "data": [
    { "id": "loc_idn_32_73_02", "name": "Coblong", "zip_code": "" }
  ]
}`,
  },
  {
    scope: "customer",
    method: "GET",
    path: "/v1/destination/sub-district/{district_id}",
    title: "Kelurahan per kecamatan canonical",
    description:
      "Membaca kelurahan/desa dan kode pos dengan ID canonical parent.",
    authentication: "Header key atau Bearer customer API key",
    parameters: ["district_id — wajib, ID canonical dari daftar kecamatan"],
    request: `GET {{base_url}}/v1/destination/sub-district/loc_idn_32_73_02`,
    response: `{
  "meta": {
    "message": "Success Get Sub District By District ID",
    "code": 200,
    "status": "success",
    "request_id": "req_example"
  },
  "data": [
    {
      "id": "loc_idn_32_73_02_1004",
      "name": "Dago",
      "zip_code": "40135"
    }
  ]
}`,
  },
  {
    scope: "customer",
    method: "GET",
    path: "/api/v1/destination/domestic-destination",
    title: "Cari lokasi SDK RajaOngkir V2",
    description:
      "Direct search wilayah lokal dengan ID numerik dan respons RajaOngkir V2. Alamat mentah, awalan administratif, dan tanda baca dinormalisasi otomatis.",
    authentication: "Header key atau Bearer customer API key",
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
      "id": 4911,
      "label": "Husen Sastranegara, Cicendo, Bandung, Jawa Barat, 40174",
      "province_name": "Jawa Barat",
      "city_name": "Bandung",
      "district_name": "Cicendo",
      "subdistrict_name": "Husen Sastranegara",
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
      "Membaca daftar provinsi dari dump region-service RajaOngkir tanpa hit provider.",
    authentication: "Header key atau Bearer customer API key",
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
      "name": "Maluku Utara"
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
    authentication: "Header key atau Bearer customer API key",
    parameters: [
      "province_id — wajib, ID dari endpoint daftar provinsi",
    ],
    request: `GET {{base_url}}/api/v1/destination/city/5`,
    response: `{
  "meta": {
    "message": "Success Get City By Province ID",
    "code": 200,
    "status": "success"
  },
  "data": [
    {
      "id": 55,
      "name": "Bandung",
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
    authentication: "Header key atau Bearer customer API key",
    parameters: ["city_id — wajib, ID dari endpoint daftar kota"],
    request: `GET {{base_url}}/api/v1/destination/district/55`,
    response: `{
  "meta": {
    "message": "Success Get District By City ID",
    "code": 200,
    "status": "success"
  },
  "data": [
    {
      "id": 442,
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
    authentication: "Header key atau Bearer customer API key",
    parameters: ["district_id — wajib, ID dari endpoint daftar kecamatan"],
    request: `GET {{base_url}}/api/v1/destination/sub-district/442`,
    response: `{
  "meta": {
    "message": "Success Get Sub District By District ID",
    "code": 200,
    "status": "success"
  },
  "data": [
    {
      "id": 4911,
      "name": "Husen Sastranegara",
      "zip_code": "40174"
    }
  ]
}`,
  },
  {
    scope: "customer",
    method: "GET",
    path: "/regions/provinces",
    title: "Region-service · daftar provinsi",
    description:
      "Façade Emisell lama dengan ID numerik yang sama seperti dump region-service RajaOngkir.",
    authentication: "Header key atau Bearer customer API key",
    request: `GET {{base_url}}/regions/provinces
key: {{api_key}}`,
    response: `{
  "data": [
    { "id": 5, "name": "Jawa Barat" }
  ]
}`,
  },
  {
    scope: "customer",
    method: "GET",
    path: "/regions/provinces/{id}",
    title: "Region-service · detail provinsi",
    description: "Membaca satu provinsi berdasarkan ID RajaOngkir lama.",
    authentication: "Header key atau Bearer customer API key",
    parameters: ["id — ID dari daftar provinsi"],
    request: `GET {{base_url}}/regions/provinces/5
key: {{api_key}}`,
    response: `{
  "data": { "id": 5, "name": "Jawa Barat" }
}`,
  },
  {
    scope: "customer",
    method: "GET",
    path: "/regions/cities",
    title: "Region-service · kota per provinsi",
    description: "Kontrak cascading kota yang dipakai form alamat Emisell.",
    authentication: "Header key atau Bearer customer API key",
    parameters: ["provinceId — wajib, ID provinsi lama"],
    request: `GET {{base_url}}/regions/cities?provinceId=5
key: {{api_key}}`,
    response: `{
  "data": [
    { "id": 55, "name": "Bandung" }
  ]
}`,
  },
  {
    scope: "customer",
    method: "GET",
    path: "/regions/cities/{id}",
    title: "Region-service · detail kota",
    description: "Membaca satu kota/kabupaten berdasarkan ID lama.",
    authentication: "Header key atau Bearer customer API key",
    parameters: ["id — ID kota/kabupaten lama"],
    request: `GET {{base_url}}/regions/cities/55
key: {{api_key}}`,
    response: `{
  "data": { "id": 55, "name": "Bandung" }
}`,
  },
  {
    scope: "customer",
    method: "GET",
    path: "/regions/districts",
    title: "Region-service · kecamatan per kota",
    description: "Kontrak cascading kecamatan yang dipakai form alamat Emisell.",
    authentication: "Header key atau Bearer customer API key",
    parameters: ["cityId — wajib, ID kota lama"],
    request: `GET {{base_url}}/regions/districts?cityId=55
key: {{api_key}}`,
    response: `{
  "data": [
    { "id": 442, "name": "Cicendo", "zipCode": "" }
  ]
}`,
  },
  {
    scope: "customer",
    method: "GET",
    path: "/regions/districts/{id}",
    title: "Region-service · detail kecamatan",
    description: "Mengembalikan kecamatan beserta ID dan nama parent-nya.",
    authentication: "Header key atau Bearer customer API key",
    parameters: ["id — ID kecamatan lama"],
    request: `GET {{base_url}}/regions/districts/442
key: {{api_key}}`,
    response: `{
  "data": {
    "id": 442,
    "name": "Cicendo",
    "cityId": 55,
    "cityName": "Bandung",
    "provinceId": 5,
    "provinceName": "Jawa Barat"
  }
}`,
  },
  {
    scope: "customer",
    method: "GET",
    path: "/regions/subdistricts",
    title: "Region-service · kelurahan per kecamatan",
    description: "Mengembalikan kelurahan dan kode pos dengan ID parent lama.",
    authentication: "Header key atau Bearer customer API key",
    parameters: ["districtId — wajib, ID kecamatan lama"],
    request: `GET {{base_url}}/regions/subdistricts?districtId=442
key: {{api_key}}`,
    response: `{
  "data": [
    {
      "id": 4911,
      "name": "Husen Sastranegara",
      "zipCode": "40174",
      "districtId": 442,
      "cityId": 55,
      "provinceId": 5
    }
  ]
}`,
  },
  {
    scope: "customer",
    method: "GET",
    path: "/shipping/domestic-cost",
    title: "Region-service · cek ongkir",
    description:
      "Cek ongkir GET kompatibel Emisell. ID kecamatan lama diterjemahkan ke lokasi internal sebelum provider dipanggil.",
    authentication: "Header key atau Bearer customer API key",
    parameters: [
      "origin dan destination — ID kecamatan lama",
      "weight — berat gram",
      "courier — kode kurir dipisahkan titik dua",
      "price — lowest atau highest, opsional",
      "serviceName — REG, EXPRESS, SAMEDAY, ECONOMY, TRUCKING, CARGO, MOTOR, atau OTHER",
    ],
    request: `GET {{base_url}}/shipping/domestic-cost?origin=1354&destination=2612&weight=1000&courier=jne
key: {{api_key}}`,
    response: `{
  "data": [
    {
      "name": "Jalur Nugraha Ekakurir (JNE)",
      "code": "jne",
      "service": "JTR",
      "description": "JNE Trucking",
      "cost": 55000,
      "etd": "3",
      "serviceName": "TRUCKING"
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
      "Membaca katalog kurir, provider ongkir dan tracking yang terpisah, kemampuan aktif, layanan lokal, dan mode kalkulasi.",
    authentication: "Header key atau Bearer customer API key",
    response: `{
  "data": [
    {
      "code": "jne",
      "name": "JNE",
      "provider_code": "rajaongkir",
      "rate_provider_code": "rajaongkir",
      "tracking_provider_code": "rajaongkir",
      "supports_domestic_cost": true,
      "supports_international_cost": true,
      "supports_tracking": true,
      "catalog_source": "https://rajaongkir.com/docs/shipping-cost/getting_started/courier_availability",
      "tracking_catalog_source": "https://www.rajaongkir.com/docs/shipping-cost/getting_started/courier_availability",
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
    authentication: "Header key atau Bearer customer API key",
    parameters: [
      "origin dan destination — ID lokasi lokal dari endpoint pencarian",
      "weight — berat final/chargeable yang sudah dihitung Emisell dalam gram",
      "courier — satu kode atau beberapa kode dipisahkan tanda titik dua",
      "dimensions — field kompatibilitas opsional; filter layanan memakai weight final",
      "item_value dan options — opsional",
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
      "weight": {
        "actual_grams": 1200,
        "billing_grams": 1200,
        "minimum_accepted_grams": 0,
        "minimum_billable_grams": 0,
        "maximum_accepted_grams": null
      },
      "eligibility": null,
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
      "Endpoint form-urlencoded atau multipart RajaOngkir V2 untuk origin dan destination hasil direct search atau endpoint sub-district. JSON tidak diterima pada kontrak ini. Tambahkan include_group=true bila Emisell membutuhkan pengelompokan layanan.",
    authentication: "Header key atau Bearer customer API key",
    parameters: [
      "origin dan destination — ID integer hasil endpoint sub-district atau domestic-destination",
      "weight — berat final/chargeable dari Emisell dalam gram; layanan di luar batas berat otomatis tidak dikembalikan",
      "courier — kode kurir dipisahkan titik dua",
      "price — lowest mengurutkan semua layanan termurah ke termahal; highest membalik urutan; tidak membatasi jumlah hasil",
      "include_group — true menambahkan canonical_service, service_group, dan service_type; default false agar tetap 1:1 RajaOngkir V2",
    ],
    request: `POST {{base_url}}/api/v1/calculate/domestic-cost
Content-Type: application/x-www-form-urlencoded
key: {{api_key}}

origin=4911&destination=25976&weight=1000&courier=jne&price=lowest&include_group=true`,
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
      "canonical_service": "REG",
      "service_group": "regular",
      "service_type": "parcel",
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
      "Drop-in endpoint form-urlencoded atau multipart untuk SDK RajaOngkir V2. Gunakan /api/v1/calculate/domestic-cost dengan bentuk request yang sama untuk ID subdistrict. JSON tidak diterima. Tambahkan include_group=true bila Emisell membutuhkan pengelompokan layanan.",
    authentication: "Header key atau Bearer customer API key",
    parameters: [
      "origin dan destination — ID integer hasil endpoint district",
      "weight — berat final/chargeable dari Emisell dalam gram; layanan di luar batas berat otomatis tidak dikembalikan",
      "courier — kode kurir dipisahkan titik dua",
      "price — lowest mengurutkan semua layanan termurah ke termahal; highest membalik urutan; tidak membatasi jumlah hasil",
      "include_group — true menambahkan canonical_service, service_group, dan service_type; default false agar tetap 1:1 RajaOngkir V2",
    ],
    request: `POST {{base_url}}/api/v1/calculate/district/domestic-cost
Content-Type: application/x-www-form-urlencoded
key: {{api_key}}

origin=442&destination=2165&weight=1000&courier=jne&price=lowest&include_group=true`,
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
      "canonical_service": "REG",
      "service_group": "regular",
      "service_type": "parcel",
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
      "Drop-in tracking sinkron dengan envelope dan field RajaOngkir V2. Snapshot, checkpoint 12/24 jam, dan negative cache memastikan pengecekan berulang tidak selalu memakai hit provider.",
    authentication: "Header key atau Bearer customer API key",
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
    authentication: "Header key atau Bearer customer API key",
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
    "waybill": "0123456789012",
    "status": "in_transit",
    "status_label": "Dalam perjalanan",
    "summary": {},
    "events": [],
    "provider": "rajaongkir",
    "provider_fetched_at": "2026-07-28T05:20:00Z",
    "next_refresh_at": "2026-07-28T17:20:00Z",
    "is_final": false,
    "refresh_queued": false,
    "last_error_code": "",
    "validation_status": "valid",
    "provider_hit_count": 3,
    "provider_hit_limit": 10,
    "polling_stopped": false
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
      "Membaca katalog kemampuan, sumber tarif RajaOngkir, dan provider tracking untuk menu Ekspedisi & Service.",
    authentication: "Bearer admin API key",
    response: `{
  "data": [
    {
      "code": "jne",
      "name": "JNE",
      "provider_code": "rajaongkir",
      "rate_provider_code": "rajaongkir",
      "tracking_provider_code": "rajaongkir",
      "supports_domestic_cost": true,
      "supports_international_cost": true,
      "supports_tracking": true,
      "catalog_source": "https://rajaongkir.com/docs/shipping-cost/getting_started/courier_availability",
      "tracking_catalog_source": "https://www.rajaongkir.com/docs/shipping-cost/getting_started/courier_availability",
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
      "Membaca snapshot tracking atau mengantrikan refresh ke worker. Nomor resi disimpan plaintext untuk integrasi backend Emisell.",
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
    "waybill": "0123456789012",
    "status": "in_transit",
    "status_label": "Dalam perjalanan",
    "events": [],
    "provider": "rajaongkir",
    "refresh_queued": false,
    "is_final": false,
    "last_error_code": "",
    "validation_status": "valid",
    "provider_hit_count": 3,
    "provider_hit_limit": 10,
    "polling_stopped": false
  },
  "meta": {"request_id": "req_example"}
}`,
  },
  {
    scope: "admin",
    method: "GET",
    path: "/v1/admin/tracking-operations",
    title: "Monitor resi dan antrean worker",
    description:
      "Sumber tabel operasional staff untuk melihat AWB penuh, validasi, merchant/order, revision, snapshot, provider hit, job pending/running/dead, serta jadwal refresh. AWB penuh hanya dibuka pada endpoint admin ini.",
    authentication: "Bearer admin API key",
    parameters: [
      "search — merchant, order, fulfillment, empat karakter akhir resi, courier, atau provider",
      "courier — filter kode ekspedisi",
      "validation_status — unverified, valid, not_found, atau invalid",
      "queue_status — pending, running, dead, final, atau idle",
      "limit dan offset — pagination",
    ],
    request: `GET {{base_url}}/v1/admin/tracking-operations?queue_status=running&limit=100`,
    response: `{
  "data": {
    "items": [{
      "courier": "jnt",
      "waybill": "JY1224870535",
      "validation_status": "valid",
      "status": "in_transit",
      "queue_status": "running",
      "provider": "rajaongkir",
      "provider_hit_count": 2,
      "provider_hit_limit": 10,
      "subscription_revision": 2
    }],
    "total": 1,
    "summary": {"total": 120, "pending": 8, "running": 2, "failed": 1, "invalid": 3, "final": 86}
  }
}`,
  },
  {
    scope: "admin",
    method: "DELETE",
    path: "/v1/admin/tracking-operations/{id}",
    title: "Hapus permanen data resi",
    description:
      "Menghapus shipment beserta antrean worker, snapshot history, subscription, revision, dan webhook outbox terkait. Tersedia di semua environment untuk staff dashboard; tindakan tetap dicatat pada audit log.",
    authentication: "Bearer admin API key",
    parameters: [
      "id — UUID internal shipment dari hasil GET /v1/admin/tracking-operations",
      "X-Admin-Actor — identitas staff yang melakukan penghapusan",
    ],
    request: `DELETE {{base_url}}/v1/admin/tracking-operations/{{tracking_shipment_id}}`,
    response: `HTTP 204 No Content`,
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
      "Memvalidasi key ke RajaOngkir atau Biteship, mengenkripsinya, dan langsung mengaktifkannya tanpa restart. Biteship hanya digunakan untuk fallback tracking.",
    authentication: "Bearer admin API key",
    parameters: [
      "provider_code — rajaongkir atau biteship",
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
    path: "/v1/admin/provider-credentials",
    title: "Tambah token fallback Biteship",
    description:
      "Menyimpan token Biteship terenkripsi untuk tracking kurir fallback seperti SiCepat. Token tidak digunakan untuk cek ongkir atau sinkronisasi katalog.",
    authentication: "Bearer admin API key",
    parameters: [
      "provider_code — biteship",
      "api_key — token biteship_live.* atau biteship_test.*",
    ],
    request: `{
  "provider_code": "biteship",
  "api_key": "{{biteship_api_token}}"
}`,
    response: `{
  "data": {
    "id": "be4c62aa-a700-4e23-bfc0-02072791b59e",
    "provider_code": "biteship",
    "credential_alias": "biteship-6c8f212a",
    "display_key": "bite••••2026",
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
      "tenant_id": "merchant_123",
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
    contract: "admin",
    scope: "admin",
    method: "GET",
    path: "/v1/admin/shipping-providers",
    title: "Master provider integrasi",
    description:
      "Membaca provider yang dapat ditampilkan pada extension Emisell beserta logo, deskripsi, status kesiapan, jumlah credential, instalasi, dan merchant aktif.",
    authentication: "Bearer admin API key",
    request: `GET {{base_url}}/v1/admin/shipping-providers
Authorization: Bearer {{admin_api_key}}`,
    response: `{
  "data": [
    {
      "code": "rajaongkir",
      "name": "RajaOngkir",
      "logo": "https://api-kurir.emisell.com/provider-logos/rajaongkir.svg",
      "description": "Integrasi RajaOngkir menggunakan API key milik seller untuk cek ongkir dan pelacakan sesuai paket akun seller.",
      "built_in": false,
      "requires_credential": true,
      "available": true,
      "display_order": 20,
      "installed_merchant_count": 12,
      "active_merchant_count": 8,
      "credential_count": 14
    }
  ],
  "meta": { "request_id": "req_example" }
}`,
  },
  {
    contract: "admin",
    scope: "admin",
    method: "POST",
    path: "/v1/admin/shipping-providers",
    title: "Tambah provider integrasi",
    description:
      "Menambahkan provider eksternal baru. Provider otomatis membutuhkan credential seller dan berstatus belum tersedia sampai adapter selesai diuji.",
    authentication: "Bearer admin API key + X-Admin-Actor",
    parameters: [
      "code — kode permanen provider, huruf kecil tanpa spasi",
      "logo — URL HTTPS publik permanen",
      "display_order — urutan 1–9999",
    ],
    request: `POST {{base_url}}/v1/admin/shipping-providers
Authorization: Bearer {{admin_api_key}}
X-Admin-Actor: emisell
Content-Type: application/json

{
  "code": "mengantar",
  "name": "Mengantar",
  "logo": "https://api-kurir.emisell.com/provider-logos/default.svg",
  "description": "Integrasi provider Mengantar untuk merchant Emisell.",
  "display_order": 40
}`,
    response: `HTTP 201 · provider dibuat dengan available=false, built_in=false, dan requires_credential=true.`,
  },
  {
    contract: "admin",
    scope: "admin",
    method: "PUT",
    path: "/v1/admin/shipping-providers/{provider_code}",
    title: "Perbarui provider integrasi",
    description:
      "Mengubah nama, logo, deskripsi, urutan, dan kesiapan provider. Kode serta model credential dikunci; provider yang masih dipakai merchant tidak dapat dibuat unavailable.",
    authentication: "Bearer admin API key + X-Admin-Actor",
    parameters: ["provider_code — kode permanen dari master provider"],
    request: `PUT {{base_url}}/v1/admin/shipping-providers/mengantar
Authorization: Bearer {{admin_api_key}}
X-Admin-Actor: emisell
Content-Type: application/json

{
  "name": "Mengantar",
  "logo": "https://api-kurir.emisell.com/provider-logos/default.svg",
  "description": "Integrasi provider Mengantar yang telah lolos pengujian adapter.",
  "available": true,
  "display_order": 40
}`,
    response: `HTTP 200 · object provider terbaru beserta jumlah merchant dan credential.`,
  },
  {
    scope: "admin",
    method: "GET",
    path: "/v1/admin/tracking-webhook",
    title: "Baca pengaturan webhook tracking",
    description:
      "Membaca URL callback, status aktif, mask secret, sumber konfigurasi, dan hasil test terakhir. Plaintext secret tidak pernah dikembalikan.",
    authentication: "Bearer admin API key",
    response: `{
  "data": {
    "configured": true,
    "callback_url": "https://api.emisell.com/api/v1/webhooks/tracking",
    "enabled": true,
    "secret_configured": true,
    "secret_hint": "whsec_ab••••••••2026",
    "source": "database",
    "last_test_success": true
  }
}`,
  },
  {
    scope: "admin",
    method: "PUT",
    path: "/v1/admin/tracking-webhook",
    title: "Simpan URL dan aktivasi webhook",
    description:
      "Menyimpan endpoint backend Emisell. Production wajib HTTPS publik dan webhook hanya dapat diaktifkan setelah secret tersedia.",
    authentication: "Bearer admin API key",
    parameters: [
      "callback_url — URL penerima webhook di backend Emisell",
      "enabled — aktifkan atau hentikan delivery event",
    ],
    request: `{
  "callback_url": "https://api.emisell.com/api/v1/webhooks/tracking",
  "enabled": true
}`,
    response: `{
  "data": {
    "configured": true,
    "enabled": true,
    "secret_configured": true,
    "source": "database"
  }
}`,
  },
  {
    scope: "admin",
    method: "POST",
    path: "/v1/admin/tracking-webhook/secret",
    title: "Generate atau rotate webhook secret",
    description:
      "Menghasilkan secret HMAC 256-bit. Plaintext hanya tampil pada response ini; database menyimpan ciphertext dan mask.",
    authentication: "Bearer admin API key",
    request: `POST {{base_url}}/v1/admin/tracking-webhook/secret`,
    response: `{
  "data": {
    "settings": {
      "secret_configured": true,
      "secret_hint": "whsec_ab••••••••2026"
    },
    "secret": "whsec_<secret-hanya-tampil-sekali>"
  }
}`,
  },
  {
    scope: "admin",
    method: "POST",
    path: "/v1/admin/tracking-webhook/test",
    title: "Test webhook Emisell",
    description:
      "Mengirim event tracking.test bertanda tangan tanpa merchant, order, AWB, alamat, atau identitas penerima.",
    authentication: "Bearer admin API key",
    request: `POST {{base_url}}/v1/admin/tracking-webhook/test`,
    response: `{
  "data": {
    "success": true,
    "http_status": 202,
    "event_id": "evt_test_<random>",
    "tested_at": "2026-08-20T12:00:00Z",
    "message": "Webhook test diterima Emisell."
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
      "display_key": "ek_live_example••••masked",
      "kind": "public",
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
    title: "Generate Public API atau Main Service key",
    description:
      "Membuat secret acak 256-bit tanpa nama dan masa berlaku. Pilih main_service untuk akses gateway:access atau public untuk API umum. Secret lengkap hanya tersedia pada response pertama.",
    authentication: "Bearer admin API key",
    request: `POST {{base_url}}/v1/admin/api-keys
Content-Type: application/json

{
  "kind": "main_service"
}`,
    response: `{
  "data": {
    "api_key": {
      "id": "74e79c7b-1f57-45ad-bcb4-43d8d92f22c5",
      "display_key": "ek_live_example••••masked",
      "active": true,
      "kind": "main_service",
      "scopes": ["shipping:read", "tracking:read", "gateway:access"]
    },
    "secret": "ek_live_<secret-hanya-tampil-sekali>"
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
  {
    contract: "gateway",
    scope: "customer",
    method: "POST",
    path: "/api/v1/tracking/verify",
    title: "Verifikasi AWB sebelum disimpan",
    description:
      "Memeriksa AWB ke ekspedisi pilihan seller. Jika tidak ditemukan, API Kurir mencoba maksimal dua kandidat kuat berdasarkan format. Pola hanya petunjuk; format baru tetap diproses ke provider yang dipilih.",
    authentication: "Customer API key; X-Emisell-Merchant-ID opsional",
    parameters: [
      "courier — ekspedisi yang dipilih seller",
      "waybill — AWB 6–40 karakter",
      "last_phone_number — opsional",
    ],
    request: `POST {{base_url}}/api/v1/tracking/verify
key: {{api_key}}
Content-Type: application/json

{
  "courier": "jne",
  "waybill": "JY1224870535"
}`,
    response: `{
  "data": {
    "status": "courier_mismatch",
    "requested_courier": "jne",
    "detected_courier": "jnt",
    "format_status": "possible",
    "candidate_couriers": ["jne", "jnt"],
    "provider_checked": true,
    "message": "Nomor resi valid, tetapi milik ekspedisi lain."
  }
}`,
  },
  {
    contract: "gateway",
    scope: "customer",
    method: "POST",
    path: "/api/v1/integrations/tracking/subscriptions",
    title: "Daftarkan tracking fulfillment",
    description:
      "Mendaftarkan AWB satu kali untuk checkpoint tracking hemat. Worker melakukan maksimal 10 hit sepanjang siklus, sedangkan pembacaan seller/customer selalu memakai snapshot lokal.",
    authentication: "Main Service API key (gateway:access) + X-Emisell-Merchant-ID",
    parameters: [
      "order_id — ID order Emisell, maksimal 128 karakter",
      "fulfillment_id — ID fulfillment unik dalam merchant",
      "courier — kode kurir canonical",
      "waybill — AWB 6–40 karakter",
      "merchant_id dikirim melalui header, bukan body",
    ],
    request: `POST {{base_url}}/api/v1/integrations/tracking/subscriptions
key: {{api_key}}
X-Emisell-Merchant-ID: {{merchant_id}}
Content-Type: application/json

{
  "order_id": "order_123",
  "fulfillment_id": "fulfillment_123",
  "courier": "jne",
  "waybill": "TEST123456789"
}`,
    response: `{
  "meta": {
    "message": "Tracking subscription registered",
    "code": 202,
    "status": "success",
    "request_id": "req_example"
  },
  "data": {
    "id": "subscription_uuid",
    "order_id": "order_123",
    "fulfillment_id": "fulfillment_123",
    "active": true,
    "shipment": {
      "courier": "jne",
      "waybill": "JY1224876789",
      "status": "unknown",
      "validation_status": "unverified",
      "provider_hit_count": 0,
      "provider_hit_limit": 10,
      "refresh_queued": true,
      "polling_stopped": false
    }
  }
}`,
  },
  {
    contract: "gateway",
    scope: "customer",
    method: "PUT",
    path: "/api/v1/integrations/tracking/subscriptions/{fulfillment_id}",
    title: "Ganti AWB fulfillment dengan aman",
    description:
      "AWB lama tetap aktif saat AWB baru diverifikasi. Setelah valid dan sesuai courier, pergantian dilakukan atomik, revision naik satu, dan hubungan AWB lama disimpan sebagai audit. Status final dikunci.",
    authentication: "Main Service API key (gateway:access) + X-Emisell-Merchant-ID",
    parameters: [
      "fulfillment_id — fulfillment yang akan diganti",
      "expected_revision — revision terakhir yang dibaca Emisell; wajib",
      "order_id, courier, waybill — data pengganti",
    ],
    request: `PUT {{base_url}}/api/v1/integrations/tracking/subscriptions/fulfillment_123
key: {{api_key}}
X-Emisell-Merchant-ID: {{merchant_id}}
Content-Type: application/json

{
  "order_id": "order_123",
  "courier": "jnt",
  "waybill": "JY1224870535",
  "expected_revision": 1
}`,
    response: `{
  "data": {
    "subscription": {
      "fulfillment_id": "fulfillment_123",
      "revision": 2,
      "active": true
    },
    "verification": {
      "status": "verified",
      "requested_courier": "jnt",
      "detected_courier": "jnt"
    }
  }
}`,
  },
  {
    contract: "gateway",
    scope: "customer",
    method: "GET",
    path: "/api/v1/integrations/tracking/subscriptions/{fulfillment_id}",
    title: "Baca snapshot tracking fulfillment",
    description:
      "Hanya membaca snapshot PostgreSQL berdasarkan merchant dan fulfillment. Endpoint ini tidak memanggil RajaOngkir/Biteship dan aman dipakai berulang oleh backend Emisell.",
    authentication: "Main Service API key (gateway:access) + X-Emisell-Merchant-ID",
    parameters: ["fulfillment_id — ID fulfillment yang sebelumnya didaftarkan"],
    request: `GET {{base_url}}/api/v1/integrations/tracking/subscriptions/{{fulfillment_id}}
key: {{api_key}}
X-Emisell-Merchant-ID: {{merchant_id}}`,
    response: `{
  "meta": {
    "message": "Success Get Tracking Subscription",
    "code": 200,
    "status": "success",
    "request_id": "req_example"
  },
  "data": {
    "order_id": "order_123",
    "fulfillment_id": "fulfillment_123",
    "active": true,
    "shipment": {
      "courier": "jne",
      "waybill": "JY1224876789",
      "status": "in_transit",
      "status_label": "Dalam perjalanan",
      "validation_status": "valid",
      "provider": "rajaongkir",
      "provider_fetched_at": "2026-08-20T10:00:00Z",
      "next_refresh_at": "2026-08-20T22:00:00Z",
      "provider_hit_count": 3,
      "provider_hit_limit": 10,
      "polling_stopped": false
    }
  }
}`,
  },
  {
    contract: "gateway",
    scope: "customer",
    method: "DELETE",
    path: "/api/v1/integrations/tracking/subscriptions/{fulfillment_id}",
    title: "Hapus tracking fulfillment merchant",
    description:
      "Menonaktifkan hubungan tracking milik merchant tanpa membuang snapshot dan riwayat audit. Webhook tertunda dibatalkan; polling berhenti jika snapshot tidak lagi memiliki subscription aktif. Request ulang aman dan POST dapat mengaktifkannya kembali.",
    authentication: "Main Service API key (gateway:access) + X-Emisell-Merchant-ID",
    parameters: [
      "fulfillment_id — fulfillment milik merchant yang tracking-nya dihentikan",
      "merchant_id wajib berasal dari header backend, bukan request body",
    ],
    request: `DELETE {{base_url}}/api/v1/integrations/tracking/subscriptions/{{fulfillment_id}}
key: {{api_key}}
X-Emisell-Merchant-ID: {{merchant_id}}`,
    response: `{
  "meta": {
    "message": "Tracking subscription removed",
    "code": 200,
    "status": "success",
    "request_id": "req_example"
  },
  "data": {
    "id": "subscription_uuid",
    "fulfillment_id": "fulfillment_123",
    "active": false,
    "revision": 1,
    "status": "removed",
    "polling_stopped": true,
    "snapshot_retained": true
  }
}`,
  },
  {
    contract: "gateway",
    scope: "customer",
    method: "GET",
    path: "/api/v1/integrations/provider-credentials",
    title: "Daftar credential provider milik merchant",
    description:
      "Mengembalikan metadata key provider milik merchant tanpa secret, UUID internal, atau tenant_id.",
    authentication: "Main Service API key (gateway:access) + X-Emisell-Merchant-ID",
    request: `GET {{base_url}}/api/v1/integrations/provider-credentials
key: {{api_key}}
X-Emisell-Merchant-ID: {{merchant_id}}`,
    response: `{
  "data": [
    {
      "provider_code": "rajaongkir",
      "display_key": "demo••••1234",
      "daily_limit": 50000,
      "active": true,
      "validation_status": "valid"
    }
  ],
  "meta": { "request_id": "req_example" }
}`,
  },
  {
    contract: "gateway",
    scope: "customer",
    method: "POST",
    path: "/api/v1/integrations/provider-credentials",
    title: "Hubungkan key RajaOngkir seller",
    description:
      "Memvalidasi key ke provider, mengenkripsinya dengan AES-256-GCM, dan mengikat credential ke merchant dari header. Key baru otomatis menggantikan key aktif lama untuk provider yang sama.",
    authentication: "Main Service API key (gateway:access) + X-Emisell-Merchant-ID",
    request: `POST {{base_url}}/api/v1/integrations/provider-credentials
key: {{api_key}}
X-Emisell-Merchant-ID: {{merchant_id}}
Content-Type: application/json

{
  "provider_code": "rajaongkir",
  "api_key": "{{seller_rajaongkir_key}}",
  "daily_limit": 50000
}`,
    response: `{
  "data": {
    "provider_code": "rajaongkir",
    "display_key": "demo••••1234",
    "daily_limit": 50000,
    "active": true,
    "validation_status": "valid"
  },
  "meta": { "request_id": "req_example" }
}`,
  },
  {
    contract: "gateway",
    scope: "customer",
    method: "POST",
    path: "/api/v1/integrations/provider-credentials/{provider_code}/disable",
    title: "Putuskan credential seller",
    description:
      "Menonaktifkan key aktif merchant berdasarkan provider code. Emisell tidak menyimpan credential ID.",
    authentication: "Main Service API key (gateway:access) + X-Emisell-Merchant-ID",
    parameters: ["provider_code — saat ini rajaongkir"],
    request: `POST {{base_url}}/api/v1/integrations/provider-credentials/rajaongkir/disable
key: {{api_key}}
X-Emisell-Merchant-ID: {{merchant_id}}`,
    response: `HTTP 204 No Content`,
  },
  {
    contract: "gateway",
    scope: "customer",
    method: "GET",
    path: "/api/v1/integrations/providers",
    title: "Katalog provider dan extension aktif",
    description:
      "Menampilkan hanya provider yang tersedia, metadata logo/deskripsi untuk listing extension, dan provider efektif merchant. Provider yang dimatikan admin otomatis tidak muncul di dashboard Emisell. Merchant baru berstatus nonaktif sampai seller memilih provider.",
    authentication: "Main Service API key (gateway:access) + X-Emisell-Merchant-ID",
    request: `GET {{base_url}}/api/v1/integrations/providers
key: {{api_key}}
X-Emisell-Merchant-ID: {{merchant_id}}`,
    response: `{
  "data": {
    "active_provider_code": null,
    "version": 0,
    "providers": [
      {
        "code": "emisell",
        "name": "Emisell Kurir",
        "logo": "https://api-kurir.emisell.com/provider-logos/emisell.svg",
        "description": "Layanan pengiriman bawaan Emisell dengan tarif dan pelacakan terpusat tanpa API key provider dari seller.",
        "built_in": true,
        "requires_credential": false,
        "available": true,
        "installed": true,
        "active": false
      },
      {
        "code": "rajaongkir",
        "name": "RajaOngkir",
        "logo": "https://api-kurir.emisell.com/provider-logos/rajaongkir.svg",
        "description": "Integrasi RajaOngkir menggunakan API key milik seller untuk cek ongkir dan pelacakan sesuai paket akun seller.",
        "built_in": false,
        "requires_credential": true,
        "available": true,
        "installed": true,
        "active": false
      }
    ]
  },
  "meta": { "request_id": "req_example" }
}`,
  },
  {
    contract: "gateway",
    scope: "customer",
    method: "POST",
    path: "/api/v1/integrations/providers/{provider_code}/activate",
    title: "Aktifkan satu provider merchant",
    description:
      "Mengganti provider aktif secara atomik. API Kurir otomatis memilih key aktif milik merchant untuk provider eksternal. expected_version mencegah perubahan paralel saling menimpa.",
    authentication: "Main Service API key (gateway:access) + X-Emisell-Merchant-ID",
    parameters: [
      "provider_code — emisell atau provider eksternal yang available",
      "expected_version — version terakhir dari GET providers",
    ],
    request: `POST {{base_url}}/api/v1/integrations/providers/rajaongkir/activate
key: {{api_key}}
X-Emisell-Merchant-ID: {{merchant_id}}
Content-Type: application/json

{
  "expected_version": 0
}`,
    response: `{
  "data": {
    "active_provider_code": "rajaongkir",
    "version": 1,
    "providers": [
      {
        "code": "emisell",
        "name": "Emisell Kurir",
        "logo": "https://api-kurir.emisell.com/provider-logos/emisell.svg",
        "description": "Layanan pengiriman bawaan Emisell dengan tarif dan pelacakan terpusat tanpa API key provider dari seller.",
        "active": false,
        "installed": true
      },
      {
        "code": "rajaongkir",
        "name": "RajaOngkir",
        "logo": "https://api-kurir.emisell.com/provider-logos/rajaongkir.svg",
        "description": "Integrasi RajaOngkir menggunakan API key milik seller untuk cek ongkir dan pelacakan sesuai paket akun seller.",
        "active": true,
        "installed": true
      }
    ]
  },
  "meta": { "request_id": "req_example" }
}`,
  },
  {
    contract: "gateway",
    scope: "customer",
    method: "POST",
    path: "/api/v1/integrations/providers/{provider_code}/deactivate",
    title: "Nonaktifkan provider merchant",
    description:
      "Menonaktifkan provider yang sedang dipakai tanpa mengaktifkan pengganti otomatis. Emisell Kurir juga dapat dinonaktifkan. Provider yang sudah tidak aktif menghasilkan respons sukses yang sama agar aman diulang.",
    authentication: "Main Service API key (gateway:access) + X-Emisell-Merchant-ID",
    parameters: [
      "provider_code — emisell atau provider eksternal yang ingin dinonaktifkan",
      "expected_version — version terakhir dari GET providers",
    ],
    request: `POST {{base_url}}/api/v1/integrations/providers/rajaongkir/deactivate
key: {{api_key}}
X-Emisell-Merchant-ID: {{merchant_id}}
Content-Type: application/json

{ "expected_version": 1 }`,
    response: `{
  "data": {
    "active_provider_code": null,
    "version": 2,
    "providers": [
      {
        "code": "emisell",
        "name": "Emisell Kurir",
        "logo": "https://api-kurir.emisell.com/provider-logos/emisell.svg",
        "description": "Layanan pengiriman bawaan Emisell dengan tarif dan pelacakan terpusat tanpa API key provider dari seller.",
        "active": false,
        "installed": true
      },
      {
        "code": "rajaongkir",
        "name": "RajaOngkir",
        "logo": "https://api-kurir.emisell.com/provider-logos/rajaongkir.svg",
        "description": "Integrasi RajaOngkir menggunakan API key milik seller untuk cek ongkir dan pelacakan sesuai paket akun seller.",
        "active": false,
        "installed": true
      }
    ]
  },
  "meta": { "request_id": "req_example" }
}`,
  },
  {
    contract: "gateway",
    scope: "customer",
    method: "GET",
    path: "/api/v1/integrations/shipping-services",
    title: "Katalog dan pilihan layanan checkout",
    description:
      "Mengembalikan kurir canonical dan layanan dari grup regular, next_day, economy, atau cargo beserta capability, status pilihan, limit, dan selectable. Grup lain tidak dikirim ke Emisell dan tidak dapat dipilih.",
    authentication: "Main Service API key (gateway:access) + X-Emisell-Merchant-ID",
    request: `GET {{base_url}}/api/v1/integrations/shipping-services
key: {{api_key}}
X-Emisell-Merchant-ID: {{merchant_id}}`,
    response: `{
  "data": {
    "preference": {
      "configured": true,
      "mode": "custom",
      "enabled_groups": [],
      "services": [
        { "courier_code": "jne", "service_code": "REG" },
        { "courier_code": "jne", "service_code": "YES" }
      ],
      "version": 2,
      "updated_at": "2026-08-19T10:00:00Z"
    },
    "limits": {
      "enforced": true,
      "couriers": {
        "maximum": 5,
        "selected": 1,
        "remaining": 4,
        "available": 14
      },
      "services": {
        "maximum": 20,
        "selected": 2,
        "remaining": 18,
        "available": 88
      }
    },
    "groups": [
      { "code": "regular", "name": "Regular" },
      { "code": "next_day", "name": "Next Day" },
      { "code": "economy", "name": "Economy" },
      { "code": "cargo", "name": "Cargo" }
    ],
    "couriers": [
      {
        "code": "jne",
        "name": "JNE",
        "provider_code": "rajaongkir",
        "rate_provider_code": "rajaongkir",
        "tracking_provider_code": "rajaongkir",
        "selection_state": "partial",
        "selectable": true,
        "selected_service_count": 2,
        "total_service_count": 9,
        "supports_domestic_cost": true,
        "supports_international_cost": true,
        "supports_tracking": true,
        "services": [
          {
            "code": "REG",
            "name": "JNE Regular",
            "group": "regular",
            "service_type": "parcel",
            "calculation_mode": "provider_quote",
            "selected": true,
            "selectable": true
          },
          {
            "code": "JTR",
            "name": "JNE Trucking",
            "group": "cargo",
            "service_type": "cargo",
            "calculation_mode": "provider_quote",
            "selected": false,
            "selectable": true
          }
        ]
      }
    ]
  },
  "meta": { "request_id": "req_example" }
}`,
  },
  {
    contract: "gateway",
    scope: "customer",
    method: "PUT",
    path: "/api/v1/integrations/shipping-services",
    title: "Simpan layanan yang tampil di checkout",
    description:
      "Mengganti pilihan custom merchant secara atomik. Mode all dan groups tidak diterima. Server menolak request yang melebihi limit kurir atau layanan tanpa mengubah pilihan lama.",
    authentication: "Main Service API key (gateway:access) + X-Emisell-Merchant-ID",
    parameters: [
      "services — seluruh pasangan courier_code + canonical service_code yang dipilih",
      "group yang dapat dipilih hanya regular, next_day, economy, dan cargo",
      "mode — opsional; bila dikirim hanya boleh custom",
      "enabled_groups — legacy opsional dan harus berupa array kosong",
      "maksimum default 5 kurir dan 20 layanan; baca limit aktual dari GET",
      "merchant_id dikirim melalui header dan tidak diterima dari body",
    ],
    request: `PUT {{base_url}}/api/v1/integrations/shipping-services
key: {{api_key}}
X-Emisell-Merchant-ID: {{merchant_id}}
Content-Type: application/json

{
  "services": [
    { "courier_code": "jne", "service_code": "REG" },
    { "courier_code": "jne", "service_code": "YES" },
    { "courier_code": "jnt", "service_code": "EZ" }
  ]
}`,
    response: `{
  "data": {
    "configured": true,
    "mode": "custom",
    "enabled_groups": [],
    "services": [
      { "courier_code": "jne", "service_code": "REG" },
      { "courier_code": "jne", "service_code": "YES" },
      { "courier_code": "jnt", "service_code": "EZ" }
    ],
    "version": 1,
    "updated_at": "2026-08-19T10:00:00Z"
  },
  "meta": { "request_id": "req_example" }
}`,
  },
  {
    contract: "gateway",
    scope: "customer",
    method: "POST",
    path: "/api/v1/calculate/district/domestic-cost",
    title: "Cek ongkir dengan key seller",
    description:
      "Kontrak respons tetap RajaOngkir V2. Main Service tidak mengirim courier; API Kurir mengambil kurir dari service yang dipilih merchant, memilih credential provider aktif, lalu memfilter hasil sesuai pilihan seller. Courier dari client lama diabaikan. Jika seller belum mengaktifkan kurir, respons HTTP 409 menandakan shipping masih nonaktif.",
    authentication: "Main Service API key (gateway:access) + X-Emisell-Merchant-ID",
    request: `POST {{base_url}}/api/v1/calculate/district/domestic-cost
key: {{api_key}}
X-Emisell-Merchant-ID: {{merchant_id}}
Content-Type: application/x-www-form-urlencoded

origin=442&destination=1354&weight=1200`,
    response: `Respons 1:1 RajaOngkir V2 dan hanya memuat service yang dipilih merchant. Jika belum ada service terpilih, API mengembalikan HTTP 422 RATE_NOT_AVAILABLE. Public API tanpa Merchant ID tetap mewajibkan courier.`,
  },
  {
    contract: "gateway",
    scope: "customer",
    method: "POST",
    path: "/api/v1/track/waybill",
    title: "Tracking dengan key seller",
    description:
      "Tracking sinkron dan refresh worker mempertahankan merchant serta credential internal milik order. Kurir fallback seperti SiCepat dapat dijawab Biteship tanpa mengubah kontrak respons.",
    authentication: "Main Service API key (gateway:access) + X-Emisell-Merchant-ID",
    request: `POST {{base_url}}/api/v1/track/waybill
key: {{api_key}}
X-Emisell-Merchant-ID: {{merchant_id}}
Content-Type: application/x-www-form-urlencoded

awb=TEST123456789&courier=sicepat`,
    response: `Respons tetap memakai envelope kompatibel RajaOngkir V2 tanpa field tambahan. Provider aktual dan hit dapat diaudit dari ledger admin serta snapshot tracking internal.`,
  },
];
