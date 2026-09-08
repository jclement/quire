// A note changing underneath you: an agent writing through MCP, vim, git
// pulling, another tab. The index watcher notices, the server announces it
// on /api/v1/events, and the open page has to agree with the file.
import { expect, test, type Page } from "@playwright/test";

async function createDoc(page: Page, title: string, markdown: string) {
  const res = await page.request.post("/api/v1/documents", {
    data: { type: "note", title, markdown },
  });
  return (await res.json()).data.path as string;
}

async function diskText(page: Page, path: string) {
  const res = await page.request.get(`/api/v1/documents/${path}`);
  return (await res.json()).data.markdown as string;
}

/** Another process writes the file, the way an agent or an editor would. */
async function writeElsewhere(page: Page, path: string, markdown: string) {
  const doc = (await (await page.request.get(`/api/v1/documents/${path}`)).json()).data;
  const res = await page.request.put(`/api/v1/documents/${path}`, {
    data: { markdown, base_sha256: doc.sha256 },
  });
  expect(res.ok()).toBeTruthy();
}

test("read mode follows the file", async ({ page }) => {
  const path = await createDoc(page, "Reload Read", "# Reload Read\n\noriginal body\n");
  await page.goto(`/doc/${path}`);
  await expect(page.getByText("original body")).toBeVisible();

  await writeElsewhere(page, path, "# Reload Read\n\nrewritten elsewhere\n");
  await expect(page.getByText("rewritten elsewhere")).toBeVisible();
  await expect(page.getByText("original body")).toHaveCount(0);
});

test("an untouched editor takes the change instead of overwriting it", async ({ page }) => {
  const path = await createDoc(page, "Reload Edit", "# Reload Edit\n\nline one\n");
  await page.goto(`/doc/${path}?edit=true`);
  const editor = page.locator(".cm-content");
  await expect(editor).toContainText("line one");

  // Nothing typed yet, so there is nothing of ours to lose.
  await writeElsewhere(page, path, "# Reload Edit\n\nline one\ntheir new line\n");
  await expect(editor).toContainText("their new line");

  // Typing on top of it and saving keeps both — the buffer was rebased on
  // their version rather than left stale and written over it.
  await editor.click();
  await page.keyboard.press("ControlOrMeta+End");
  await page.keyboard.type("\nmy line");
  await page.keyboard.press("ControlOrMeta+s");
  await expect.poll(() => diskText(page, path)).toContain("my line");
  const after = await diskText(page, path);
  expect(after).toContain("their new line");
  expect(after).toContain("line one");
});

test("a dirty editor is never overwritten, and says so", async ({ page }) => {
  const path = await createDoc(page, "Reload Dirty", "# Reload Dirty\n\nline one\n");
  await page.goto(`/doc/${path}?edit=true`);
  const editor = page.locator(".cm-content");
  await expect(editor).toContainText("line one");

  // Unsaved work first: now the buffer must not be touched from underneath.
  await editor.click();
  await page.keyboard.press("ControlOrMeta+End");
  await page.keyboard.type("\nmy unsaved work");
  await writeElsewhere(page, path, "# Reload Dirty\n\nline one\ntheirs\n");
  await page.waitForTimeout(500);
  await expect(editor).toContainText("my unsaved work");
  await expect(editor).not.toContainText("theirs");

  // Saving is a conflict rather than a clobber, and theirs is still on disk.
  await page.keyboard.press("ControlOrMeta+s");
  await expect(page.getByText(/changed on disk/i)).toBeVisible();
  expect(await diskText(page, path)).toContain("theirs");
  await expect(page.getByRole("button", { name: "Keep mine" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Take disk" })).toBeVisible();
});

test("frontmatter changed elsewhere reaches the strip without touching the body", async ({ page }) => {
  const path = await createDoc(page, "Reload Meta", "# Reload Meta\n\nbody stays\n");
  await page.goto(`/doc/${path}?edit=true`);
  const editor = page.locator(".cm-content");
  await expect(editor).toContainText("body stays");

  // An agent tagging the note: frontmatter only, which the editor never shows.
  const res = await page.request.patch(`/api/v1/documents/${path}`, {
    data: { set: { tags: ["from-an-agent"] } },
  });
  expect(res.ok()).toBeTruthy();

  await expect(page.getByRole("link", { name: "from-an-agent" })).toBeVisible();
  await expect(editor).toContainText("body stays");
  await expect(editor).not.toContainText("tags:");
});
