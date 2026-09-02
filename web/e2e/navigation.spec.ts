// The keyboard-first surfaces and the views nothing else exercises: the
// command palette, the calendar, and Mermaid rendering. quire's pitch is
// "fast, keyboard-first"; none of that was covered by a test.
import { expect, test } from "@playwright/test";

test.describe.configure({ mode: "serial" });

test("the command palette finds and opens a document", async ({ page }) => {
  await page.request.post("/api/v1/documents", {
    data: {
      type: "note",
      title: "Palette Target",
      markdown: "# Palette Target\n\nFindable.\n",
    },
  });

  await page.goto("/");
  await page.keyboard.press("ControlOrMeta+k");

  const input = page.getByLabel("Command palette input");
  await expect(input).toBeVisible();
  await input.fill("palette target");

  // Scoped to the palette's listbox: the area switcher's <select> also has
  // <option>s, and they are hidden.
  const option = page.getByRole("listbox").getByRole("option").filter({ hasText: /palette target/i }).first();
  await expect(option).toBeVisible();
  await page.keyboard.press("Enter");

  await expect(page.getByRole("heading", { name: "Palette Target" }).first()).toBeVisible();
  await expect(page).toHaveURL(/palette-target/);
});

test("the palette exposes commands behind >", async ({ page }) => {
  await page.goto("/");
  await page.keyboard.press("ControlOrMeta+k");
  await page.getByLabel("Command palette input").fill(">");
  await expect(page.getByRole("listbox").getByRole("option").first()).toBeVisible();
});

test("Escape closes the palette without navigating", async ({ page }) => {
  await page.goto("/calendar");
  await page.keyboard.press("ControlOrMeta+k");
  await expect(page.getByLabel("Command palette input")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByLabel("Command palette input")).toHaveCount(0);
  await expect(page).toHaveURL(/calendar/);
});

test("the calendar shows the month and marks a day with a daily note", async ({
  page,
}) => {
  // Creating today's daily note should light up today's cell.
  const today = new Date().toISOString().slice(0, 10);
  await page.request.post(`/api/v1/daily/${today}`);

  await page.goto("/calendar");
  // A month grid, not an empty shell. The heading specifically: daily-note
  // titles are also dates, so a bare text match for the year is ambiguous
  // once other specs have created some.
  await expect(
    page.getByRole("heading", { name: new RegExp(String(new Date().getFullYear())) }).first(),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: new RegExp(String(new Date().getFullYear())) }).first(),
  ).toBeVisible();
  const cell = page.getByRole("link", { name: new RegExp(`${Number(today.slice(8, 10))}\\b`) }).first();
  await expect(cell).toBeVisible();
});

test("a mermaid diagram renders as SVG, not as code", async ({ page }) => {
  const created = await page.request.post("/api/v1/documents", {
    data: {
      type: "note",
      title: "Diagram Note",
      markdown: "# Diagram Note\n\n```mermaid\ngraph TD;\n  A-->B;\n```\n",
    },
  });
  const { data } = await created.json();

  await page.goto(`/doc/${data.path}`);
  // Mermaid is lazy-loaded, so give the chunk a moment.
  await expect(page.locator("svg[id^='mermaid'], .mermaid svg").first()).toBeVisible({
    timeout: 20_000,
  });
});
