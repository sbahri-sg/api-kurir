import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, ".", "");
  const apiTarget =
    env.VITE_DEV_API_TARGET?.trim() || "http://127.0.0.1:8080";
  const apiProxy = {
    "/v1": {
      target: apiTarget,
      changeOrigin: true,
    },
  };

  return {
    plugins: [react()],
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
