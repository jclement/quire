// End-to-end tests against a real quire binary serving the real embedded
// SPA — not a dev server, so what is tested is what ships.
//
// Two instances, because they need incompatible configurations:
//   app  — auth off, so every spec is one navigation from the thing it tests;
//   auth — passkey mode on a non-loopback address, the only way to exercise
//          the WebAuthn ceremonies and the bootstrap enrollment gate.
import { defineConfig, devices } from "@playwright/test";

// Ports unlikely to collide with `mise run dev` (:8321) or Vite (:5173).
const PORT = Number(process.env.QUIRE_E2E_PORT ?? 8351);
// A second instance in passkey mode, for the auth ceremonies. They need a
// real WebAuthn authenticator (Chromium's virtual one) and a server that
// actually demands a credential, which the auth-none instance cannot be.
const AUTH_PORT = PORT + 1;

/** Every QUIRE_* a developer's mise.local.toml might set, neutralised.
 *  Playwright merges process.env, and a test suite must not be one leaked
 *  variable away from writing to someone's real notes or emailing them. */
const CLEAN_ENV = {
  QUIRE_GIT: "false",
  QUIRE_UPDATE_CHECK: "false",
  QUIRE_SMTP_HOST: "",
  QUIRE_SMTP_PORT: "",
  QUIRE_SMTP_USER: "",
  QUIRE_SMTP_PASS: "",
  QUIRE_SMTP_FROM: "",
  QUIRE_DIGEST_TO: "",
  QUIRE_DIGEST_TIME: "",
  QUIRE_TRUSTED_PROXIES: "",
  QUIRE_LOG_LEVEL: "warn",
};

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

  projects: [
    {
      name: "app",
      testIgnore: /auth\.spec\.ts/,
      use: { ...devices["Desktop Chrome"], baseURL: `http://127.0.0.1:${PORT}` },
    },
    {
      name: "auth",
      testMatch: /auth\.spec\.ts/,
      use: {
        ...devices["Desktop Chrome"],
        // localhost, not 127.0.0.1: WebAuthn's relying-party ID must be a
        // domain, and the browser treats localhost as a secure context, so
        // passkeys work here without TLS.
        baseURL: `http://localhost:${AUTH_PORT}`,
      },
    },
  ],

  // Built and started by `mise run test:e2e`, which compiles the binary with
  // the frontend embedded first, so this only has to run it.
  webServer: [
    {
      command: `../tmp/quire-e2e serve`,
      url: `http://127.0.0.1:${PORT}/api/v1/health`,
      reuseExistingServer: false,
      stdout: "pipe",
      stderr: "pipe",
      env: {
        ...CLEAN_ENV,
        QUIRE_DATA_DIR: "../tmp/e2e-data",
        QUIRE_ADDR: `127.0.0.1:${PORT}`,
        QUIRE_BASE_URL: `http://127.0.0.1:${PORT}`,
        QUIRE_AUTH_MODE: "none",
      },
    },
    {
      // Bound to 0.0.0.0 on purpose: the bootstrap enrollment gate is
      // skipped on a loopback listener (nothing remote can race it), and
      // that gate is one of the things worth testing. Its output is teed to
      // a file because claiming the instance needs the code it prints.
      command: `../tmp/quire-e2e serve > ../tmp/e2e-auth.log 2>&1`,
      url: `http://localhost:${AUTH_PORT}/api/v1/health`,
      reuseExistingServer: false,
      env: {
        ...CLEAN_ENV,
        QUIRE_DATA_DIR: "../tmp/e2e-auth-data",
        QUIRE_ADDR: `0.0.0.0:${AUTH_PORT}`,
        QUIRE_BASE_URL: `http://localhost:${AUTH_PORT}`,
        QUIRE_AUTH_MODE: "passkey",
        QUIRE_LOG_LEVEL: "info",
      },
    },
  ],
});
