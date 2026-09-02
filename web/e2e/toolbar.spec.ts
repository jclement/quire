// The editor toolbar and the live properties strip: buttons rewrite the
// cursor line, the task details form edits the emoji grammar, and a tag
// added from the strip while editing lands in the buffer without losing
// what was typed.
import { expect, test, type Page } from "@playwright/test";

test.describe.configure({ mode: "serial" });

async function openInEditor(page: Page, title: string, markdown: string) {
  const res = await page.request.post("/api/v1/documents", {
    data: { type: "note", title, markdown },
  });
  const { data } = await res.json();
  await page.goto(`/doc/${data.path}`);
  await expect(page.getByRole("heading", { level: 1 }).first()).toBeVisible();
  await page.keyboard.press("e");
  const editor = page.locator(".cm-content");
  await expect(editor).toBeVisible();
  return { path: data.path as string, editor };
}

async function diskText(page: Page, path: string) {
  const res = await page.request.get(`/api/v1/documents/${path}`);
  const { data } = await res.json();
  return data.markdown as string;
}

const toolbar = (page: Page) => page.getByRole("toolbar", { name: "Editor tools" });

test("heading, task, details and callout act on the cursor line", async ({ page }) => {
  const { path, editor } = await openInEditor(page, "Toolbar Doc", "# Toolbar Doc\n\ncall sarah\n\nsome prose\n");
  await editor.getByText("call sarah").click();

  await toolbar(page).getByRole("button", { name: "Heading" }).click();
  await page.getByRole("menuitem", { name: "Heading 2" }).click();
  await expect(editor).toContainText("## call sarah");
  await expect(toolbar(page).getByRole("button", { name: "Heading" })).toContainText("H2");
  await toolbar(page).getByRole("button", { name: "Heading" }).click();
  await page.getByRole("menuitem", { name: "Plain text" }).click();

  await toolbar(page).getByRole("button", { name: "Task", exact: true }).click();
  await expect(editor).toContainText("- [ ] call sarah");
  await expect(toolbar(page).getByRole("button", { name: "Task", exact: true })).toBeDisabled();

  await toolbar(page).getByRole("button", { name: "Task details" }).click();
  const details = page.getByRole("dialog", { name: "Task details" });
  await details.getByLabel("Due").fill("2026-09-10");
  await details.getByLabel("Priority").selectOption("1");
  await details.getByLabel("Waiting").check();
  await details.getByLabel("Repeat").fill("every week");
  await details.getByRole("button", { name: "Apply" }).click();
  await expect(editor).toContainText("- [ ] call sarah ⏫ 📅 2026-09-10 ⏳ 🔁 every week");

  // Reopening shows what is on the line.
  await toolbar(page).getByRole("button", { name: "Task details" }).click();
  await expect(page.getByRole("dialog", { name: "Task details" }).getByLabel("Due")).toHaveValue("2026-09-10");
  await page.keyboard.press("Escape");

  await editor.getByText("some prose").click();
  await toolbar(page).getByRole("button", { name: "Callout" }).click();
  await page.getByRole("menuitem", { name: "Warning" }).click();
  await expect(editor).toContainText("> [!warning]");
  await expect(toolbar(page).getByRole("button", { name: "Callout" })).toContainText("Warning");

  await page.keyboard.press("ControlOrMeta+s");
  await expect.poll(() => diskText(page, path)).toContain("> [!warning]\n> some prose");
});

test("Table inserts a starter table and Drawing inserts an embed", async ({ page }) => {
  const { path, editor } = await openInEditor(page, "Toolbar Insert", "# Toolbar Insert\n\nintro\n");
  await editor.getByText("intro").click();
  await toolbar(page).getByRole("button", { name: "Table", exact: true }).click();
  await expect(editor).toContainText("| Column | Column |");
  await expect(toolbar(page).getByRole("button", { name: "Reformat table" })).toBeEnabled();

  await toolbar(page).getByRole("button", { name: "Drawing" }).click();
  await expect(page.getByRole("dialog", { name: "Drawing" })).toBeVisible();
  await page.getByRole("button", { name: "Cancel" }).click();
  await expect(editor).toContainText("![Drawing](attachments/");
  await page.keyboard.press("ControlOrMeta+s");
  await expect.poll(() => diskText(page, path)).toContain("| Column | Column |");
});

test("the properties strip stays live while editing", async ({ page }) => {
  await page.request.put("/api/v1/areas", {
    data: { areas: [{ name: "work", color: "blue" }, { name: "personal", color: "green" }] },
  });
  const { path, editor } = await openInEditor(page, "Live Strip", "# Live Strip\n\nbody\n");
  await editor.click();
  await page.keyboard.press("ControlOrMeta+End");
  await page.keyboard.type("\nunsaved words");

  await page.getByRole("button", { name: "Add tag to this document" }).click();
  await page.getByLabel("Search tags").fill("live");
  await page.keyboard.press("Enter");
  // The buffer gains the frontmatter and keeps the words typed before it.
  await expect(editor).toContainText("tags: [live]");
  await expect(editor).toContainText("unsaved words");

  await page.getByRole("button", { name: "Document area" }).click();
  await page.getByRole("listbox", { name: "Choose area" }).getByRole("option", { name: "Work", exact: true }).click();
  await expect(editor).toContainText("area: work");
  await expect(page.getByRole("button", { name: "Document area" })).toContainText("work");

  const text = await diskText(page, path);
  expect(text).toContain("tags: [live]");
  expect(text).toContain("area: work");
  expect(text).toContain("unsaved words");
});
