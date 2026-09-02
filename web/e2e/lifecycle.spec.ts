// The document lifecycle from the UI: rename with link rewriting, delete,
// and daily-note navigation. All three are destructive or navigational and
// none had browser coverage.
import { expect, test } from "@playwright/test";

test.describe.configure({ mode: "serial" });

async function createDoc(
  page: import("@playwright/test").Page,
  type: string,
  title: string,
  markdown: string,
) {
  const res = await page.request.post("/api/v1/documents", {
    data: { type, title, markdown },
  });
  expect(res.status()).toBe(201);
  return (await res.json()).data.path as string;
}

/** Opens the document header's "…" menu. */
async function openActions(page: import("@playwright/test").Page) {
  await page.getByLabel("Document actions").click();
}

test("renaming a document rewrites the links pointing at it", async ({ page }) => {
  await createDoc(page, "note", "Old Name", "# Old Name\n\nThe target.\n");
  const referrer = await createDoc(
    page,
    "note",
    "Referrer",
    "# Referrer\n\nPoints at [[Old Name]].\n",
  );

  await page.goto("/doc/notes/old-name.md");
  await openActions(page);
  await page.getByRole("button", { name: /rename/i }).click();

  const input = page.getByLabel("New path");
  await expect(input).toBeVisible();
  await input.fill("notes/new-name.md");
  await page.keyboard.press("Enter");

  // The document moves…
  await expect.poll(async () => (await page.request.get("/api/v1/documents/notes/new-name.md")).status()).toBe(200);
  expect((await page.request.get("/api/v1/documents/notes/old-name.md")).status()).toBe(404);

  // …and the link that would otherwise have broken follows it. Links that
  // still resolve are deliberately left alone, so this is the interesting
  // half of the behaviour.
  const doc = await page.request.get(`/api/v1/documents/${referrer}`);
  expect(await doc.text()).toContain("new-name");
});

test("deleting a document asks first, then removes it", async ({ page }) => {
  const path = await createDoc(page, "note", "Doomed Doc", "# Doomed Doc\n\nBye.\n");

  await page.goto(`/doc/${path}`);
  await openActions(page);
  await page.getByRole("button", { name: /delete/i }).click();

  // Destructive actions are always a confirmation, and it names what goes.
  await expect(page.getByRole("heading", { name: /delete doomed doc\?/i })).toBeVisible();
  await expect(page.getByText(path)).toBeVisible();

  await page.getByRole("button", { name: "Delete", exact: true }).click();

  await expect.poll(async () => (await page.request.get(`/api/v1/documents/${path}`)).status()).toBe(404);
});

test("cancelling a delete leaves the document alone", async ({ page }) => {
  const path = await createDoc(page, "note", "Spared Doc", "# Spared Doc\n\nStays.\n");

  await page.goto(`/doc/${path}`);
  await openActions(page);
  await page.getByRole("button", { name: /delete/i }).click();
  await expect(page.getByRole("heading", { name: /delete spared doc\?/i })).toBeVisible();

  await page.getByRole("button", { name: /cancel/i }).click();

  await page.waitForTimeout(300);
  expect((await page.request.get(`/api/v1/documents/${path}`)).status()).toBe(200);
});

test("daily notes navigate by day and create on demand", async ({ page }) => {
  await page.goto("/daily/2026-03-14");
  // A day with no note yet offers to start one rather than 404ing.
  const start = page.getByRole("button", { name: /start|create/i }).first();
  await expect(start).toBeVisible();
  await start.click();
  await expect(page.getByRole("heading", { level: 1 }).first()).toBeVisible();

  // Previous/next move a day at a time.
  await page.getByLabel("Previous day").click();
  await expect(page).toHaveURL(/2026-03-13/);
  await page.getByLabel("Next day").click();
  await expect(page).toHaveURL(/2026-03-14/);
});
