import { createServer } from "node:http";
import { randomUUID } from "node:crypto";
import { pathToFileURL } from "node:url";

export const VERSION = "1.0.3";
export const PROVIDER_CODE = "rajaongkir";
export const PROVIDER_NAME = "RajaOngkir";

const PORT = integer(process.env.PORT, 8080);
const COST_BASE_URL = normalizedBaseURL(
  process.env.SHIPPING_COST_BASE_URL,
  "https://rajaongkir.komerce.id/api/v1/",
);
const DELIVERY_BASE_URL = normalizedBaseURL(
  process.env.SHIPPING_DELIVERY_BASE_URL,
  process.env.RAJAONGKIR_DELIVERY_ENV === "sandbox"
    ? "https://api-sandbox.collaborator.komerce.id/"
    : "https://api.collaborator.komerce.id/",
);
const MAX_BODY_BYTES = 64 * 1024;
const UPSTREAM_TIMEOUT_MS = integer(process.env.UPSTREAM_TIMEOUT_MS, 8000);
const DEFAULT_COURIERS = [
  "jne", "sicepat", "ide", "sap", "jnt", "ninja", "tiki", "lion",
  "anteraja", "pos", "ncs", "rex", "rpx", "sentral", "star", "wahana", "dse",
];
const ALLOWED_GROUPS = new Set(["regular", "next_day", "economy", "cargo"]);
const DELIVERY_PATHS = Object.freeze({
  create: "order/api/v1/orders/store",
  detail: "order/api/v1/orders/detail",
  cancel: "order/api/v1/orders/cancel",
  label: "order/api/v1/orders/print-label",
  pickup: "order/api/v1/pickup/request",
});

function integer(value, fallback) {
  const parsed = Number.parseInt(value ?? "", 10);
  return Number.isInteger(parsed) && parsed > 0 ? parsed : fallback;
}

function normalizedBaseURL(value, fallback) {
  const parsed = new URL(String(value || fallback));
  if (!new Set(["https:", "http:"]).has(parsed.protocol)) {
    throw new Error("Base URL upstream wajib HTTP atau HTTPS.");
  }
  parsed.pathname = `${parsed.pathname.replace(/\/+$/, "")}/`;
  parsed.search = "";
  parsed.hash = "";
  return parsed;
}

function requestID(request) {
  const value = String(request.headers["x-request-id"] || "").trim();
  return /^[A-Za-z0-9._:-]{1,128}$/.test(value) ? value : randomUUID();
}

function reply(response, status, payload, id) {
  response.writeHead(status, {
    "Content-Type": "application/json; charset=utf-8",
    "Cache-Control": "no-store",
    "X-Content-Type-Options": "nosniff",
    "X-Request-Id": id,
  });
  response.end(JSON.stringify(payload));
}

function fail(response, status, code, message, id, details = null) {
  reply(response, status, { error: { code, message, details, request_id: id } }, id);
}

async function readJSON(request) {
  let size = 0;
  const chunks = [];
  for await (const chunk of request) {
    size += chunk.length;
    if (size > MAX_BODY_BYTES) throw new Error("BODY_TOO_LARGE");
    chunks.push(chunk);
  }
  const raw = Buffer.concat(chunks).toString("utf8").trim();
  return raw ? JSON.parse(raw) : {};
}

function header(request, name) {
  const value = request.headers[name.toLowerCase()];
  return String(Array.isArray(value) ? value[0] || "" : value || "").trim();
}

function uniqueStrings(value, fallback, maximum = 20) {
  if (!Array.isArray(value) || value.length === 0) return [...fallback];
  return [...new Set(value.map((item) => String(item).trim().toLowerCase()).filter(Boolean))]
    .slice(0, maximum);
}

export function serviceGroup(code, description = "") {
  const value = `${code} ${description}`.toUpperCase();
  if (/(JTR|TRUCK|TRUCKING|CARGO|KARGO|DARAT|LAUT|UDARA|REX10)/.test(value)) return "cargo";
  if (/(OKE|ECO|EKONOMI|ECONOMY|HEMAT|SAVE)/.test(value)) return "economy";
  if (/(YES|ONS|NEXT|NEXTDAY|OVERNIGHT|SDS|SAME DAY|SUPER SPEED)/.test(value)) return "next_day";
  return "regular";
}

