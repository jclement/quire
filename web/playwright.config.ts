// End-to-end tests against a real quire binary serving the real embedded
// SPA — not a dev server, so what is tested is what ships.
//
// Auth is off (loopback-only mode "none"), deliberately: these tests are
// about the app, and the auth gating is covered thoroughly by unit tests in
// internal/auth. Running them signed-out keeps every spec one navigation
// from the thing it is testing.
import { defineConfig, devices } from "@playwright/test";

// A port unlikely to collide with `mise run dev` (:8321) or Vite (:5173).
const PORT = Number(process.env.QUIRE_E2E_PORT ?? 8351);

export default defineConfig({
  testDir: "./e2e",
  // The suite drives one server with real state on disk, so tests must not
  // race each other through it.
  workers: 1,
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? "github" : "list",
  timeout: 30_000,
  expect: { timeout: 10_000 },

  use: {
    baseURL: `http://127.0.0.1:${PORT}`,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },

  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],

  // Built and started by `mise run test:e2e`, which compiles the binary with
  // the frontend embedded first, so this only has to run it.
  webServer: {
    command: `../tmp/quire-e2e serve`,
    url: `http://127.0.0.1:${PORT}/api/v1/health`,
    reuseExistingServer: false,
    stdout: "pipe",
    stderr: "pipe",
    env: {
      QUIRE_DATA_DIR: "../tmp/e2e-data",
      QUIRE_ADDR: `127.0.0.1:${PORT}`,
      QUIRE_BASE_URL: `http://127.0.0.1:${PORT}`,
      QUIRE_AUTH_MODE: "none",
      QUIRE_GIT: "false",
      QUIRE_UPDATE_CHECK: "false",
      // A developer's mise.local.toml points QUIRE_* at their real vault and
      // a real digest recipient. Playwright merges process.env, so every one
      // of those has to be neutralised here by name — a test suite must not
      // be one leaked variable away from writing to someone's notes or
      // emailing them.
      QUIRE_SMTP_HOST: "",
      QUIRE_SMTP_PORT: "",
      QUIRE_SMTP_USER: "",
      QUIRE_SMTP_PASS: "",
      QUIRE_SMTP_FROM: "",
      QUIRE_DIGEST_TO: "",
      QUIRE_DIGEST_TIME: "",
      QUIRE_TRUSTED_PROXIES: "",
      QUIRE_LOG_LEVEL: "warn",
    },
  },
});
