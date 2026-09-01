// Vite config: React SPA with Tailwind v4. In dev, /api and /mcp proxy to the
// Go server (see scripts/dev.sh); in production Go serves the built assets.
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    proxy: {
      "/api": "http://127.0.0.1:8321",
      "/mcp": "http://127.0.0.1:8321",
    },
  },
});