function mapUpstreamStatus(status) {
  if (status === 400) return 400;
  if (status === 401 || status === 403) return 401;
  if (status === 404) return 404;
  if (status === 409) return 409;
  if (status === 429) return 429;
  if (status === 422) return 422;
  return status >= 500 ? 502 : 422;
}

function upstreamMessage(payload, fallback) {
  return String(payload?.meta?.message || payload?.message || payload?.data?.errors || fallback);
}

async function upstreamRequest(baseURL, path, apiKey, options = {}) {
  const target = new URL(path, baseURL);
  for (const [name, value] of Object.entries(options.query || {})) {
    if (value !== undefined && value !== null && String(value).trim() !== "") {
      target.searchParams.set(name, String(value));
    }
  }
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), UPSTREAM_TIMEOUT_MS);
  try {
    const headers = {
      Accept: "application/json",
      "User-Agent": `emisell-rajaongkir-hosted/${VERSION}`,
      [options.authHeader || "key"]: apiKey,
    };
    let body;
    if (options.form) {
      headers["Content-Type"] = "application/x-www-form-urlencoded";
      body = new URLSearchParams(options.form);
    } else if (options.json !== undefined) {
      headers["Content-Type"] = "application/json";
      body = JSON.stringify(options.json);
    }
    const response = await fetch(target, {
      method: options.method || "GET",
      headers,
      body,
      signal: controller.signal,
      redirect: "error",
    });
    const raw = await response.text();
    let payload = {};
    try {
      payload = raw ? JSON.parse(raw) : {};
    } catch {
      payload = { message: "Respons upstream bukan JSON." };
    }
    return { status: response.status, payload };
  } finally {
    clearTimeout(timeout);
  }
}

function requireKey(request, response, id, name, code) {
  const value = header(request, name);
  if (!value) {
    fail(response, 401, code, `Header ${name} RajaOngkir wajib diisi.`, id);
    return "";
  }
  return value;
}

async function rates(request, response, id, apiKey) {
  const input = await readJSON(request);
  const origin = String(input?.origin?.district_id || "").trim();
  const destination = String(input?.destination?.district_id || "").trim();
  const weight = Number.parseInt(input?.weight_grams, 10);
  const couriers = uniqueStrings(input?.courier_codes, DEFAULT_COURIERS);
  const groups = new Set(uniqueStrings(input?.service_groups, ALLOWED_GROUPS));
  if (!origin || !destination || !Number.isInteger(weight) || weight < 1 || weight > 1_000_000) {
    fail(response, 422, "INVALID_RATE_REQUEST", "Origin, destination, atau berat tidak valid.", id);
    return;
  }
  const upstream = await upstreamRequest(COST_BASE_URL, "calculate/district/domestic-cost", apiKey, {
    method: "POST",
    form: {
      origin,
      destination,
      weight: String(weight),
      courier: couriers.join(":"),
      price: input?.price === "highest" ? "highest" : "lowest",
    },
  });
  if (upstream.status < 200 || upstream.status >= 300) {
    fail(response, mapUpstreamStatus(upstream.status), "RAJAONGKIR_RATE_ERROR", upstreamMessage(upstream.payload, "Tarif tidak tersedia."), id);
    return;
  }
  const quotes = (Array.isArray(upstream.payload?.data) ? upstream.payload.data : [])
    .map((item) => {
      const group = serviceGroup(item.service, item.description);
      return {
        provider_code: PROVIDER_CODE,
        courier_code: String(item.code || "").trim().toLowerCase(),
        courier_name: String(item.name || item.code || "").trim(),
        service_code: String(item.service || "").trim(),
        service_name: String(item.description || item.service || "").trim(),
        service_group: group,
        price: Number(item.cost || 0),
        currency: "IDR",
        etd: String(item.etd || "").trim(),
      };
    })
    .filter((item) => item.courier_code && item.service_code && item.price >= 0 && groups.has(item.service_group));
  reply(response, 200, { data: { quotes }, meta: sourceMeta("shipping_cost") }, id);
}

