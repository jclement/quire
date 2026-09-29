// The decision log: bullets under a meeting's "Decisions" heading and
// documents tagged decision, collected newest first, filterable, and shown
// on the rail of the people and projects they are about.
import { expect, test, type Page } from "@playwright/test";

async function create(page: Page, type: string, title: string, markdown?: string) {
  const res = await page.request.post("/api/v1/documents", {
    data: { type, title, ...(markdown ? { markdown } : {}) },
  });
  expect(res.ok()).toBeTruthy();
  return (await res.json()).data.path as string;
}

test("decisions from meetings and records are logged, filtered and shown on the project", async ({
  page,
}) => {
  const project = await create(page, "project", "Heliograph");
  await create(page, "person", "Wren Castellan");
  await create(
    page,
    "meeting",
    "Heliograph kickoff",
    "---\ndate: 2026-09-10\npeople: [\"[[Wren Castellan]]\"]\nproject: \"[[Heliograph]]\"\n---\n" +
      "# Heliograph kickoff\n\n## Notes\n\n- long discussion\n\n## Decisions\n\n" +
      "- Launch behind a feature flag\n- Budget capped at 40k\n\n## Action items\n\n- [ ] Draft the flag plan\n",
  );
  await create(
    page,
    "note",
    "Heliograph uses Postgres",
    "---\ntags: [decision]\ndate: 2026-09-12\nproject: \"[[Heliograph]]\"\n---\n" +
      "# Heliograph uses Postgres\n\n## Decision\n\n- Postgres, not SQLite\n",
  );

  await page.goto("/decisions");
  await expect(page.getByRole("heading", { name: "Decisions", level: 1 })).toBeVisible();
  const flag = page.getByRole("listitem").filter({ hasText: "Launch behind a feature flag" });
  await expect(flag).toContainText("2026-09-10");
  await expect(flag.getByRole("link", { name: "Heliograph kickoff" })).toBeVisible();
  await expect(flag.getByRole("link", { name: "Wren Castellan" })).toBeVisible();
  await expect(flag.getByRole("link", { name: "Heliograph", exact: true })).toBeVisible();
  // The record is one row, titled by the document — its own "## Decision"
  // section is not listed a second time.
  await expect(page.getByRole("link", { name: "Heliograph uses Postgres" })).toHaveCount(1);
  await expect(page.getByText("Postgres, not SQLite")).toHaveCount(0);
  // Newest first: the record (09-12) sits above the meeting's bullets (09-10).
  const rows = page.getByRole("listitem").filter({ hasText: /Heliograph/ });
  await expect(rows.first()).toContainText("Heliograph uses Postgres");

  await page.getByLabel("Filter decisions").fill("budget");
  await expect(page.getByRole("listitem").filter({ hasText: "Budget capped at 40k" })).toBeVisible();
  await expect(flag).toHaveCount(0);

  // The project page's rail carries its decisions.
  await page.goto(`/doc/${project}`);
  const rail = page.getByRole("navigation", { name: "Decisions" });
  await expect(rail).toContainText("Launch behind a feature flag");
  await expect(rail).toContainText("Heliograph uses Postgres");
});
