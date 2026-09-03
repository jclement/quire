// The app's time zone: a fresh install adopts the first browser's zone, and
// Settings can change it; every date in the app is reckoned in it.
import { expect, test } from "@playwright/test";

test.use({ timezoneId: "Pacific/Auckland" });

test("a fresh install takes the browser's zone; Settings can change it", async ({ page }) => {
  // Start unset, whatever earlier specs did.
  await page.request.put("/api/v1/timezone", { data: { timezone: "" } });
  await page.goto("/");
  await expect
    .poll(async () => (await (await page.request.get("/api/v1/timezone")).json()).data.timezone)
    .toBe("Pacific/Auckland");

  await page.goto("/settings");
  const zone = page.getByLabel("Time zone");
  await expect(zone).toHaveValue("Pacific/Auckland");
  await expect(page.getByText("(Pacific/Auckland)")).toBeVisible();

  await zone.fill("America/Edmonton");
  await page.getByRole("button", { name: "Save time zone" }).click();
  await expect(page.getByText("Time zone: America/Edmonton")).toBeVisible();
  const info = (await (await page.request.get("/api/v1/timezone")).json()).data;
  expect(info.effective).toBe("America/Edmonton");
  expect(info.now).toMatch(/-0[67]:00$/);

  const bad = await page.request.put("/api/v1/timezone", { data: { timezone: "Mars/Olympus" } });
  expect(bad.status()).toBe(400);
  // Leave the suite in the browser's zone.
  await page.request.put("/api/v1/timezone", { data: { timezone: "" } });
});
