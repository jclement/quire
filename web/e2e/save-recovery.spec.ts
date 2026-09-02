// A save whose response never arrives (the tunnel between browser and
// server dropped it) must not strand the page: the write did land, so the
// client has to notice that rather than retry into a conflict with itself
// and then ignore every later refetch. Task toggles were the visible
// symptom — the checkbox flashed and nothing changed.
import { expect, test } from "@playwright/test";

test("a save whose response is lost is recognised as saved, and toggles still show", async ({
  page,
}) => {
  const res = await page.request.post("/api/v1/documents", {
    data: { type: "note", title: "Lost Save", markdown: "# Lost Save\n\n- [ ] existing task\n" },
  });
  const path = (await res.json()).data.path as string;
  await expect
    .poll(async () => (await (await page.request.get("/api/v1/tasks?view=inbox")).text()))
    .toContain("existing task");

  // The first PUT reaches the server; its response is dropped on the way back.
  let dropped = 0;
  await page.route(/\/api\/v1\/documents\/.*/, async (route) => {
    if (route.request().method() === "PUT" && dropped === 0) {
      dropped++;
      await route.fetch();
      await route.abort("failed");
      return;
    }
    await route.continue();
  });

  await page.goto(`/doc/${path}`);
  await expect(page.getByRole("heading", { level: 1 }).first()).toBeVisible();
  await page.keyboard.press("e");
  const editor = page.locator(".cm-content");
  await editor.click();
  await page.keyboard.press("ControlOrMeta+End");
  await page.keyboard.type("\ntyped line");
  await page
    .getByRole("group", { name: "View mode" })
    .getByRole("button", { name: "Read", exact: true })
    .click();

  // No conflict banner, no error: the server has the text, so it is saved.
  await expect(page.getByText("typed line")).toBeVisible();
  await expect(page.getByText("changed on disk")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Keep mine" })).toHaveCount(0);
  expect(dropped).toBe(1);

  const box = page.getByRole("checkbox", { name: /existing task/ });
  await box.click();
  await expect(box).toBeChecked();
  await expect
    .poll(async () => (await (await page.request.get(`/api/v1/documents/${path}`)).json()).data.markdown)
    .toContain("- [x] existing task");
});