async function tracking(request, response, id, apiKey) {
  const input = await readJSON(request);
  const waybill = String(input?.waybill_number || "").trim();
  const courier = String(input?.courier_code || "").trim().toLowerCase();
  if (!/^[A-Za-z0-9-]{6,64}$/.test(waybill) || !/^[a-z0-9_-]{2,32}$/.test(courier)) {
    fail(response, 422, "INVALID_TRACKING_REQUEST", "Nomor resi atau kode kurir tidak valid.", id);
    return;
  }
  const query = { awb: waybill, courier };
  if (input?.last_phone_number) query.last_phone_number = String(input.last_phone_number).trim();
  const upstream = await upstreamRequest(COST_BASE_URL, "track/waybill", apiKey, { method: "POST", query });
  if (upstream.status < 200 || upstream.status >= 300 || !upstream.payload?.data) {
    fail(response, mapUpstreamStatus(upstream.status), "RAJAONGKIR_TRACKING_ERROR", upstreamMessage(upstream.payload, "Resi tidak ditemukan."), id);
    return;
  }
  const data = upstream.payload.data;
  const summary = data.summary || {};
  const history = (Array.isArray(data.manifest) ? data.manifest : []).map((item) => ({
    code: String(item.manifest_code || "").trim(),
    description: String(item.manifest_description || "").trim(),
    event_at: `${String(item.manifest_date || "").trim()} ${String(item.manifest_time || "").trim()}`.trim(),
    location: String(item.city_name || "").trim(),
  }));
  reply(response, 200, {
    data: {
      waybill_number: String(summary.waybill_number || waybill).trim(),
      courier_code: String(summary.courier_code || courier).trim().toLowerCase(),
      courier_name: String(summary.courier_name || courier).trim(),
      service_code: String(summary.service_code || "").trim(),
      status: String(summary.status || "unknown").trim().toLowerCase(),
      delivered: Boolean(data.delivered),
      origin: String(summary.origin || "").trim(),
      destination: String(summary.destination || "").trim(),
      history,
    },
    meta: sourceMeta("shipping_cost"),
  }, id);
}

function sourceMeta(product) {
  return { provider_code: PROVIDER_CODE, upstream: "rajaongkir", product };
}

function providerPayload(input) {
  const payload = input?.provider_payload;
  return payload && typeof payload === "object" && !Array.isArray(payload) ? payload : null;
}

