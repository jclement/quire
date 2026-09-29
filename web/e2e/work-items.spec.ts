// Work-item references: with a URL template in Settings, AB#2433 and #2433
// link to the tracker in rendered notes and in task rows; without one they
// are plain text, and a number is never a tag either way.
import { expect, test } from "@playwright/test";

const ADO = "https://dev.azure.com/barreleye/Barreleye/_workitems/edit/{id}";

test("work item numbers link to the tracker once Settings says where", async ({ page }) => {
  await page.request.put("/api/v1/work-items", { data: { url_template: "" } });
  const created = await page.request.post("/api/v1/documents", {
    data: {
      type: "note",
      title: "Environment work",
      markdown: "# Environment work\n\n- [ ] #7433 Auto-update of environments 📅 2026-01-01\n\nTracked in AB#7440.\n",
    },
  });
  const path = (await created.json()).data.path as string;

  // Off: plain text, and not a tag.
  await page.goto(`/doc/${path}`);
  await expect(page.getByText("Tracked in AB#7440.")).toBeVisible();
  await expect(page.getByRole("link", { name: "AB#7440" })).toHaveCount(0);

  await page.goto("/settings");
  await page.getByLabel("Work item URL").fill(ADO);
  await page.getByRole("button", { name: "Save work item URL" }).click();
  await expect(page.getByText("Work item numbers are now links")).toBeVisible();

  await page.goto(`/doc/${path}`);
  await expect(page.getByRole("link", { name: "AB#7440" })).toHaveAttribute(
    "href",
    "https://dev.azure.com/barreleye/Barreleye/_workitems/edit/7440",
  );
  await expect(page.getByRole("link", { name: "#7433" })).toHaveAttribute(
    "href",
    "https://dev.azure.com/barreleye/Barreleye/_workitems/edit/7433",
  );

  // The overdue task's row on Today links it too.
  await page.goto("/today");
  const row = page.getByRole("listitem").filter({ hasText: "Auto-update of environments" });
  await expect(row.getByRole("link", { name: "#7433" })).toHaveAttribute(
    "href",
    "https://dev.azure.com/barreleye/Barreleye/_workitems/edit/7433",
  );

  const tags = (await (await page.request.get("/api/v1/tags")).json()).data as { tag: string }[];
  expect(tags.map((t) => t.tag)).not.toContain("7433");

  await page.request.put("/api/v1/work-items", { data: { url_template: "" } });
});
