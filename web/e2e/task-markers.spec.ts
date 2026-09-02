// Tasks written with `*` or `+` bullets toggle like `-` ones: the indexer
// always accepted them, so the checkbox was live, and the toggle used to
// write the line back unchanged.
import { expect, test } from "@playwright/test";

test("a `* [ ]` task toggles from read mode", async ({ page }) => {
  const res = await page.request.post("/api/v1/documents", {
    data: { type: "note", title: "Star Tasks", markdown: "# Star Tasks\n\n* [ ] star task\n+ [ ] plus task\n" },
  });
  const path = (await res.json()).data.path as string;
  await expect
    .poll(async () => (await (await page.request.get("/api/v1/tasks?view=inbox")).text()))
    .toContain("plus task");
  await page.goto(`/doc/${path}`);
  await expect(page.getByRole("heading", { level: 1 }).first()).toBeVisible();
  for (const name of ["star task", "plus task"]) {
    const box = page.getByRole("checkbox", { name: new RegExp(name) });
    await box.click();
    await expect(box).toBeChecked();
  }
  const text = (await (await page.request.get(`/api/v1/documents/${path}`)).json()).data.markdown as string;
  expect(text).toMatch(/\* \[x\] star task ✅ \d{4}-\d{2}-\d{2}/);
  expect(text).toMatch(/\+ \[x\] plus task ✅ \d{4}-\d{2}-\d{2}/);
});
