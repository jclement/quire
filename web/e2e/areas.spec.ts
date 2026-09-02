// Areas: work / personal / unclassified, as a frontmatter key discovered
// from the vault. The switcher badge narrows Browse, Search, Tasks and Today
// to one or several areas; new documents file under the current area; the
// area badge on a document re-files it in place; the journal stays whole.
import { expect, test, type Page } from "@playwright/test";

test.describe.configure({ mode: "serial" });

async function seed(page: Page) {
  // Areas are opt-in: define two so the switcher exists at all.
  await page.request.put("/api/v1/areas", {
    data: { areas: [{ name: "work", color: "blue" }, { name: "personal", color: "green" }] },
  });
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

/** Opens the sidebar switcher and picks one option by label. */
async function pickArea(page: Page, label: string) {
  await page.getByRole("button", { name: "Area", exact: true }).click();
  await page
    .getByRole("listbox", { name: "Choose areas" })
    .getByRole("option", { name: label, exact: true })
    .click();
}

/** Closes the switcher popup if it is open (multi-select leaves it open). */
async function closeSwitcher(page: Page) {
  if (await page.getByRole("listbox", { name: "Choose areas" }).count()) {
    await page.keyboard.press("Escape");
  }
}

const switcher = (page: Page) => page.getByRole("button", { name: "Area", exact: true });

test("the switcher narrows Browse, and Unclassified is the unfiled set", async ({ page }) => {
  await seed(page);
  await page.goto("/browse/note");
  await expect(switcher(page)).toBeVisible();
  await expect(switcher(page)).toContainText("all");
  // Seeded areas are offered even before use; discovered ones join them.
  await switcher(page).click();
  const list = page.getByRole("listbox", { name: "Choose areas" });
  for (const label of ["All areas", "Work", "Personal", "Unclassified"]) {
    await expect(list.getByRole("option", { name: label, exact: true })).toHaveCount(1);
  }
  await page.keyboard.press("Escape");

  await pickArea(page, "Work");
  await closeSwitcher(page);
  await expect(switcher(page)).toContainText("Work");
  await expect(page.getByRole("link", { name: "Work Roadmap" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Beach Trip" })).toHaveCount(0);

  // Several at once: Work stays on, Personal joins it.
  await pickArea(page, "Personal");
  await closeSwitcher(page);
  // Several areas: the badge shows their dots, not a list of names.
  await expect(switcher(page)).not.toContainText("Work");
  await expect(switcher(page).getByRole("img", { name: "Work" })).toHaveCount(1);
  await expect(switcher(page).getByRole("img", { name: "Personal" })).toHaveCount(1);
  await expect(page.getByRole("link", { name: "Work Roadmap" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Beach Trip" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Loose Thought" })).toHaveCount(0);

  // "All areas" clears the selection.
  await pickArea(page, "All areas");
  await expect(page.getByRole("link", { name: "Loose Thought" })).toBeVisible();

  await pickArea(page, "Unclassified");
  await closeSwitcher(page);
  await expect(page.getByRole("link", { name: "Loose Thought" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Work Roadmap" })).toHaveCount(0);

  // The choice survives a reload.
  await page.reload();
  await expect(switcher(page)).toContainText("Unclassified");
  await pickArea(page, "All areas");
});

test("Today follows the switcher; the daily note is in every area", async ({ page }) => {
  await page.goto("/");
  await pickArea(page, "Personal");
  await closeSwitcher(page);
  await expect(page.getByText("book the beach").first()).toBeVisible();
  await expect(page.getByText("ship the roadmap")).toHaveCount(0);
  await pickArea(page, "All areas");
});

test("a new document files under the current area and opens for editing", async ({ page }) => {
  await page.goto("/browse/note");
  await pickArea(page, "Work");
  await closeSwitcher(page);
  await page.getByRole("button", { name: /new note/i }).click();
  const title = page.getByRole("textbox").first();
  await title.fill("Made In Work");
  await page.keyboard.press("Enter");
  await expect(page.getByRole("heading", { name: "Made In Work" }).first()).toBeVisible();
  // Fresh documents open in the editor rather than an empty read view.
  await expect(page.locator(".cm-content")).toBeVisible();

  const doc = await (await page.request.get("/api/v1/documents/notes/made-in-work.md")).json();
  expect(doc.data.area).toBe("work");
  expect(doc.data.markdown).toContain("area: work");
  await pickArea(page, "All areas");
});

test("the area badge re-files a document in place", async ({ page }) => {
  await page.goto("/doc/notes/loose-thought.md");
  const badge = page.getByRole("button", { name: "Document area" });
  await expect(badge).toContainText("unassigned");
  await badge.click();
  await page
    .getByRole("listbox", { name: "Choose area" })
    .getByRole("option", { name: "Personal", exact: true })
    .click();
  await expect
    .poll(async () => (await (await page.request.get("/api/v1/documents/notes/loose-thought.md")).json()).data.area)
    .toBe("personal");
  await expect(badge).toContainText("personal");
  // Back to unassigned removes the key rather than writing area: "".
  await badge.click();
  await page
    .getByRole("listbox", { name: "Choose area" })
    .getByRole("option", { name: "Unassigned", exact: true })
    .click();
  await expect
    .poll(async () => (await (await page.request.get("/api/v1/documents/notes/loose-thought.md")).json()).data.markdown)
    .not.toContain("area:");
  await expect(badge).toContainText("unassigned");
});

test("search honours the switcher and a typed area: wins", async ({ page }) => {
  await page.goto("/search");
  await pickArea(page, "Work");
  await closeSwitcher(page);
  // Bare is:task, with the switcher on Work — the 📅 marker is parsed out of
  // task text at index time, so it is not a searchable term.
  await page.getByLabel("Search query").fill("is:task");
  await expect(page.getByText("ship the roadmap").first()).toBeVisible();
  await expect(page.getByText("book the beach")).toHaveCount(0);
  await page.getByLabel("Search query").fill("is:task area:personal");
  await expect(page.getByText("book the beach").first()).toBeVisible();
  await pickArea(page, "All areas");
});
