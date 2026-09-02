// Completing a recurring task spawns its next occurrence on the line below
// — from a checkbox in read mode, the same as from the API or the CLI.
import { expect, test } from "@playwright/test";

test("ticking a 🔁 task writes the next occurrence", async ({ page }) => {
  const res = await page.request.post("/api/v1/documents", {
    data: {
      type: "note",
      title: "Chores",
      markdown: "# Chores\n\n- [ ] change the filter 📅 2026-09-01 🛫 2026-08-25 🔁 every month\n",
    },
  });
  const path = (await res.json()).data.path as string;
  await expect
    .poll(async () => (await (await page.request.get("/api/v1/tasks?view=today")).text()))
    .toContain("change the filter");
  await page.goto(`/doc/${path}`);
  await page.getByRole("checkbox", { name: /change the filter/ }).first().click();
  await expect
    .poll(async () => (await (await page.request.get(`/api/v1/documents/${path}`)).json()).data.markdown as string)
    .toContain("- [ ] change the filter 📅 2026-10-01 🛫 2026-09-24 🔁 every month");
  const text = (await (await page.request.get(`/api/v1/documents/${path}`)).json()).data.markdown as string;
  expect(text).toMatch(/- \[x\] change the filter 📅 2026-09-01 🛫 2026-08-25 🔁 every month ✅ \d{4}-\d{2}-\d{2}\n- \[ \] change the filter 📅 2026-10-01/);
});
