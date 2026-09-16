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

/** How many documents the embedder has finished, or -1 while it still works. */
async function embedded(page: Page) {
  const res = await page.request.get("/api/v1/semantic/status");
  const { data } = await res.json();
  return data.enabled && data.pending === 0 ? (data.documents as number) : -1;
}

/** The embedded count once the queue is quiet — the baseline to measure against. */
async function quiet(page: Page) {
  await expect.poll(() => embedded(page)).toBeGreaterThanOrEqual(0);
  return await embedded(page);
}

/** Waits until `want` more documents than `baseline` have been embedded.
 *  Waiting on `pending === 0` alone is not a barrier: it is briefly true
 *  after a create and before the embedder has picked the document up, so
 *  the search that follows raced a half-filled index — sometimes missing a
 *  document, sometimes catching the list mid-render. */
async function settled(page: Page, baseline: number, want: number) {
  await expect.poll(() => embedded(page)).toBeGreaterThanOrEqual(baseline + want);
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
  // A word no other spec's document contains, in all three seeded notes.
  // The fake embedder is bag-of-words, so this lifts exactly these three
  // clear of the ~100 documents the rest of the suite leaves in the vault,
  // which is what makes the ranking assertion below independent of
  // whatever ran before it.
  const nonce = `zq${Date.now().toString(36)}`;
  const baseline = await quiet(page);
  const rollout = await createDoc(
    page,
    "Cluster rollout",
    `# Cluster rollout\n\nThe kubernetes cluster rollout waits on the ingress upgrade landing, with the platform team on call for the cutover window. ${nonce}\n`,
  );
  const ingress = await createDoc(page, "Ingress upgrade", `# Ingress upgrade\n\nDrain the kubernetes cluster before the ingress upgrade so the rollout has no traffic to disturb during the cutover window. ${nonce}\n`);
  const lunch = await createDoc(page, "Lunch plans", `# Lunch plans\n\nTacos on Thursday with the platform team. ${nonce}\n`);
  await settled(page, baseline, 3);

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
  // Scoped to the results list, as the exact-mode half above already is:
  // an unscoped listitem query also matches the page's other lists.
  const rows = list.getByRole("listitem");
  await expect(rows.first()).toContainText("Cluster rollout");
  // "Even lunch comes back, last" is a claim about ORDER, not about the
  // page's first screenful: semantic mode ranks the whole vault, and by the
  // time this spec runs that vault holds a hundred documents, so the
  // deliberate worst match sits far below the result limit. Ask the API,
  // with the nonce narrowing the field to the three seeded notes.
  const ranked = await (
    await page.request.get(
      `/api/v1/search?mode=semantic&limit=100&q=${encodeURIComponent(`${nonce} kubernetes rollout`)}`,
    )
  ).json();
  const order = (ranked.data as { path: string }[]).map((r) => r.path);
  expect(order).toContain(lunch);
  expect(order.indexOf(lunch)).toBeGreaterThan(order.indexOf(ingress));

  await page.reload();
  await expect(page.getByRole("switch", { name: "Semantic search" })).toHaveAttribute(
    "aria-checked",
    "true",
  );

  // The document rail lists what is near this note.
  await page.setViewportSize({ width: 1400, height: 900 });
  await page.goto(`/doc/${rollout}`);
  const related = page.getByRole("navigation", { name: "Similar documents" });
  await expect(related).toBeVisible();
  await expect(related.getByRole("link").first()).toContainText("Ingress upgrade");
});
