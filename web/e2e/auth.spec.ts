// The passkey ceremonies, against a real quire in passkey mode using
// Chromium's virtual WebAuthn authenticator.
//
// This is the one part of the product no other test can reach: registration
// and login need an authenticator and a secure context, so unit tests can
// only check the gating around them. It needs no test-only bypass in the
// product — the virtual authenticator is a browser feature.
//
// The specs run in order against one instance: unclaimed, then claimed, then
// signed out and back in.
import { expect, test, type BrowserContext, type Page } from "@playwright/test";
import { readFileSync } from "node:fs";

test.describe.configure({ mode: "serial" });

// One context for the whole file. These specs are a single story — unclaimed,
// claimed, signed out, signed back in — and Playwright's default fresh
// context per test would throw away the session between chapters.
let context: BrowserContext;
let page: Page;
// Chrome allows exactly one internal authenticator per browser, so it is
// created once here and its credentials are cleared when a spec needs to
// look like a device that has never enrolled.
let cdp: Awaited<ReturnType<BrowserContext["newCDPSession"]>>;
let authenticatorId: string;

test.beforeAll(async ({ browser }) => {
  context = await browser.newContext();
  page = await context.newPage();
  ({ cdp, authenticatorId } = await addAuthenticator(page));
});

test.afterAll(async () => {
  await context.close();
});

/** Gives the page an authenticator that approves silently. */
async function addAuthenticator(page: Page) {
  const cdp = await page.context().newCDPSession(page);
  await cdp.send("WebAuthn.enable");
  const { authenticatorId } = await cdp.send("WebAuthn.addVirtualAuthenticator", {
    options: {
      protocol: "ctap2",
      transport: "internal",
      hasResidentKey: true,
      hasUserVerification: true,
      isUserVerified: true,
      automaticPresenceSimulation: true,
    },
  });
  return { cdp, authenticatorId };
}

/** The enrollment code the server printed at startup. */
function enrollmentCode(): string {
  const log = readFileSync("../tmp/e2e-auth.log", "utf8");
  const match = log.match(/enrollment_code=([A-Z2-7]+)/);
  if (!match) throw new Error("no enrollment code in the auth server's log");
  return match[1]!;
}

test("an unclaimed instance refuses to be claimed without the code", async () => {
  await page.goto("/");
  await expect(page.getByText("Claim this quire")).toBeVisible();

  await page.getByLabel("Enrollment code").fill("WRONGWRONGWRONG1");
  await page.getByLabel("Passkey name").fill("Impostor key");
  await page.getByRole("button", { name: "Create passkey" }).click();

  // The whole point of the gate: whoever finds the URL first must not get
  // the vault.
  await expect(page.getByText(/enrollment code/i).last()).toBeVisible();
  await expect(page.getByText("Claim this quire")).toBeVisible();
});

test("claiming with the code registers a passkey and shows recovery codes", async () => {
  await page.goto("/");

  await page.getByLabel("Enrollment code").fill(enrollmentCode());
  await page.getByLabel("Passkey name").fill("E2E key");
  await page.getByRole("button", { name: "Create passkey" }).click();

  // Recovery codes are shown exactly once; losing them can mean losing the
  // vault, so this screen is load-bearing.
  const codes = page.locator("li, code").filter({ hasText: /^[a-z2-7]{4}-[a-z2-7]{4}$/ });
  await expect(codes.first()).toBeVisible({ timeout: 15_000 });
  expect(await codes.count()).toBe(8);

  await page.getByRole("button", { name: /continue|done|saved|got it/i }).first().click();
  // Signed in: the app itself, not a login screen.
  await expect(page.getByRole("link", { name: "Settings" })).toBeVisible();
});

test("the API is reachable once signed in and refuses a stranger", async ({ browser }) => {
  // The session from claiming is still in this context.
  await page.goto("/");
  await expect(page.getByRole("link", { name: "Settings" })).toBeVisible();
  expect((await page.request.get("/api/v1/documents")).status()).toBe(200);

  const stranger = await browser.newContext();
  expect((await stranger.request.get("/api/v1/documents")).status()).toBe(401);
  // And the claim window is shut: the instance now has a passkey.
  expect((await stranger.request.post("/api/v1/auth/register/begin")).status()).toBe(401);
  await stranger.close();
});

test("signing out ends the session server-side", async () => {
  await page.goto("/settings");
  await page.getByRole("button", { name: /sign out/i }).click();

  await expect(page.getByRole("button", { name: /sign in with passkey/i })).toBeVisible();
  // Not merely a cleared cookie: the session is revoked, so the API refuses.
  await expect.poll(async () => (await page.request.get("/api/v1/documents")).status()).toBe(401);
});

test("the passkey created while claiming signs you back in", async () => {
  await page.goto("/");
  await expect(page.getByRole("button", { name: /sign in with passkey/i })).toBeVisible();

  // The credential from claiming is still on the authenticator, which is
  // what makes this a real ceremony rather than a surviving cookie.
  const held = await cdp.send("WebAuthn.getCredentials", { authenticatorId });
  expect(held.credentials.length).toBeGreaterThan(0);

  await page.getByRole("button", { name: /sign in with passkey/i }).click();

  await expect(page.getByRole("link", { name: "Settings" })).toBeVisible();
  await expect.poll(async () => (await page.request.get("/api/v1/documents")).status()).toBe(200);
});

test("a device that has never enrolled cannot sign in", async () => {
  // Same browser, no credential: the ceremony must fail cleanly — an error
  // the user can act on, not a hang and not a way in.
  await cdp.send("WebAuthn.clearCredentials", { authenticatorId });
  await context.clearCookies();
  await page.goto("/");

  await page.getByRole("button", { name: /sign in with passkey/i }).click();
  await expect(page.getByRole("button", { name: /sign in with passkey/i })).toBeVisible();
  expect((await page.request.get("/api/v1/documents")).status()).toBe(401);
});
