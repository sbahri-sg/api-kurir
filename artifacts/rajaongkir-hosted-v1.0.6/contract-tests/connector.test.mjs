import test from "node:test";
import assert from "node:assert/strict";
import {
  canonicalCourierName,
  canonicalServiceName,
  createConnectorServer,
  deliveryBaseURL,
  executionMode,
  pickupProviderResult,
  PROVIDER_CODE,
  serviceGroup,
} from "../src/server.mjs";

async function withServer(run) {
  const server = createConnectorServer();
  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
  const address = server.address();
  try {
    await run(`http://127.0.0.1:${address.port}/partner/v1`);
  } finally {
    await new Promise((resolve) => server.close(resolve));
  }
}

test("service group hanya menghasilkan empat group checkout", () => {
  assert.equal(serviceGroup("REG", "Regular"), "regular");
  assert.equal(serviceGroup("OKE", "Ekonomi"), "economy");
  assert.equal(serviceGroup("YES", "Yakin Esok Sampai"), "next_day");
  assert.equal(serviceGroup("JTR", "Trucking"), "cargo");
});

test("label rate dinormalisasi untuk checkout Emisell", () => {
  assert.equal(canonicalCourierName("jne", "Jalur Nugraha Ekakurir (JNE)"), "JNE");
  assert.equal(canonicalServiceName("jne", "REG", "Layanan Reguler"), "Regular");
  assert.equal(canonicalServiceName("jne", "JTR<130", "JNE Trucking"), "Trucking");
  assert.equal(canonicalCourierName("jnt", "J&T Express"), "J&T");
  assert.equal(canonicalServiceName("jnt", "EZ", "J&T EZ"), "EZ");
  assert.equal(canonicalServiceName("anteraja", "DOK", "Anteraja Document"), "Document");
  assert.equal(canonicalCourierName("wahana", "Wahana Express"), "Wahana");
  assert.equal(canonicalServiceName("wahana", "Normal", "Wahana Express"), "Regular");
});

test("kode kurir tracking memakai kode canonical request", () => {
  assert.equal(canonicalCourierName("jnt", "J&T Express"), "J&T");
});

test("Shipping Delivery memilih base URL berdasarkan execution mode", () => {
  const sandboxRequest = { headers: { "x-emisell-execution-mode": "sandbox" } };
  const liveRequest = { headers: { "x-emisell-execution-mode": "live" } };

  assert.equal(executionMode(sandboxRequest), "sandbox");
  assert.equal(deliveryBaseURL(sandboxRequest).origin, "https://api-sandbox.collaborator.komerce.id");
  assert.equal(executionMode(liveRequest), "live");
  assert.equal(deliveryBaseURL(liveRequest).origin, "https://api.collaborator.komerce.id");
});

test("pickup menolak item gagal walaupun envelope upstream sukses", () => {
  const result = pickupProviderResult({
    meta: { code: 201, status: "success" },
    data: [{ order_no: "KOM-FAILED", status: "failed", awb: "" }],
  }, "KOM-FAILED");

  assert.equal(result.ok, false);
  assert.equal(result.orderNo, "KOM-FAILED");
  assert.equal(result.providerStatus, "failed");
});

test("pickup sukses memetakan order dan AWB", () => {
  const result = pickupProviderResult({
    data: [{ order_no: "KOM-SUCCESS", status: "success", awb: "JNE1234567890" }],
  }, "KOM-SUCCESS");

  assert.equal(result.ok, true);
  assert.equal(result.orderNo, "KOM-SUCCESS");
  assert.equal(result.waybillNumber, "JNE1234567890");
});

test("health, capabilities, dan services dapat dibaca tanpa credential upstream", async () => {
  await withServer(async (baseURL) => {
    const health = await fetch(`${baseURL}/health`);
    assert.equal(health.status, 200);
    assert.equal((await health.json()).status, "ok");

    const capabilities = await fetch(`${baseURL}/capabilities`);
    assert.equal(capabilities.status, 200);
    const capabilityPayload = await capabilities.json();
    assert.equal(capabilityPayload.provider_code, PROVIDER_CODE);
    assert.equal(capabilityPayload.capabilities.rates, true);

    const services = await fetch(`${baseURL}/services`);
    assert.equal(services.status, 200);
    assert.deepEqual(
      (await services.json()).data.map((item) => item.group),
      ["regular", "next_day", "economy", "cargo"],
    );
  });
});

test("rate dan shipment menolak request tanpa credential yang sesuai", async () => {
  await withServer(async (baseURL) => {
    const rate = await fetch(`${baseURL}/rates`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({}),
    });
    assert.equal(rate.status, 401);
    assert.equal((await rate.json()).error.code, "RAJAONGKIR_COST_KEY_REQUIRED");

    const shipment = await fetch(`${baseURL}/shipments`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({}),
    });
    assert.equal(shipment.status, 401);
    assert.equal((await shipment.json()).error.code, "RAJAONGKIR_DELIVERY_KEY_REQUIRED");
  });
});