function shipmentProviderPayload(input) {
  const passthrough = providerPayload(input);
  if (passthrough) return passthrough;
  const shipment = input && typeof input === "object" ? input : {};
  const sender = shipment.sender || {};
  const recipient = shipment.recipient || {};
  const parcel = shipment.package || {};
  const payment = shipment.payment || {};
  const items = Array.isArray(parcel.items) ? parcel.items : [];
  if (!shipment.merchant_reference || !sender.name || !recipient.name || !shipment.courier_code || !shipment.service_code) {
    return null;
  }
  const paymentMethod = String(payment.type || "").toLowerCase() === "cod" ? "COD" : "BANK TRANSFER";
  const address = (value, note) => note ? `${value} (${note})` : value;
  const point = (latitude, longitude) => Number.isFinite(latitude) && Number.isFinite(longitude)
    ? `${latitude.toFixed(7)}, ${longitude.toFixed(7)}` : "";
  const payload = {
    order_date: new Intl.DateTimeFormat("en-CA", { timeZone: "Asia/Jakarta" }).format(new Date()),
    brand_name: shipment.brand_name || "Emisell",
    shipper_name: sender.name,
    shipper_phone: sender.phone,
    shipper_email: sender.email || "",
    shipper_destination_id: Number(sender.destination_id || 0),
    shipper_address: address(sender.address || "", sender.address_note || ""),
    receiver_name: recipient.name,
    receiver_phone: recipient.phone,
    receiver_email: recipient.email || "",
    receiver_destination_id: Number(recipient.destination_id || 0),
    receiver_address: address(recipient.address || "", recipient.address_note || ""),
    shipping: String(shipment.courier_code).toUpperCase(),
    shipping_type: shipment.service_code,
    payment_method: paymentMethod,
    shipping_cost: Number(payment.shipping_cost || 0),
    shipping_cashback: Number(payment.shipping_cashback || 0),
    service_fee: Number(payment.service_fee || 0),
    additional_cost: Number(payment.additional_cost || 0),
    grand_total: Number(payment.grand_total || 0),
    cod_value: Number(payment.cod_value || 0),
    insurance_value: Number(payment.insurance_value || 0),
    notes: shipment.notes || "",
    order_details: items.map((item) => ({
      product_name: item.name,
      product_variant_name: item.variant || "",
      product_price: Number(item.unit_value || 0),
      product_weight: Number(item.weight_grams || 0),
      product_width: Number(item.width_cm || parcel.width_cm || 0),
      product_height: Number(item.height_cm || parcel.height_cm || 0),
      product_length: Number(item.length_cm || parcel.length_cm || 0),
      qty: Number(item.quantity || 0),
      subtotal: Number(item.unit_value || 0) * Number(item.quantity || 0),
    })),
  };
  const originPoint = point(sender.latitude, sender.longitude);
  const destinationPoint = point(recipient.latitude, recipient.longitude);
  if (originPoint) payload.origin_pin_point = originPoint;
  if (destinationPoint) payload.destination_pin_point = destinationPoint;
  return payload;
}

function pickupProviderPayload(input) {
  const passthrough = providerPayload(input);
  if (passthrough) return passthrough;
  const scheduledAt = new Date(input?.scheduled_at || "");
  const shipmentID = String(input?.provider_shipment_id || "").trim();
  if (!shipmentID || Number.isNaN(scheduledAt.getTime())) return null;
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone: "Asia/Jakarta", year: "numeric", month: "2-digit", day: "2-digit",
    hour: "2-digit", minute: "2-digit", second: "2-digit", hourCycle: "h23",
  }).formatToParts(scheduledAt);
  const value = (type) => parts.find((part) => part.type === type)?.value || "";
  const vehicle = String(input?.vehicle || "motor").trim().toLowerCase();
  const vehicleLabel = vehicle === "mobil" ? "Mobil" : vehicle === "truk" ? "Truk" : "Motor";
  return {
    pickup_date: `${value("year")}-${value("month")}-${value("day")}`,
    pickup_time: `${value("hour")}:${value("minute")}:${value("second")}`,
    pickup_vehicle: vehicleLabel,
    orders: [{ order_no: shipmentID }],
  };
}

async function createShipment(request, response, id, apiKey) {
  const payload = shipmentProviderPayload(await readJSON(request));
  if (!payload) {
    fail(response, 422, "INVALID_SHIPMENT_REQUEST", "Data shipment canonical belum lengkap.", id);
    return;
  }
  const upstream = await upstreamRequest(DELIVERY_BASE_URL, DELIVERY_PATHS.create, apiKey, {
    method: "POST", authHeader: "x-api-key", json: payload,
  });
  if (upstream.status < 200 || upstream.status >= 300) {
    fail(response, mapUpstreamStatus(upstream.status), "RAJAONGKIR_SHIPMENT_ERROR", upstreamMessage(upstream.payload, "Order gagal dibuat."), id);
    return;
  }
  const data = upstream.payload?.data || {};
  reply(response, 201, {
    data: {
      partner_shipment_id: String(data.order_no || data.order_id || "").trim(),
      provider_shipment_id: String(data.order_id || "").trim(),
      waybill_number: String(data.awb || "").trim(),
      status: "created",
      provider_payload: data,
    },
    meta: sourceMeta("shipping_delivery"),
  }, id);
}

