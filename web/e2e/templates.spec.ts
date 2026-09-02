// Templates: the starter set installs from Settings and never overwrites,
// the meeting default shapes a new meeting without any choosing, a named
// template is offered in the New dialog, and templates stay out of Notes.
import { expect, test } from "@playwright/test";

test.describe.configure({ mode: "serial" });

test("the starter set installs from Settings, idempotently", async ({ page }) => {
  await page.goto("/settings");
  await page.getByRole("button", { name: /install starter templates/i }).click();
  await expect(page.getByText(/installed \d+ templates/i)).toBeVisible();

  await page.getByRole("button", { name: /install starter templates/i }).click();
  await expect(page.getByText(/already installed/i)).toBeVisible();

  const list = await (await page.request.get("/api/v1/templates")).json();
  const names = (list.data as Array<{ name: string; default: boolean; for: string }>).map((t) => t.name);
  for (const expected of ["meeting", "daily", "decision", "one-on-one", "incident"]) {
    expect(names, `missing ${expected}`).toContain(expected);
  }
});

test("a new meeting takes the meeting default without asking", async ({ page }) => {
  await page.goto("/browse/meeting");
  await page.getByRole("button", { name: /new meeting/i }).click();
  await page.getByRole("textbox").first().fill("Templated Sync");
  await page.keyboard.press("Enter");
  await expect(page.getByRole("heading", { name: "Templated Sync" }).first()).toBeVisible();
  // New documents open in the editor, so the template shows as source.
  await expect(page.locator(".cm-content")).toContainText("Action items");
});

test("a named template is offered for notes and inherits its frontmatter", async ({ page }) => {
  await page.goto("/browse/note");
  await page.getByRole("button", { name: /new note/i }).click();
  const picker = page.getByLabel("Template");
  await expect(picker).toBeVisible();
  await picker.selectOption("decision");
  await page.getByRole("textbox").first().fill("Use SQLite");
  await page.keyboard.press("Enter");
  await expect(page.getByRole("heading", { name: "Use SQLite" }).first()).toBeVisible();
  await expect(page.locator(".cm-content")).toContainText("Consequences");

  const doc = await (await page.request.get("/api/v1/documents/notes/use-sqlite.md")).json();
  expect(doc.data.tags).toContain("decision");
  expect(doc.data.markdown).not.toContain("{{");
});

test("templates stay out of Notes and have their own place", async ({ page }) => {
  await page.goto("/browse/note");
  await expect(page.getByRole("link", { name: /^decision$/ })).toHaveCount(0);
  await page.goto("/browse/template");
  await expect(page.getByRole("link", { name: /^decision$/ })).toBeVisible();
});

test("a new daily note takes the daily template", async ({ page }) => {
  await page.request.post("/api/v1/daily/2031-05-05");
  const doc = await (await page.request.get("/api/v1/documents/daily/2031-05-05.md")).json();
  expect(doc.data.markdown).toContain("# 2031-05-05");
  expect(doc.data.markdown).toContain("## Focus");
});
