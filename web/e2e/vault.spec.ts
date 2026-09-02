// The core loop, in a browser: create a document, edit it, have the edit
// reach disk, find it again by search, and see it linked from elsewhere.
//
// These cover the seam unit tests cannot: the wiring between the SPA, the
// API, and the vault on disk. Two real bugs in quire's history lived exactly
// there — a document created with two frontmatter blocks, and a checkbox
// toggle that updated the file but not the view — and neither was visible to
// a Go test or a component test.
import { expect, test } from "@playwright/test";
import { mkdirSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";

/** Writes straight into the vault on disk, the way vim or Obsidian would. */
function writeOutsideTheApp(rel: string, body: string) {
  const full = join("../tmp/e2e-data/vault", rel);
  mkdirSync(dirname(full), { recursive: true });
  writeFileSync(full, body);
}

test.describe.configure({ mode: "serial" });

test("Today shows a task the moment it exists", async ({ page }) => {
  await page.goto("/");
  // Today's heading is the date itself, not the word "Today".
  await expect(page.getByRole("heading", { level: 1 }).first()).toBeVisible();

  const created = await page.request.post("/api/v1/tasks", {
    data: { text: "Book the dentist", due: "today" },
  });
  expect(created.ok()).toBeTruthy();
  // "today" must be resolved to an ISO date server-side. This test caught
  // it being written to the markdown as the literal word, which produced a
  // task no view could ever surface.
  expect((await created.json()).data.due).toMatch(/^\d{4}-\d{2}-\d{2}$/);

  // Writing a task means writing markdown, then the watcher reindexing it,
  // then Today recomposing — the whole spine. The reindex is debounced, so
  // poll the API for the index catching up before asserting on the view
  // rather than racing it with a single reload.
  await expect
    .poll(
      async () => {
        const res = await page.request.get("/api/v1/tasks?view=today");
        return (await res.text()).includes("Book the dentist");
      },
      { timeout: 15_000 },
    )
    .toBe(true);

  await page.reload();
  // It lands under the "Due today" section, and the empty state cannot be
  // showing once something is due. There is deliberately no assertion that
  // the page *started* empty: other specs share this server and create tasks
  // of their own, so that only held when this spec happened to run first.
  await expect(page.getByRole("heading", { name: /due today/i })).toBeVisible();
  await expect(page.getByText("Book the dentist").first()).toBeVisible();
  await expect(page.getByText("Nothing on the hook")).toHaveCount(0);
});

test("an unresolvable date is refused, not written to the file", async ({ page }) => {
  const res = await page.request.post("/api/v1/tasks", {
    data: { text: "Vague plans", due: "someday" },
  });
  expect(res.status()).toBe(400);
  expect(await res.text()).toContain("VALIDATION_ERROR");
});

test("quick capture works on a phone", async ({ page }) => {
  // The capture button is the mobile toolbar's centre action and does not
  // exist on desktop, so this needs a phone-sized viewport. Mobile capture
  // is a headline feature; it deserves to be exercised at 375px.
  await page.setViewportSize({ width: 375, height: 812 });
  await page.goto("/");

  await page.getByLabel("Quick capture").click();
  await page.getByLabel("New task text").fill("Buy milk");
  await page.keyboard.press("Enter");

  // Capture writes an undated task, so it lands in the inbox rather than
  // today — the "get it out of my head now, triage later" path.
  await expect
    .poll(
      async () => {
        const tasks = await page.request.get("/api/v1/tasks?view=inbox");
        return (await tasks.text()).includes("Buy milk");
      },
      { timeout: 15_000 },
    )
    .toBe(true);
});

test("a document created through the API renders correctly", async ({ page }) => {
  // Seeded through the API so the test is about rendering, not authoring.
  const created = await page.request.post("/api/v1/documents", {
    data: {
      type: "note",
      title: "Rendering Check",
      markdown: [
        "# Rendering Check",
        "",
        "A [[Sarah Chen]] wikilink and a task.",
        "",
        "- [ ] an open task",
        "- [x] a done task",
        "",
        "> [!warning] Careful",
        "> Callouts render as panels.",
        "",
        "```go",
        'fmt.Println("hi")',
        "```",
      ].join("\n"),
    },
  });
  expect(created.status()).toBe(201);
  const { data } = await created.json();

  await page.goto(`/doc/${data.path}`);

  await expect(page.getByRole("heading", { name: "Rendering Check" }).first()).toBeVisible();
  // A document must never render two frontmatter blocks — a real past bug
  // where the second showed up as body text.
  await expect(page.getByText("---").first()).toHaveCount(0);
  // Checkboxes, callouts and highlighted code all render.
  await expect(page.locator('input[type="checkbox"]')).toHaveCount(2);
  // The SPA marks callouts with data-callout (the share renderer uses a
  // .callout class — two pipelines, one syntax).
  await expect(page.locator('[data-callout="warning"]')).toBeVisible();
  await expect(page.getByText("Careful")).toBeVisible();
  await expect(page.getByText('fmt.Println("hi")')).toBeVisible();
});

test("toggling a checkbox updates both the view and the file", async ({ page }) => {
  const created = await page.request.post("/api/v1/documents", {
    data: {
      type: "note",
      title: "Toggle Check",
      markdown: "# Toggle Check\n\n- [ ] flip me\n",
    },
  });
  const { data } = await created.json();
  const path = data.path;

  await page.goto(`/doc/${path}`);
  const checkbox = page.locator('input[type="checkbox"]').first();
  await expect(checkbox).not.toBeChecked();
  await checkbox.click();

  // The view must follow the file, not a stale local buffer — the exact
  // shape of a bug this project has already shipped once.
  await expect(checkbox).toBeChecked();
  await expect
    .poll(async () => {
      const doc = await page.request.get(`/api/v1/documents/${path}`);
      return (await doc.text()).includes("- [x] flip me");
    })
    .toBe(true);
});

test("search finds a document by its content", async ({ page }) => {
  await page.request.post("/api/v1/documents", {
    data: {
      type: "note",
      title: "Zanzibar Trip",
      markdown: "# Zanzibar Trip\n\nSpice tour on the Thursday.\n",
    },
  });

  await page.goto("/search");
  await page.getByLabel("Search query").fill("zanzibar");

  await expect(page.getByText("Zanzibar Trip").first()).toBeVisible();
});

test("an imported document with spaces in its path opens from a link", async ({
  page,
}) => {
  // Vaults imported from Obsidian routinely have "Meeting Notes/2026-08-15
  // Acme Sync.md". Those paths reach the router through the /doc/* splat and
  // must survive the round trip, spaces and all.
  const created = await page.request.post("/api/v1/documents", {
    data: { type: "note", title: "Spaced Out Note", markdown: "# Spaced Out Note\n\nBody here.\n" },
  });
  const { data } = await created.json();

  // Force a path with a space, the way an import would have one.
  const spaced = "notes/A Spaced Path.md";
  await page.request.post("/api/v1/rename", {
    data: { path: data.path, new_path: spaced, rewrite_links: true },
  });

  await page.goto(`/doc/${encodeURI(spaced)}`);
  await expect(page.getByRole("heading", { name: "Spaced Out Note" }).first()).toBeVisible();

  // And reached by clicking, not just by typing the URL — this is where an
  // unencoded href would break.
  await page.goto("/browse/note");
  await page.getByRole("link", { name: /spaced out note/i }).first().click();
  await expect(page.getByRole("heading", { name: "Spaced Out Note" }).first()).toBeVisible();
});

// "You should be able to edit the same Markdown files outside the
// application without breaking the system" is an explicit MVP criterion, and
// it is the whole chain: fsnotify → reindex → SSE → the open page updating.
// Nothing else in the suite exercises it.
test("an edit made outside the app reaches an open page", async ({ page }) => {
  writeOutsideTheApp("notes/external-edit.md", "# External Edit\n\nOriginal body.\n");

  await page.goto("/doc/notes/external-edit.md");
  await expect(page.getByText("Original body.")).toBeVisible();

  // Edit the file underneath the open page — no reload, no interaction.
  writeOutsideTheApp(
    "notes/external-edit.md",
    "# External Edit\n\nRewritten by vim while the page was open.\n",
  );

  await expect(page.getByText("Rewritten by vim while the page was open.")).toBeVisible();
  await expect(page.getByText("Original body.")).toHaveCount(0);
});

test("a document created outside the app appears in browse", async ({ page }) => {
  await page.goto("/browse/note");
  writeOutsideTheApp("notes/appeared-from-nowhere.md", "# Appeared From Nowhere\n\nHello.\n");

  await expect(
    page.getByRole("link", { name: /appeared from nowhere/i }),
  ).toBeVisible();
});

test("wikilinks produce a backlink on the target", async ({ page }) => {
  await page.request.post("/api/v1/documents", {
    data: { type: "person", title: "Sarah Chen", markdown: "# Sarah Chen\n" },
  });
  await page.request.post("/api/v1/documents", {
    data: {
      type: "meeting",
      title: "Quarterly Review",
      markdown: "# Quarterly Review\n\nMet [[Sarah Chen]] about reporting.\n",
    },
  });

  await page.goto("/doc/people/sarah-chen.md");
  // Backlinks are always shown in the rail — the relationship model is the
  // point of the app, so a missing backlink is a product bug.
  const backlinks = page.getByLabel("Backlinks");
  await expect(backlinks).toBeVisible();
  await expect(backlinks.getByText("Quarterly Review")).toBeVisible();
});
