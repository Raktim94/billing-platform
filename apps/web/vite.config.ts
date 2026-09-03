import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    // Proxies /api/* to apps/server in dev so the browser only ever talks
    // to one origin — same-origin means the session cookie's
    // SameSite=Lax/Strict setting (internal/platform/config, Stage 2)
    // just works, no CORS_ALLOWED_ORIGINS configuration needed for local
    // development. Override the target via VITE_API_PROXY_TARGET if
    // apps/server is running somewhere other than :8080 (e.g. the Stage
    // 10a docker-compose HTTP_PORT override).
    proxy: {
      "/api": {
        target: process.env.VITE_API_PROXY_TARGET ?? "http://localhost:8080",
        changeOrigin: true,
      },
    },
  },
});
