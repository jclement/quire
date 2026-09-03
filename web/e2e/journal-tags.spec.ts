// The journal (every daily note on one scrolling page), tags (a browse
// page, clickable chips, and #tags in prose linking to search), and the
// agent-activity section in Settings.
import { expect, test } from "@playwright/test";
import { mkdirSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";

test.describe.configure({ mode: "serial" });

function writeVault(rel: string, body: string) {
  const full = join("../tmp/e2e-data/vault", rel);
  mkdirSync(dirname(full), { recursive: true });
  writeFileSync(full, body);
}

// Local calendar date, never toISOString(): that is the UTC date, which is
// tomorrow every evening west of Greenwich — and the app reckons in the
// browser's zone.
const iso = (d: Date) =>
  `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
const daysAgo = (n: number) => {
  const d = new Date();
  d.setDate(d.getDate() - n);
  return iso(d);
};

test("the journal shows today first, then history, newest first", async ({ page }) => {
  // A run of past days written straight into the vault.
  for (const n of [1, 2, 3]) {
    writeVault(`daily/${daysAgo(n)}.md`, `# ${daysAgo(n)}\n\nEntry from ${n} day(s) ago.\n`);
  }
  await page.request.post(`/api/v1/daily/${daysAgo(0)}`);
  // Let the watcher index the files first: this test is about order, and
  // live updating after an external write is covered by the toggle test.
  await expect
    .poll(async () => ((await (await page.request.get("/api/v1/daily?limit=50")).json()).data as unknown[]).length, {
      timeout: 15_000,
    })
    .toBeGreaterThanOrEqual(3);

  await page.goto("/journal");
  await expect(page.getByRole("heading", { name: "Journal" })).toBeVisible();
  await expect(page.getByText("Entry from 3 day(s) ago.")).toBeVisible();
  const entries = await page.locator("section[aria-label]").allInnerTexts();
  const oneIdx = entries.findIndex((t) => t.includes("Entry from 1 day"));
  const threeIdx = entries.findIndex((t) => t.includes("Entry from 3 day"));
  expect(oneIdx).toBeGreaterThanOrEqual(0);
  expect(oneIdx).toBeLessThan(threeIdx);
  // Today is on top, marked.
  expect(entries[0]!.toLowerCase()).toContain("today");
});

test("a task toggled in the journal reaches the file", async ({ page }) => {
  const day = daysAgo(1);
  writeVault(`daily/${day}.md`, `# ${day}\n\n- [ ] journal toggle\n`);
  // The journal reads documents from disk, but a toggle resolves the task
  // id through the index, which trails the watcher by its debounce. Wait
  // for the index to know the task before clicking, or the click lands in
  // that window and the toggle 404s.
  await expect
    .poll(async () => (await page.request.get("/api/v1/tasks?view=inbox")).text(), { timeout: 15_000 })
    .toContain("journal toggle");
  await page.goto("/journal");
  const box = page.locator("section[aria-label]").filter({ hasText: "journal toggle" }).locator('input[type="checkbox"]').first();
  await expect(box).toBeVisible({ timeout: 15_000 });
  await box.click();
  await expect
    .poll(async () => (await page.request.get(`/api/v1/documents/daily/${day}.md`)).text())
    .toContain("- [x] journal toggle");
});

test("tags page lists every tag with counts and links to search", async ({ page }) => {
  await page.request.post("/api/v1/documents", {
    data: { type: "note", title: "Tagged One", markdown: "# Tagged One\n\nAbout #quarterly-review and #hiring.\n" },
  });
  await page.request.post("/api/v1/documents", {
    data: { type: "note", title: "Tagged Two", markdown: "---\ntags: [hiring]\n---\n# Tagged Two\n\nMore.\n" },
  });

  await page.goto("/tags");
  const hiring = page.getByRole("link", { name: /#hiring/ });
  await expect(hiring).toBeVisible({ timeout: 15_000 });
  await expect(hiring).toContainText("2"); // body tag + frontmatter tag

  await hiring.click();
  await expect(page).toHaveURL(/q=tag%3Ahiring|q=tag:hiring/);
  await expect(page.getByText("Tagged One").first()).toBeVisible();
  await expect(page.getByText("Tagged Two").first()).toBeVisible();
});

test("a #tag in prose is a link to the tag search", async ({ page }) => {
  const res = await page.request.post("/api/v1/documents", {
    data: {
      type: "note",
      title: "Prose Tags",
      markdown: "# Prose Tags\n\nDiscussed #roadmap with the team. Issue #123 is not a tag. `#code` is not either.\n",
    },
  });
  const { data } = await res.json();
  await page.goto(`/doc/${data.path}`);

  const link = page.getByRole("link", { name: "#roadmap" });
  await expect(link).toBeVisible();
  await expect(page.getByRole("link", { name: "#123" })).toHaveCount(0);
  await expect(page.getByRole("link", { name: "#code" })).toHaveCount(0);

  await link.click();
  await expect(page).toHaveURL(/tag(%3A|:)roadmap/);
});

test("browse rows show tags as links", async ({ page }) => {
  await page.goto("/browse/note");
  const chip = page.getByRole("link", { name: "#roadmap" }).first();
  await expect(chip).toBeVisible();
  await chip.click();
  await expect(page).toHaveURL(/tag(%3A|:)roadmap/);
});

test("settings shows the agent activity section", async ({ page }) => {
  await page.goto("/settings");
  await expect(page.getByRole("heading", { name: /agent activity/i })).toBeVisible();
  // Under auth-none every caller is the owner, so nothing is audited here;
  // the recording itself is covered in Go.
  await expect(page.getByText("No agent activity yet.")).toBeVisible();
});

test("MCP advertises the new tools with annotations", async ({ request }) => {
  const init = await request.post("/mcp", {
    headers: { "content-type": "application/json", accept: "application/json, text/event-stream" },
    data: { jsonrpc: "2.0", id: 1, method: "initialize", params: { protocolVersion: "2025-06-18", capabilities: {}, clientInfo: { name: "e2e", version: "1" } } },
  });
  const sid = init.headers()["mcp-session-id"];
  expect(sid).toBeTruthy();
  const h = { "content-type": "application/json", accept: "application/json, text/event-stream", "mcp-session-id": sid! };
  await request.post("/mcp", { headers: h, data: { jsonrpc: "2.0", method: "notifications/initialized" } });
  const list = await request.post("/mcp", { headers: h, data: { jsonrpc: "2.0", id: 2, method: "tools/list" } });
  const body = await list.text();
  const json = JSON.parse(body.split("data: ").pop()!.trim());
  const tools = json.result.tools as Array<{ name: string; annotations?: Record<string, unknown> }>;
  const names = tools.map((t) => t.name);
  for (const expected of ["list_documents", "list_tags", "get_daily", "edit_task", "link_entity", "set_frontmatter"]) {
    expect(names, `missing ${expected}`).toContain(expected);
  }
  expect(tools.find((t) => t.name === "search")?.annotations?.readOnlyHint).toBe(true);
  expect(tools.find((t) => t.name === "update_document")?.annotations?.destructiveHint).toBe(true);
});
