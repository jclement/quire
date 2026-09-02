// The document header and properties strip: tag chips with "+", the share
// button lit while a link exists, "Insert drawing" in the ⋯ menu, and the
// remembered Edit / Split preference for new documents.
import { expect, test, type Page } from "@playwright/test";

test.describe.configure({ mode: "serial" });

async function createDoc(page: Page, title: string, markdown: string) {
  const res = await page.request.post("/api/v1/documents", {
    data: { type: "note", title, markdown },
  });
  const { data } = await res.json();
  return data.path as string;
}

async function diskText(page: Page, path: string) {
  const res = await page.request.get(`/api/v1/documents/${path}`);
  const { data } = await res.json();
  return data.markdown as string;
}

test("tags are chips: add from the vault's tags or as typed, remove with ×", async ({ page }) => {
  await createDoc(page, "Tag Source", "# Tag Source\n\nabout #ops and #hiring\n");
  const path = await createDoc(page, "Tag Target", "# Tag Target\n\nplain\n");
  await page.goto(`/doc/${path}`);
  await expect(page.getByRole("heading", { level: 1 }).first()).toBeVisible();

  await page.getByRole("button", { name: "Add tag to this document" }).click();
  const popover = page.getByRole("dialog", { name: "Add tag" });
  // Existing vault tags are offered…
  await expect(popover.getByRole("button", { name: "ops" })).toBeVisible();
  await popover.getByRole("button", { name: "ops" }).click();
  await expect.poll(() => diskText(page, path)).toContain("tags: [ops]");

  // …and a new one can be typed.
  await page.getByRole("button", { name: "Add tag to this document" }).click();
  await page.getByLabel("Search tags").fill("#Roadmap");
  await page.keyboard.press("Enter");
  await expect.poll(() => diskText(page, path)).toContain("tags: [ops, roadmap]");

  // Chips link to the tag search and remove cleanly; the last removal drops the key.
  await expect(page.getByRole("link", { name: "roadmap" })).toHaveAttribute("href", /tag%3Aroadmap|tag:roadmap/);
  await page.getByRole("button", { name: "Remove tag ops" }).click();
  await expect.poll(() => diskText(page, path)).toContain("tags: [roadmap]");
  await page.getByRole("button", { name: "Remove tag roadmap" }).click();
  await expect.poll(() => diskText(page, path)).not.toContain("tags:");
});

test("the share button lights up while a link exists", async ({ page }) => {
  const path = await createDoc(page, "Shared Note", "# Shared Note\n");
  await page.goto(`/doc/${path}`);
  const share = page.getByRole("button", { name: "Share this document" });
  await expect(share).toHaveAttribute("aria-pressed", "false");
  const res = await page.request.post("/api/v1/shares", { data: { path } });
  expect(res.status()).toBe(201);
  const { data } = await res.json();
  await page.reload();
  await expect(share).toHaveAttribute("aria-pressed", "true");
  await page.request.delete(`/api/v1/shares/${data.token}`);
  await page.reload();
  await expect(share).toHaveAttribute("aria-pressed", "false");
});

test("Insert drawing lives in the ⋯ menu too", async ({ page }) => {
  const path = await createDoc(page, "Menu Sketch", "# Menu Sketch\n");
  await page.goto(`/doc/${path}`);
  await page.getByRole("button", { name: "Document actions" }).click();
  await page.getByRole("button", { name: "Insert drawing" }).click();
  await expect(page.getByRole("dialog", { name: "Drawing" })).toBeVisible();
  await expect.poll(() => diskText(page, path)).toContain("![Drawing](attachments/");
  await page.getByRole("button", { name: "Cancel" }).click();
});

test("new documents open in the last-used editing mode", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 900 });
  const first = await createDoc(page, "Mode One", "# Mode One\n");
  await page.goto(`/doc/${first}`);
  await page.getByRole("group", { name: "View mode" }).getByRole("button", { name: "Split" }).click();
  await expect(page.locator(".cm-content")).toBeVisible();

  await page.goto("/browse/note");
  await page.getByRole("button", { name: /new note/i }).click();
  await page.getByRole("textbox").first().fill("Mode Two");
  await page.keyboard.press("Enter");
  await expect(page.getByRole("heading", { name: "Mode Two" }).first()).toBeVisible();
  const modeGroup = page.getByRole("group", { name: "View mode" });
  await expect(modeGroup.getByRole("button", { name: "Split" })).toHaveAttribute("aria-pressed", "true");

  // Choosing Edit is remembered the same way.
  await modeGroup.getByRole("button", { name: "Edit" }).click();
  await page.goto("/browse/note");
  await page.getByRole("button", { name: /new note/i }).click();
  await page.getByRole("textbox").first().fill("Mode Three");
  await page.keyboard.press("Enter");
  await expect(page.getByRole("heading", { name: "Mode Three" }).first()).toBeVisible();
  await expect(modeGroup.getByRole("button", { name: "Edit" })).toHaveAttribute("aria-pressed", "true");
});
