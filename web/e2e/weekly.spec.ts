// The weekly tier: a review composed from the index, and the week's own
// note created on demand.
import { expect, test, type Page } from "@playwright/test";

/** The ISO week label for a date, the way the server writes it. */
function isoWeek(d: Date): string {
  const t = new Date(Date.UTC(d.getFullYear(), d.getMonth(), d.getDate()));
  const day = t.getUTCDay() || 7;
  t.setUTCDate(t.getUTCDate() + 4 - day);
  const yearStart = new Date(Date.UTC(t.getUTCFullYear(), 0, 1));
  const week = Math.ceil(((t.getTime() - yearStart.getTime()) / 86400000 + 1) / 7);
  return `${t.getUTCFullYear()}-W${String(week).padStart(2, "0")}`;
}

async function createDoc(page: Page, type: string, title: string, markdown: string) {
  const res = await page.request.post("/api/v1/documents", {
    data: { type, title, markdown },
  });
  return (await res.json()).data.path as string;
}

test("the week composes what landed, what slipped and what has gone quiet", async ({ page }) => {
  const today = new Date();
  const iso = (d: Date) =>
    `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
  await createDoc(
    page,
    "note",
    "Week Work",
    `# Week Work\n\n- [x] landed in the week ✅ ${iso(today)}\n- [ ] slipped away 📅 2026-01-05\n`,
  );
  await createDoc(page, "project", "Silent Project", "---\nstatus: active\n---\n# Silent Project\n");
  await expect
    .poll(async () => {
      const res = await page.request.get("/api/v1/weekly/this");
      const { data } = await res.json();
      return (data.completed as { text: string }[]).map((t) => t.text).join(",");
    })
    .toContain("landed in the week");

  await page.goto("/weekly");
  // The week's own note repeats the label as its H1, so anchor on the first.
  await expect(page.getByRole("heading", { name: isoWeek(today) }).first()).toBeVisible();
  await expect(page.getByRole("region", { name: "Landed" })).toContainText("landed in the week");
  await expect(page.getByRole("region", { name: "Slipped" })).toContainText("slipped away");
  const stalled = page.getByRole("region", { name: "Projects with no next action" });
  await expect(stalled).toContainText("Silent Project");

  // The week's own note is created on demand and opens for writing.
  await page.getByRole("button", { name: /Start the note for/ }).click();
  await expect(page).toHaveURL(new RegExp(`weekly/${isoWeek(today)}`));
  await expect(page.locator(".cm-content")).toBeVisible();
  const doc = await (
    await page.request.get(`/api/v1/documents/weekly/${isoWeek(today)}.md`)
  ).json();
  expect(doc.data.type).toBe("weekly");

  // Once written, the review carries it rather than offering to start it.
  await page.goto("/weekly");
  await expect(page.getByRole("button", { name: /Start the note for/ })).toHaveCount(0);
});

test("weeks step backwards and forwards", async ({ page }) => {
  await page.goto("/weekly/2026-W36");
  await expect(page.getByRole("heading", { name: "2026-W36" }).first()).toBeVisible();
  await expect(page.getByText("2026-08-31 → 2026-09-06")).toBeVisible();
  await page.getByRole("link", { name: "Week 2026-W35" }).click();
  await expect(page).toHaveURL(/weekly\/2026-W35/);
  await expect(page.getByText("2026-08-24 → 2026-08-30")).toBeVisible();
  const bad = await page.request.get("/api/v1/weekly/2026-W99");
  expect(bad.status()).toBe(400);
});
