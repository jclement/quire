// Two writers, one file. quire never auto-merges (DESIGN.md): a save whose
// base sha is stale gets a 409, autosave freezes, and the user picks
// keep-mine or take-disk explicitly.
//
// This is the path the save machinery is most easily broken by, and the one
// where getting it wrong destroys someone's writing rather than merely
// showing something stale.
import { expect, test } from "@playwright/test";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";

const VAULT = "../tmp/e2e-data/vault";

function writeOnDisk(rel: string, body: string) {
  const full = join(VAULT, rel);
  mkdirSync(dirname(full), { recursive: true });
  writeFileSync(full, body);
}
const readOnDisk = (rel: string) => readFileSync(join(VAULT, rel), "utf8");

test.describe.configure({ mode: "serial" });

/** Opens the editor on a fresh doc and types into it. */
async function editorWith(page: import("@playwright/test").Page, rel: string, typed: string) {
  await page.goto(`/doc/${rel}`);
  // A file written a moment ago is indexed asynchronously; wait for the
  // document to render before trying to edit it.
  await expect(page.getByRole("heading", { level: 1 }).first()).toBeVisible();
  await page.keyboard.press("e"); // read → edit
  const editor = page.locator(".cm-content");
  await expect(editor).toBeVisible();
  await editor.click();
  await page.keyboard.press("ControlOrMeta+a");
  await page.keyboard.type(typed);
  return editor;
}

test("keep mine overwrites the disk version", async ({ page }) => {
  const rel = "notes/conflict-keep.md";
  writeOnDisk(rel, "# Conflict Keep\n\nbase\n");
  await page.goto(`/doc/${rel}`);
  await expect(page.getByText("base")).toBeVisible();

  const editor = await editorWith(page, rel, "# Conflict Keep\n\nmine\n");
  await expect(editor).toContainText("mine");

  // Someone else rewrites the file while the editor holds unsaved text.
  writeOnDisk(rel, "# Conflict Keep\n\ntheirs\n");
  await page.waitForTimeout(1200); // let the watcher publish

  await page.keyboard.press("ControlOrMeta+s");
  await expect(page.getByText("This file changed on disk while you were editing.")).toBeVisible();

  // Clicking the button blurs the editor. That used to fire an ordinary save
  // carrying the same stale base sha, which conflicted again and swallowed
  // the choice — so this click is the regression, not just a step.
  await page.getByRole("button", { name: "Keep mine" }).click();
  await expect(page.getByText("This file changed on disk while you were editing.")).toHaveCount(0);

  await expect.poll(() => readOnDisk(rel)).toContain("mine");
});

test("take disk abandons the local edit", async ({ page }) => {
  const rel = "notes/conflict-take.md";
  writeOnDisk(rel, "# Conflict Take\n\nbase\n");

  await editorWith(page, rel, "# Conflict Take\n\nmine\n");
  writeOnDisk(rel, "# Conflict Take\n\ntheirs\n");
  await page.waitForTimeout(1200);

  await page.keyboard.press("ControlOrMeta+s");
  await expect(page.getByText("This file changed on disk while you were editing.")).toBeVisible();

  await page.getByRole("button", { name: "Take disk" }).click();
  await expect(page.getByText("This file changed on disk while you were editing.")).toHaveCount(0);

  // The disk version wins, and the editor now holds it — so a later save
  // cannot resurrect the abandoned text.
  await expect.poll(() => readOnDisk(rel)).toContain("theirs");
  await expect(page.locator(".cm-content")).toContainText("theirs");
});
