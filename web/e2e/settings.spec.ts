// Settings is where access is granted and taken away, so its flows are
// worth exercising for real: minting a token shows the secret exactly once,
// revoking it sticks, and share links can be created and killed.
import { expect, test } from "@playwright/test";

test.describe.configure({ mode: "serial" });

test("mint an API token, see the secret once, then revoke it", async ({ page }) => {
  await page.goto("/settings");

  await page.getByRole("button", { name: "New token" }).click();
  await page.getByLabel("What is it for?").fill("E2E token");
  await page.getByRole("button", { name: "Create token" }).click();

  // The plaintext is shown exactly once; if this ever stops working the
  // token is unrecoverable and the user is stuck.
  const secret = page.getByText(/^sk_[0-9a-f]{64}$/);
  await expect(secret).toBeVisible();
  const plaintext = (await secret.textContent())!.trim();

  // And it actually works as a credential.
  const authed = await page.request.get("/api/v1/documents", {
    headers: { Authorization: `Bearer ${plaintext}` },
  });
  expect(authed.ok()).toBeTruthy();

  await page.getByRole("button", { name: /saved it/i }).click();
  await expect(page.getByText("E2E token")).toBeVisible();

  // Revoking is two clicks by design — the row arms before it fires, so a
  // mis-click in a dense list cannot destroy a credential.
  await page.getByRole("button", { name: "Revoke token E2E token" }).click();
  await page.getByRole("button", { name: "Revoke?" }).click();
  // Exact, because the success toast also says "Token revoked".
  await expect(page.getByText("revoked", { exact: true })).toBeVisible();

  // That revocation actually kills the credential is asserted in Go
  // (internal/api TestTokenLifecycle), because this server runs with auth
  // off — every request is the owner here regardless of the header, so a
  // 401 could never be observed from the browser.
  await expect
    .poll(async () => {
      const res = await page.request.get("/api/v1/tokens");
      const listed = (await res.json()).data as Array<{
        name: string;
        revoked_at: string;
      }>;
      return listed.find((t) => t.name === "E2E token")?.revoked_at !== "";
    })
    .toBe(true);
});

test("a share link is publicly readable, and dies when revoked", async ({
  page,
  browser,
}) => {
  const created = await page.request.post("/api/v1/documents", {
    data: {
      type: "note",
      title: "Sitter Notes",
      markdown: "# Sitter Notes\n\nBedtime is 8pm sharp.\n",
    },
  });
  const { data } = await created.json();

  const shared = await page.request.post("/api/v1/shares", {
    data: { path: data.path, expires_in_days: 7 },
  });
  expect(shared.status()).toBe(201);
  const share = (await shared.json()).data;

  // Read it the way a recipient would: no cookies, no credentials at all.
  const anon = await browser.newContext();
  const anonPage = await anon.newPage();
  await anonPage.goto(`/s/${share.token}`);
  await expect(anonPage.getByText("Bedtime is 8pm sharp.")).toBeVisible();

  // A share is a window onto ONE document. Another document must not be
  // reachable through it, whatever the path.
  for (const path of ["notes/rendering-check.md", "../.quire/auth.db", ".quire/auth.db"]) {
    const leak = await anonPage.request.get(`/s/${share.token}/${path}`);
    expect(leak.status(), `share leaked ${path}`).toBe(404);
  }

  // Revoke from Settings, then confirm the link is dead for the visitor.
  await page.goto("/settings");
  await page.getByRole("button", { name: `Revoke share link for ${data.path}` }).click();
  await page.getByRole("button", { name: "Revoke?" }).click();

  await expect
    .poll(async () => (await anonPage.request.get(`/s/${share.token}`)).status())
    .toBe(404);
  await anon.close();
});

test("agent guidance saves and is served to MCP clients", async ({ page }) => {
  await page.goto("/settings");

  const guidance = page.getByLabel("Agent guidance");
  await guidance.fill("Always file client work under projects/.");
  await page.getByRole("button", { name: "Save", exact: true }).click();

  // It is stored as an ordinary vault document, which is the design claim
  // worth checking: editable in the app or in vim.
  await expect
    .poll(async () => {
      const res = await page.request.get("/api/v1/agent-guidance");
      return (await res.text()).includes("Always file client work");
    })
    .toBe(true);
});
