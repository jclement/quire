// Areas: work / personal / unclassified, as a frontmatter key discovered
// from the vault. The switcher narrows Browse, Search, Tasks and Today; new
// documents file under the current area; the area chip on a document
// re-files it in place; the journal stays whole.
import { expect, test } from "@playwright/test";

test.describe.configure({ mode: "serial" });

async function seed(page: import("@playwright/test").Page) {
  for (const [title, area, body] of [
    ["Work Roadmap", "work", "- [ ] ship the roadmap 📅 2026-09-02"],
    ["Beach Trip", "personal", "- [ ] book the beach 📅 2026-09-02"],
    ["Loose Thought", "", "- [ ] unfiled idea"],
  ] as const) {
    await page.request.post("/api/v1/documents", {
      data: {
        type: "note",
        title,
        area,
        markdown: `# ${title}\n\n${body}\n`,
      },
    });
  }
}

test("the switcher narrows Browse, and Unclassified is the unfiled set", async ({ page }) => {
  await seed(page);
  await page.goto("/browse/note");
  const area = page.getByLabel("Area", { exact: true });
  await expect(area).toBeVisible();
  // Seeded areas are offered even before use; discovered ones join them.
  // Order-free: ties sort alphabetically, and counts shift as specs run.
  for (const label of ["All areas", "Work", "Personal", "Unclassified"]) {
    await expect(area.locator("option", { hasText: label })).toHaveCount(1);
  }

  await area.selectOption("work");
  await expect(page.getByRole("link", { name: "Work Roadmap" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Beach Trip" })).toHaveCount(0);

  await area.selectOption("none");
  await expect(page.getByRole("link", { name: "Loose Thought" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Work Roadmap" })).toHaveCount(0);

  // The choice survives a reload.
  await page.reload();
  await expect(page.getByLabel("Area", { exact: true })).toHaveValue("none");
  await page.getByLabel("Area", { exact: true }).selectOption("");
});

test("Today follows the switcher; the daily note is in every area", async ({ page }) => {
  await page.goto("/");
  const area = page.getByLabel("Area", { exact: true });
  await area.selectOption("personal");
  await expect(page.getByText("book the beach").first()).toBeVisible();
  await expect(page.getByText("ship the roadmap")).toHaveCount(0);
  await area.selectOption("");
});

test("a new document files under the current area", async ({ page }) => {
  await page.goto("/browse/note");
  await page.getByLabel("Area", { exact: true }).selectOption("work");
  await page.getByRole("button", { name: /new note/i }).click();
  const title = page.getByRole("textbox").first();
  await title.fill("Made In Work");
  await page.keyboard.press("Enter");
  await expect(page.getByRole("heading", { name: "Made In Work" }).first()).toBeVisible();

  const doc = await (await page.request.get("/api/v1/documents/notes/made-in-work.md")).json();
  expect(doc.data.area).toBe("work");
  expect(doc.data.markdown).toContain("area: work");
  await page.getByLabel("Area", { exact: true }).selectOption("");
});

test("the area chip re-files a document in place", async ({ page }) => {
  await page.goto("/doc/notes/loose-thought.md");
  const chip = page.getByLabel("Document area");
  await expect(chip).toHaveValue("");
  await chip.selectOption("personal");
  await expect
    .poll(async () => (await (await page.request.get("/api/v1/documents/notes/loose-thought.md")).json()).data.area)
    .toBe("personal");
  // Back to unclassified removes the key rather than writing area: "".
  await chip.selectOption("");
  await expect
    .poll(async () => (await (await page.request.get("/api/v1/documents/notes/loose-thought.md")).json()).data.markdown)
    .not.toContain("area:");
});

test("search honours the switcher and a typed area: wins", async ({ page }) => {
  await page.goto("/search");
  await page.getByLabel("Area", { exact: true }).selectOption("work");
  // Bare is:task, with the switcher on Work — the 📅 marker is parsed out of
  // task text at index time, so it is not a searchable term.
  await page.getByLabel("Search query").fill("is:task");
  await expect(page.getByText("ship the roadmap").first()).toBeVisible();
  await expect(page.getByText("book the beach")).toHaveCount(0);
  await page.getByLabel("Search query").fill("is:task area:personal");
  await expect(page.getByText("book the beach").first()).toBeVisible();
  await page.getByLabel("Area", { exact: true }).selectOption("");
});
