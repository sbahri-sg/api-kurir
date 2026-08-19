import { readFileSync } from "node:fs";
import type { IncomingMessage, ServerResponse } from "node:http";
import { fileURLToPath } from "node:url";
import { defineConfig, loadEnv, type Plugin } from "vite";
import react from "@vitejs/plugin-react";

const openApiAssets = {
  "/openapi/api-kurir-public-v1.yaml": readFileSync(
    fileURLToPath(new URL("../../openapi/public.yaml", import.meta.url)),
    "utf8",
  ),
  "/openapi/api-kurir-partner-v1.yaml": readFileSync(
    fileURLToPath(new URL("../../openapi/partner-v1.yaml", import.meta.url)),
    "utf8",
  ),
};

function serveOpenApiAsset(
  request: IncomingMessage,
  response: ServerResponse,
  next: () => void,
) {
  const path = request.url?.split("?", 1)[0] ?? "";
  const source = openApiAssets[path as keyof typeof openApiAssets];
  if (!source) {
    next();
    return;
  }
  response.statusCode = 200;
  response.setHeader("Content-Type", "application/yaml; charset=utf-8");
  response.end(source);
}

function openApiAssetsPlugin(): Plugin {
  return {
    name: "api-kurir-openapi-assets",
    configureServer(server) {
      server.middlewares.use(serveOpenApiAsset);
    },
    configurePreviewServer(server) {
      server.middlewares.use(serveOpenApiAsset);
    },
    generateBundle() {
      for (const [path, source] of Object.entries(openApiAssets)) {
        this.emitFile({
          type: "asset",
          fileName: path.slice(1),
          source,
        });
      }
    },
  };
}

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, ".", "");
  const apiTarget =
    env.VITE_DEV_API_TARGET?.trim() || "http://127.0.0.1:8080";
  const proxyOptions = {
    target: apiTarget,
    changeOrigin: true,
  };
  const apiProxy = {
    "/v1": {
      ...proxyOptions,
    },
    "/api/v1": {
      ...proxyOptions,
    },
  };

  return {
    plugins: [react(), openApiAssetsPlugin()],
    server: {
      host: "0.0.0.0",
      port: 5173,
      strictPort: true,
      proxy: apiProxy,
    },
    preview: {
      host: "0.0.0.0",
      port: 4173,
      strictPort: true,
      proxy: apiProxy,
    },
  };
});
