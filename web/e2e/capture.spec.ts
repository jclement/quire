// Quick capture: a task or a plain thought, both landing in the day's
// Captured section rather than at the end of whatever the file has become,
// and an exact due date without a second visit.
import { expect, test, type Page } from "@playwright/test";

const iso = (d: Date) =>
  `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;

async function todayNote(page: Page) {
  const res = await page.request.get(`/api/v1/daily/${iso(new Date())}`);
  if (!res.ok()) return "";
  return (await res.json()).data.markdown as string;
}

/** Gives today's note a Captured section, without touching the starter set
 *  (another spec owns that, and installing it here would race it). */
async function shapeTodaysNote(page: Page) {
  const day = iso(new Date());
  await page.request.post(`/api/v1/daily/${day}`);
  const doc = (await (await page.request.get(`/api/v1/daily/${day}`)).json()).data;
  if (doc.markdown.includes("## Captured")) return;
  await page.request.put(`/api/v1/documents/${doc.path}`, {
    data: {
      markdown: `${doc.markdown.trimEnd()}\n\n## Log\n\n- something typed later\n\n## Captured\n`,
      base_sha256: doc.sha256,
    },
  });
}

test("a thought is captured as prose, not as a checkbox", async ({ page }) => {
  await shapeTodaysNote(page);
  await page.goto("/");
  await page.keyboard.press("c");
  const input = page.getByRole("textbox").first();
  await expect(input).toBeVisible();

  await page.getByRole("button", { name: "note", exact: true }).click();
  // A note has no due date, so the day chips step aside.
  await expect(page.getByRole("button", { name: "today", exact: true })).toHaveCount(0);
  await input.fill("the solver gets slower above 800 wells");
  await page.keyboard.press("Enter");

  await expect
    .poll(() => todayNote(page))
    .toContain("- the solver gets slower above 800 wells");
  const note = await todayNote(page);
  expect(note).not.toContain("- [ ] the solver gets slower");
  // It sits in the Captured section, below the text typed under an earlier
  // heading rather than after it at the end of the file.
  expect(note.indexOf("## Captured")).toBeLessThan(
    note.indexOf("the solver gets slower"),
  );
  expect(note.indexOf("something typed later")).toBeLessThan(
    note.indexOf("## Captured"),
  );
  // And it is not a task, so the inbox stays clean.
  const inbox = await (await page.request.get("/api/v1/tasks?view=inbox")).text();
  expect(inbox).not.toContain("the solver gets slower");
});

test("a task can take an exact date without leaving the sheet", async ({ page }) => {
  await shapeTodaysNote(page);
  await page.goto("/");
  await page.keyboard.press("c");
  const input = page.getByRole("textbox").first();
  await input.fill("signed permission slip");
  await page.getByLabel("Due date").fill("2026-09-11");
  // Enter saves from the text field, which is where the cursor lives.
  await input.click();
  await page.keyboard.press("Enter");

  await expect
    .poll(() => todayNote(page))
    .toContain("- [ ] signed permission slip 📅 2026-09-11");
});
