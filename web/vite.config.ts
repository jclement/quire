// Vite config: React SPA with Tailwind v4. In dev, /api and /mcp proxy to the
// Go server (see scripts/dev.sh); in production Go serves the built assets.
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import { defineConfig, type Plugin } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

const FONT_ROOT = "node_modules/@excalidraw/excalidraw/dist/prod/fonts";
// Xiaolai is a 12MB CJK face; every other family together is under 500KB.
// CJK text in a drawing falls back to a system font instead.
const SKIPPED_FONT_FAMILIES = new Set(["Xiaolai"]);

/** Every font file to ship, relative to FONT_ROOT. */
function fontFiles(): string[] {
  const out: string[] = [];
  const walk = (dir: string) => {
    for (const entry of readdirSync(dir)) {
      const full = join(dir, entry);
      if (statSync(full).isDirectory()) {
        if (!SKIPPED_FONT_FAMILIES.has(entry)) walk(full);
      } else {
        out.push(relative(FONT_ROOT, full));
      }
    }
  };
  if (existsSync(FONT_ROOT)) walk(FONT_ROOT);
  return out;
}

/**
 * Self-hosts Excalidraw's fonts under /excalidraw/fonts/ — served from
 * node_modules in dev, emitted into the bundle for production — so the
 * drawing editor never reaches for its CDN fallback (which the CSP blocks
 * anyway). DrawingDialog points EXCALIDRAW_ASSET_PATH here.
 */
function excalidrawFonts(): Plugin {
  const prefix = "/excalidraw/fonts/";
  return {
    name: "quire-excalidraw-fonts",
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        if (!req.url?.startsWith(prefix)) return next();
        const rel = decodeURIComponent(req.url.slice(prefix.length).split("?")[0] ?? "");
        const file = join(FONT_ROOT, rel);
        if (rel.includes("..") || !existsSync(file)) {
          res.statusCode = 404;
          res.end();
          return;
        }
        res.setHeader("Content-Type", "font/woff2");
        res.end(readFileSync(file));
      });
    },
    generateBundle() {
      for (const rel of fontFiles()) {
        this.emitFile({
          type: "asset",
          fileName: `excalidraw/fonts/${rel}`,
          source: readFileSync(join(FONT_ROOT, rel)),
        });
      }
    },
  };
}

export default defineConfig({
  plugins: [react(), tailwindcss(), excalidrawFonts()],
  build: {
    // The Go binary embeds the built SPA from internal/webui/dist
    // (webui_prod.go's go:embed), so the production build lands there.
    outDir: "../internal/webui/dist",
    emptyOutDir: true,
  },
  server: {
    proxy: {
      "/api": "http://127.0.0.1:8321",
      "/mcp": "http://127.0.0.1:8321",
    },
  },
});
