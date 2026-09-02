// Semantic search against the fake embeddings endpoint: the toggle exists
// only because the server says it can, results rank by meaning (well —
// shared words, which is what the fake understands), the URL carries the
// mode, and a document's rail shows what else is near it.
import { expect, test, type Page } from "@playwright/test";

test.describe.configure({ mode: "serial" });

async function createDoc(page: Page, title: string, markdown: string) {
  const res = await page.request.post("/api/v1/documents", {
    data: { type: "note", title, markdown },
  });
  const { data } = await res.json();
  return data.path as string;
}

/** Waits for the embedder to have nothing pending. */
async function settled(page: Page) {
  await expect
    .poll(async () => {
      const res = await page.request.get("/api/v1/semantic/status");
      const { data } = await res.json();
      return data.enabled && data.pending === 0 ? data.documents : -1;
    })
    .toBeGreaterThan(0);
}

test("the health endpoint advertises semantic search and Settings shows it", async ({
  page,
}) => {
  const health = await (await page.request.get("/api/v1/health")).json();
  expect(health.data.semantic_search).toBe(true);
  await page.goto("/settings");
  await expect(page.getByRole("heading", { name: "Semantic search" })).toBeVisible();
  await expect(page.getByText("text-embedding-3-small")).toBeVisible();
});

test("semantic mode ranks by meaning and survives reload; text mode is exact", async ({
  page,
}) => {
  const rollout = await createDoc(
    page,
    "Cluster rollout",
    "# Cluster rollout\n\nThe kubernetes cluster rollout waits on the ingress upgrade landing, with the platform team on call for the cutover window.\n",
  );
  await createDoc(page, "Ingress upgrade", "# Ingress upgrade\n\nDrain the kubernetes cluster before the ingress upgrade so the rollout has no traffic to disturb during the cutover window.\n");
  await createDoc(page, "Lunch plans", "# Lunch plans\n\nTacos on Thursday with the platform team.\n");
  await settled(page);

  await page.goto("/search");
  const toggle = page.getByRole("switch", { name: "Semantic search" });
  await expect(toggle).toHaveAttribute("aria-checked", "false");
  await page.getByLabel("Search query").fill("kubernetes rollout");

  // Exact mode: only documents containing the words.
  const list = page.getByRole("list").filter({ has: page.getByText("Cluster rollout") });
  await expect(list.getByRole("listitem").first()).toContainText("Cluster rollout");

  await toggle.click();
  await expect(toggle).toHaveAttribute("aria-checked", "true");
  await expect(page).toHaveURL(/mode=semantic/);
  const rows = page.getByRole("listitem");
  await expect(rows.first()).toContainText("Cluster rollout");
  // Everything is ranked in semantic mode — even lunch comes back, last.
  await expect(rows.filter({ hasText: "Lunch plans" })).toHaveCount(1);
  const titles = await rows.allInnerTexts();
  expect(titles.findIndex((t) => t.includes("Lunch plans"))).toBeGreaterThan(
    titles.findIndex((t) => t.includes("Ingress upgrade")),
  );

  await page.reload();
  await expect(page.getByRole("switch", { name: "Semantic search" })).toHaveAttribute(
    "aria-checked",
    "true",
  );

  // The document rail lists what is near this note.
  await page.setViewportSize({ width: 1400, height: 900 });
  await page.goto(`/doc/${rollout}`);
  const related = page.getByRole("navigation", { name: "Related documents" });
  await expect(related).toBeVisible();
  await expect(related.getByRole("link").first()).toContainText("Ingress upgrade");
});
