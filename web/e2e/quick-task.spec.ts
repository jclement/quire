// "+ text [when]" in the command palette adds a task to today's daily
// note without leaving the page; a trailing date word sets the due date.
import { expect, test } from "@playwright/test";

test("+ in the palette adds a task to today's note", async ({ page }) => {
  await page.goto("/browse/note");
  await page.keyboard.press("ControlOrMeta+k");
  const input = page.getByLabel("Command palette input");
  await input.fill("+ call the plumber tomorrow");
  const option = page.getByRole("listbox").getByRole("option").first();
  await expect(option).toContainText("Add task: call the plumber");
  await expect(option).toContainText("tomorrow");
  await page.keyboard.press("Enter");
  await expect(page.getByText(/Added task, due tomorrow/)).toBeVisible();

  const today = new Date();
  const tomorrow = new Date(today.getTime() + 86_400_000);
  const iso = (d: Date) =>
    `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
  await expect
    .poll(async () => {
      const res = await page.request.get(`/api/v1/daily/${iso(today)}`);
      return res.ok() ? ((await res.json()).data.markdown as string) : "";
    })
    .toContain(`- [ ] call the plumber 📅 ${iso(tomorrow)}`);
  // The page did not navigate.
  await expect(page).toHaveURL(/browse\/note/);
});
