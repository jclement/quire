// Areas are opt-in and defined in Settings with a colour. Nothing
// area-shaped appears until two or more exist; then the switcher, chips and
// dots do, in the chosen colour.
import { expect, test } from "@playwright/test";

test.describe.configure({ mode: "serial" });

test("with fewer than two areas defined, nothing area-shaped shows", async ({ page }) => {
  await page.request.put("/api/v1/areas", { data: { areas: [] } });
  await page.goto("/browse/note");
  await expect(page.getByLabel("Area", { exact: true })).toHaveCount(0);

  await page.request.put("/api/v1/areas", { data: { areas: [{ name: "work", color: "blue" }] } });
  await page.reload();
  await expect(page.getByLabel("Area", { exact: true })).toHaveCount(0);
});

test("defining a second area in Settings, with a colour, turns everything on", async ({ page }) => {
  await page.goto("/settings");
  await page.getByLabel("New area name").fill("Side Project");
  await page.getByRole("button", { name: "Add", exact: true }).click();
  const group = page.getByRole("radiogroup", { name: /colour for side project/i });
  await group.getByRole("radio", { name: "violet" }).click();
  await page.getByRole("button", { name: "Save areas" }).click();
  await expect(page.getByText("Areas saved")).toBeVisible();

  const list = (await (await page.request.get("/api/v1/areas")).json()).data as Array<{
    area: string;
    color: string;
    defined: boolean;
  }>;
  const side = list.find((a) => a.area === "side project");
  expect(side?.color).toBe("violet");
  expect(side?.defined).toBe(true);

  await page.goto("/browse/note");
  const switcher = page.getByLabel("Area", { exact: true });
  await expect(switcher).toBeVisible();
  await expect(switcher.locator("option", { hasText: "Side project" })).toHaveCount(1);

  await page.request.post("/api/v1/documents", {
    data: { type: "note", title: "Violet Note", area: "side project", markdown: "# Violet Note\n" },
  });
  await page.goto("/browse/note");
  const dot = page.getByRole("img", { name: "Area: side project" }).first();
  await expect(dot).toBeVisible();
  expect(await dot.evaluate((el) => getComputedStyle(el).backgroundColor)).not.toBe("rgba(0, 0, 0, 0)");

  await page.goto("/doc/notes/violet-note.md");
  await expect(page.getByLabel("Document area")).toHaveValue("side project");
});

test("bad definitions are refused with a reason", async ({ page }) => {
  const dup = await page.request.put("/api/v1/areas", {
    data: { areas: [{ name: "work", color: "blue" }, { name: "Work", color: "red" }] },
  });
  expect(dup.status()).toBe(400);
  expect(await dup.text()).toContain("twice");
  expect((await page.request.put("/api/v1/areas", { data: { areas: [{ name: "none" }] } })).status()).toBe(400);
  expect((await page.request.put("/api/v1/areas", { data: { areas: [{ name: "x", color: "beige" }] } })).status()).toBe(400);
});