async function shipmentDetail(response, id, apiKey, shipmentID) {
  const upstream = await upstreamRequest(DELIVERY_BASE_URL, DELIVERY_PATHS.detail, apiKey, {
    method: "GET", authHeader: "x-api-key", query: { order_no: shipmentID },
  });
  if (upstream.status < 200 || upstream.status >= 300) {
    fail(response, mapUpstreamStatus(upstream.status), "RAJAONGKIR_SHIPMENT_NOT_FOUND", upstreamMessage(upstream.payload, "Order tidak ditemukan."), id);
    return;
  }
  const data = upstream.payload?.data || {};
  reply(response, 200, {
    data: {
      partner_shipment_id: String(data.order_no || shipmentID),
      provider_shipment_id: String(data.order_id || ""),
      waybill_number: String(data.awb || ""),
      status: String(data.order_status || "unknown").toLowerCase(),
      courier_code: String(data.shipping || "").toLowerCase(),
      service_code: String(data.shipping_type || ""),
      live_tracking_url: String(data.live_tracking_url || ""),
      provider_payload: data,
    },
    meta: sourceMeta("shipping_delivery"),
  }, id);
}

async function cancelShipment(response, id, apiKey, shipmentID) {
  const upstream = await upstreamRequest(DELIVERY_BASE_URL, DELIVERY_PATHS.cancel, apiKey, {
    method: "PUT", authHeader: "x-api-key", json: { order_no: shipmentID },
  });
  if (upstream.status < 200 || upstream.status >= 300) {
    fail(response, mapUpstreamStatus(upstream.status), "RAJAONGKIR_CANCEL_ERROR", upstreamMessage(upstream.payload, "Order tidak dapat dibatalkan."), id);
    return;
  }
  reply(response, 200, {
    data: { partner_shipment_id: shipmentID, status: "cancelled" },
    meta: sourceMeta("shipping_delivery"),
  }, id);
}

async function shipmentLabel(response, id, apiKey, shipmentID, format) {
  const pages = new Set(["page_1", "page_2", "page_4", "page_5", "page_6"]);
  const page = pages.has(format) ? format : "page_5";
  const upstream = await upstreamRequest(DELIVERY_BASE_URL, DELIVERY_PATHS.label, apiKey, {
    method: "POST", authHeader: "x-api-key", query: { page, order_no: shipmentID },
  });
  if (upstream.status < 200 || upstream.status >= 300) {
    fail(response, mapUpstreamStatus(upstream.status), "RAJAONGKIR_LABEL_ERROR", upstreamMessage(upstream.payload, "Label tidak tersedia."), id);
    return;
  }
  const data = upstream.payload?.data || {};
  reply(response, 200, {
    data: {
      partner_shipment_id: shipmentID,
      format: page,
      content_type: "application/pdf",
      file_url: String(data.path || ""),
      base64: String(data.base_64 || ""),
    },
    meta: sourceMeta("shipping_delivery"),
  }, id);
}

async function pickup(request, response, id, apiKey) {
  const payload = pickupProviderPayload(await readJSON(request));
  if (!payload) {
    fail(response, 422, "INVALID_PICKUP_REQUEST", "Jadwal pickup atau provider_shipment_id tidak valid.", id);
    return;
  }
  const upstream = await upstreamRequest(DELIVERY_BASE_URL, DELIVERY_PATHS.pickup, apiKey, {
    method: "POST", authHeader: "x-api-key", json: payload,
  });
  if (upstream.status < 200 || upstream.status >= 300) {
    fail(response, mapUpstreamStatus(upstream.status), "RAJAONGKIR_PICKUP_ERROR", upstreamMessage(upstream.payload, "Pickup gagal dibuat."), id);
    return;
  }
  const data = upstream.payload?.data ?? upstream.payload;
  reply(response, 201, {
    data: {
      pickup_id: String(data?.pickup_no || data?.pickup_id || "").trim(),
      status: "requested",
      provider_payload: data,
    },
    meta: sourceMeta("shipping_delivery"),
  }, id);
}

