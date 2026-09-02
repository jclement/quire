// Share pages are a second, independent markdown pipeline: goldmark in Go,
// not the SPA's React renderer. Same syntax, different code — so "it renders
// in the app" says nothing about what a share recipient sees.
//
// They are also the only surface anonymous strangers load, which makes what
// they do and do not expose worth pinning down.
import { expect, test } from "@playwright/test";

test.describe.configure({ mode: "serial" });

/** Creates a document and shares it, returning the public token. */
async function share(page: import("@playwright/test").Page, title: string, markdown: string) {
  const created = await page.request.post("/api/v1/documents", {
    data: { type: "note", title, markdown },
  });
  const { data } = await created.json();
  const res = await page.request.post("/api/v1/shares", { data: { path: data.path } });
  return { token: (await res.json()).data.token as string, path: data.path as string };
}

test("a share page renders callouts, tasks and code for an anonymous reader", async ({
  page,
  browser,
}) => {
  const { token } = await share(
    page,
    "Rich Share",
    [
      "# Rich Share",
      "",
      "> [!warning] Mind the gap",
      "> Callouts become panels.",
      "",
      "- [ ] an open task",
      "- [x] a done task",
      "",
      "```go",
      'fmt.Println("shared")',
      "```",
      "",
      "A [[Private Page]] wikilink.",
    ].join("\n"),
  );

  // No cookies, no credentials — exactly what a recipient has.
  const anon = await browser.newContext();
  const reader = await anon.newPage();
  await reader.goto(`/s/${token}`);

  await expect(reader.getByText("Mind the gap")).toBeVisible();
  await expect(reader.locator(".callout")).toBeVisible();
  await expect(reader.locator('input[type="checkbox"]')).toHaveCount(2);
  await expect(reader.getByText('fmt.Println("shared")')).toBeVisible();

  // Wikilinks are flattened: their targets are private, so a share must not
  // turn into a way to discover or reach other documents.
  await expect(reader.getByText("Private Page")).toBeVisible();
  await expect(reader.locator('a[href*="Private"]')).toHaveCount(0);
  await expect(reader.locator('a[href^="/doc/"]')).toHaveCount(0);

  await anon.close();
});

test("a share page carries a strict CSP and asks not to be indexed", async ({
  page,
  request,
}) => {
  const { token } = await share(page, "Header Share", "# Header Share\n\nBody.\n");

  const res = await request.get(`/s/${token}`);
  expect(res.status()).toBe(200);
  const headers = res.headers();
  // The page carries no JavaScript at all, and the policy says so.
  expect(headers["content-security-policy"]).toContain("default-src 'none'");
  expect(headers["x-robots-tag"]).toContain("noindex");
  expect(headers["referrer-policy"]).toContain("no-referrer");
});

test("markdown never serves through the share file route", async ({ page, request }) => {
  const { token } = await share(page, "Guard Share", "# Guard Share\n\nBody.\n");

  // Even the shared document's own markdown is not fetchable as a file:
  // the page is the only representation a reader gets.
  for (const path of [
    "notes/guard-share.md",
    "notes/rich-share.md",
    "../.quire/auth.db",
  ]) {
    const res = await request.get(`/s/${token}/${path}`);
    expect(res.status(), `share served ${path}`).toBe(404);
  }
});

test("a revoked share is indistinguishable from one that never existed", async ({
  page,
  request,
}) => {
  const { token } = await share(page, "Doomed Share", "# Doomed Share\n\nBody.\n");
  expect((await request.get(`/s/${token}`)).status()).toBe(200);

  await page.request.delete(`/api/v1/shares/${token}`);

  const revoked = await request.get(`/s/${token}`);
  const neverExisted = await request.get("/s/aaaaaaaaaaaaaaaa");
  expect(revoked.status()).toBe(404);
  expect(neverExisted.status()).toBe(404);
});
