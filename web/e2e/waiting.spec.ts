// Waiting-for: grouped by who, oldest first, stale waits flagged, and the
// person's own page leading with what they owe.
import { expect, test } from "@playwright/test";

test("the Waiting view groups by who and flags stale waits", async ({ page }) => {
  await page.request.post("/api/v1/documents", {
    data: { type: "person", title: "Wendy Waitwell" },
  });
  // An old wait written by hand, and a fresh one delegated through the API.
  await page.request.post("/api/v1/documents", {
    data: {
      type: "note",
      title: "Waiting Fixture",
      markdown: "# Waiting Fixture\n\n- [ ] waiting-old SOC evidence from [[Wendy Waitwell]] ⏳ 2026-01-05\n",
    },
  });
  const fresh = await page.request.post("/api/v1/tasks", {
    data: { text: "waiting-new pen test report", waiting_on: "Wendy Waitwell" },
  });
  expect(fresh.ok()).toBeTruthy();

  await page.goto("/tasks/waiting");
  const group = page.locator("section").filter({ has: page.getByRole("heading", { name: /Wendy Waitwell/ }) });
  await expect(group).toBeVisible();
  const rows = group.getByRole("listitem");
  // Oldest first.
  await expect(rows.nth(0)).toContainText("waiting-old SOC evidence");
  await expect(rows.nth(1)).toContainText("waiting-new pen test report");
  // Ages: the old one is stale (red, with a chase hint), the new one 0 days.
  await expect(rows.nth(0).getByTitle(/time to chase/)).toContainText(/⏳ \d+d/);
  await expect(rows.nth(1)).toContainText("⏳ 0d");

  // The person page's rail leads with what they owe.
  await page.goto("/doc/people/wendy-waitwell.md");
  const rail = page.getByRole("navigation", { name: "Waiting on them" });
  await expect(rail).toContainText("waiting-old SOC evidence");
  await expect(rail).toContainText("waiting-new pen test report");
});