function shipmentRoute(pathname) {
  const match = pathname.match(/^\/partner\/v1\/shipments\/([A-Za-z0-9._~-]{1,128})(?:\/(cancel|label))?$/);
  return match ? { shipmentID: decodeURIComponent(match[1]), action: match[2] || "detail" } : null;
}

export function createConnectorServer() {
  return createServer(async (request, response) => {
    const id = requestID(request);
    const url = new URL(request.url || "/", "http://connector.local");
    try {
      if (request.method === "GET" && url.pathname === "/partner/v1/health") {
        reply(response, 200, { status: "ok", version: VERSION, upstream: "rajaongkir" }, id);
        return;
      }
      if (request.method === "GET" && url.pathname === "/partner/v1/capabilities") {
        reply(response, 200, {
          provider_code: PROVIDER_CODE,
          provider_name: PROVIDER_NAME,
          contract_version: "v1",
          capabilities: { rates: true, shipments: true, pickup: true, tracking: true },
        }, id);
        return;
      }
      if (request.method === "GET" && url.pathname === "/partner/v1/services") {
        reply(response, 200, {
          data: [...ALLOWED_GROUPS].map((group) => ({ group, enabled: true })),
        }, id);
        return;
      }
      if (request.method === "POST" && url.pathname === "/partner/v1/rates") {
        const key = requireKey(request, response, id, "key", "RAJAONGKIR_COST_KEY_REQUIRED");
        if (key) await rates(request, response, id, key);
        return;
      }
      if (request.method === "POST" && url.pathname === "/partner/v1/tracking/waybills") {
        const key = requireKey(request, response, id, "key", "RAJAONGKIR_COST_KEY_REQUIRED");
        if (key) await tracking(request, response, id, key);
        return;
      }
      if (request.method === "POST" && url.pathname === "/partner/v1/shipments") {
        const key = requireKey(request, response, id, "x-api-key", "RAJAONGKIR_DELIVERY_KEY_REQUIRED");
        if (key) await createShipment(request, response, id, key);
        return;
      }
      if (request.method === "POST" && url.pathname === "/partner/v1/pickups") {
        const key = requireKey(request, response, id, "x-api-key", "RAJAONGKIR_DELIVERY_KEY_REQUIRED");
        if (key) await pickup(request, response, id, key);
        return;
      }
      const dynamic = shipmentRoute(url.pathname);
      if (dynamic) {
        const key = requireKey(request, response, id, "x-api-key", "RAJAONGKIR_DELIVERY_KEY_REQUIRED");
        if (!key) return;
        if (request.method === "GET" && dynamic.action === "detail") {
          await shipmentDetail(response, id, key, dynamic.shipmentID);
          return;
        }
        if (request.method === "POST" && dynamic.action === "cancel") {
          await cancelShipment(response, id, key, dynamic.shipmentID);
          return;
        }
        if (request.method === "GET" && dynamic.action === "label") {
          await shipmentLabel(response, id, key, dynamic.shipmentID, url.searchParams.get("format"));
          return;
        }
      }
      fail(response, 404, "ENDPOINT_NOT_FOUND", "Endpoint connector tidak ditemukan.", id);
    } catch (error) {
      if (error?.message === "BODY_TOO_LARGE") {
        fail(response, 413, "BODY_TOO_LARGE", "Payload melebihi 64 KB.", id);
        return;
      }
      if (error instanceof SyntaxError) {
        fail(response, 400, "INVALID_JSON", "Payload JSON tidak valid.", id);
        return;
      }
      const timeout = error?.name === "AbortError";
      fail(
        response,
        timeout ? 504 : 502,
        timeout ? "UPSTREAM_TIMEOUT" : "UPSTREAM_UNAVAILABLE",
        timeout ? "RajaOngkir melewati batas waktu." : "RajaOngkir tidak dapat dihubungi.",
        id,
      );
    }
  });
}

const isMain = process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href;
if (isMain) {
  createConnectorServer().listen(PORT, "0.0.0.0", () => {
    console.log(JSON.stringify({ event: "connector_started", provider: PROVIDER_CODE, port: PORT, version: VERSION }));
  });
}
