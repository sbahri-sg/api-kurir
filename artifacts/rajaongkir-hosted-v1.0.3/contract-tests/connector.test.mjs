import test from "node:test";
import assert from "node:assert/strict";
import {
  createConnectorServer,
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
