import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Vite dev server proxies REST + SSE to the Go backend so the React app can
// hit relative paths in both dev and production (embedded) modes.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: "http://localhost:8080",
        changeOrigin: true,
      },
      "/sse": {
        target: "http://localhost:8080",
        changeOrigin: true,
        ws: false,
      },
      "/healthz": "http://localhost:8080",
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
});
