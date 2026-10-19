// Project status and the Projects board: the status chip sets a status and
// stamps the closing date in the file, the board shows a column per status,
// dragging a card changes its status or its place, and the board keeps
// closed columns to what closed recently.
import { expect, test, type Locator, type Page } from "@playwright/test";

test.describe.configure({ mode: "serial" });

async function createProject(page: Page, title: string, frontmatter: string) {
  const res = await page.request.post("/api/v1/documents", {
    data: { type: "project", title, markdown: `---\n${frontmatter}\n---\n# ${title}\n` },
  });
  expect(res.ok()).toBeTruthy();
  return (await res.json()).data.path as string;
}

async function frontmatterOf(page: Page, path: string) {
  const res = await page.request.get(`/api/v1/documents/${path}`);
  return (await res.json()).data.frontmatter as Record<string, unknown>;
}

const column = (page: Page, label: string) =>
  page.getByRole("region", { name: "Projects board" }).getByRole("region", { name: label, exact: true });
const card = (scope: Locator, title: string) => scope.getByRole("link", { name: title });

/** A mouse drag in steps, the way dnd-kit's pointer sensor needs to see it. */
async function drag(page: Page, from: Locator, to: Locator, below = false) {
  const a = (await from.boundingBox())!;
  const b = (await to.boundingBox())!;
  await page.mouse.move(a.x + a.width / 2, a.y + a.height / 2);
  await page.mouse.down();
  await page.mouse.move(a.x + a.width / 2, a.y + a.height / 2 + 8, { steps: 4 });
  const y = below ? b.y + b.height - 4 : b.y + b.height / 2;
  await page.mouse.move(b.x + b.width / 2, y, { steps: 12 });
  await page.waitForTimeout(100);
  await page.mouse.up();
}

test("the status chip sets a status and the file records when it closed", async ({ page }) => {
  const path = await createProject(page, "Chip Project", "status: someday\ntags: [infra]");
  await page.goto(`/doc/${path}`);
  const chip = page.getByRole("button", { name: "Project status" });
  // An older word reads as its status.
  await expect(chip).toContainText("planned");
  // Same height as the tags chip beside it.
  const tags = page.getByRole("button", { name: "Add tag to this document" }).locator("..");
  expect((await chip.boundingBox())!.height).toBe((await tags.boundingBox())!.height);

  await chip.click();
  await page.getByRole("option", { name: "Complete" }).click();
  await expect(chip).toContainText("complete");
  await expect.poll(async () => (await frontmatterOf(page, path)).status).toBe("complete");
  const fm = await frontmatterOf(page, path);
  expect(String(fm.completed)).toMatch(/^\d{4}-\d{2}-\d{2}/);

  await chip.click();
  await page.getByRole("option", { name: "Active" }).click();
  await expect.poll(async () => (await frontmatterOf(page, path)).completed).toBeUndefined();
});

test("the board drags between columns and within one, and remembers the view", async ({ page }) => {
  await createProject(page, "Board Alpha", "status: active");
  await createProject(page, "Board Beta", "status: active");
  const gamma = await createProject(page, "Board Gamma", "status: hold");
  await page.goto("/browse/project");
  await page.getByRole("button", { name: "Board" }).click();
  await expect(column(page, "Active")).toBeVisible();

  // The view survives a reload.
  await page.reload();
  await expect(page.getByRole("region", { name: "Projects board" })).toBeVisible();

  // Across columns: Active → On hold changes the status.
  const alpha = card(column(page, "Active"), "Board Alpha");
  await drag(page, alpha, card(column(page, "On hold"), "Board Gamma"));
  await expect(card(column(page, "On hold"), "Board Alpha")).toBeVisible();
  const alphaPath = "projects/board-alpha.md";
  await expect.poll(async () => (await frontmatterOf(page, alphaPath)).status).toBe("hold");

  // Within a column: Gamma below Alpha, and the ranks say so on disk.
  const hold = column(page, "On hold");
  const titles = () => hold.getByRole("link").allTextContents();
  const before = await titles();
  const upper = before.findIndex((t) => t.includes("Board Alpha")) < before.findIndex((t) => t.includes("Board Gamma"))
    ? "Board Alpha"
    : "Board Gamma";
  const lower = upper === "Board Alpha" ? "Board Gamma" : "Board Alpha";
  await drag(page, card(hold, upper), card(hold, lower), true);
  await expect.poll(async () => {
    const t = await titles();
    return t.findIndex((x) => x.includes(lower)) < t.findIndex((x) => x.includes(upper));
  }).toBe(true);
  await expect
    .poll(async () => {
      const a = Number((await frontmatterOf(page, alphaPath)).rank);
      const g = Number((await frontmatterOf(page, gamma)).rank);
      return upper === "Board Alpha" ? a > g : g > a;
    })
    .toBe(true);

  // Clicking a card still opens it.
  await card(column(page, "On hold"), "Board Beta").or(card(column(page, "Active"), "Board Beta")).click();
  await expect(page.getByRole("heading", { level: 1, name: "Board Beta" }).first()).toBeVisible();
});

test("closed columns keep to what closed recently; columns collapse and stay collapsed", async ({ page }) => {
  await createProject(page, "Long Done", "status: complete\ncompleted: 2025-01-15");
  await createProject(page, "Just Done", "status: completed");
  await page.goto("/browse/project");
  await page.getByRole("button", { name: "Board" }).click();
  const complete = column(page, "Complete");
  await expect(card(complete, "Just Done")).toBeVisible();
  await expect(card(complete, "Long Done")).toHaveCount(0);
  await complete.getByRole("button", { name: /older/ }).click();
  await expect(card(complete, "Long Done")).toBeVisible();

  // Cancelled starts collapsed; Planned can be folded and stays folded.
  await expect(page.getByRole("button", { name: "Expand Cancelled" })).toBeVisible();
  await column(page, "Planned").getByRole("button", { name: "Collapse Planned" }).click();
  await page.reload();
  await expect(page.getByRole("button", { name: "Expand Planned" })).toBeVisible();
  await page.getByRole("button", { name: "Expand Planned" }).click();
  await expect(column(page, "Planned")).toBeVisible();

  // Back to the list, which shows each project's status.
  await page.getByRole("button", { name: "List" }).click();
  await expect(page.getByRole("row", { name: /Just Done/ })).toContainText("complete");
});
