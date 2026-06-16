import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    // 允许 ONLYOFFICE DS 容器经 host.docker.internal 拉取桥插件（dev 用 --host 0.0.0.0；见 .env ONLYOFFICE_PLUGIN_URL）。
    allowedHosts: ["host.docker.internal"],
    proxy: {
      "/api": "http://localhost:3001",
      "/health": "http://localhost:3001",
    },
  },
});
