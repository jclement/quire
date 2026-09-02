// The visual table editor: a grid over a GFM table, reachable from read
// mode (writes straight to disk) and from the editor's table panel (writes
// into the buffer). The thing it exists for is escaping — a pipe typed into
// a cell must come out as `\|` and the table must still be a table.
import { expect, test, type Page } from "@playwright/test";

test.describe.configure({ mode: "serial" });

const DOC =
  "# Roster\n\nBefore.\n\n| Name | Role |\n|---|---|\n| Sarah | Platform |\n| Bo | CTO |\n\nAfter.\n";

async function createDoc(page: Page, title: string, markdown: string) {
  const res = await page.request.post("/api/v1/documents", {
    data: { type: "note", title, markdown },
  });
  const { data } = await res.json();
  return data.path as string;
}

async function diskText(page: Page, path: string) {
  const res = await page.request.get(`/api/v1/documents/${path}`);
  const { data } = await res.json();
  return data.markdown as string;
}

test("read mode: edit a cell with a pipe, add a row, and it lands on disk escaped", async ({
  page,
}) => {
  const path = await createDoc(page, "Grid Read", DOC);
  await page.goto(`/doc/${path}`);
  await expect(page.getByRole("heading", { level: 1 }).first()).toBeVisible();

  // The button lives on the rendered table and shows on hover.
  await page.getByRole("cell", { name: "Sarah" }).hover();
  await page.getByRole("button", { name: "Edit table" }).click();
  const dialog = page.getByRole("dialog", { name: "Edit table" });
  await expect(dialog).toBeVisible();

  // The grid reflects the source.
  await expect(dialog.getByLabel("Header, column 2")).toHaveValue("Role");
  await expect(dialog.getByLabel("Row 2, column 1")).toHaveValue("Bo");

  await dialog.getByLabel("Row 1, column 2").fill("Platform | Infra");
  await dialog.getByRole("button", { name: "Add row" }).click();
  await dialog.getByLabel("Row 3, column 1").fill("Ada");
  await dialog.getByLabel("Row 3, column 2").fill("Research");
  await dialog.getByRole("button", { name: "Column 2 alignment: default" }).click(); // → left
  await dialog.getByRole("button", { name: "Save table" }).click();
  await expect(dialog).toHaveCount(0);

  // Rendered immediately, with the pipe as literal text in one cell.
  await expect(page.getByRole("cell", { name: "Platform | Infra" })).toBeVisible();
  await expect(page.getByRole("cell", { name: "Ada" })).toBeVisible();

  await expect.poll(() => diskText(page, path)).toContain("Platform \\| Infra");
  const text = await diskText(page, path);
  expect(text).toContain("| Ada");
  expect(text).toMatch(/\| -{3,} \| :-{3,} \|/); // column 2 is now left-aligned
  // Everything around the table is untouched.
  expect(text).toContain("Before.\n\n| Name");
  expect(text).toContain("\n\nAfter.\n");
});

test("read mode: cancel changes nothing", async ({ page }) => {
  const path = await createDoc(page, "Grid Cancel", DOC);
  await page.goto(`/doc/${path}`);
  await page.getByRole("cell", { name: "Sarah" }).hover();
  await page.getByRole("button", { name: "Edit table" }).click();
  const dialog = page.getByRole("dialog", { name: "Edit table" });
  await dialog.getByLabel("Row 1, column 1").fill("Nope");
  await dialog.getByRole("button", { name: "Cancel" }).click();
  await expect(dialog).toHaveCount(0);
  await expect(page.getByRole("cell", { name: "Sarah" })).toBeVisible();
  expect(await diskText(page, path)).toBe(DOC);
});

test("editor: the table panel opens the grid and writes into the buffer", async ({
  page,
}) => {
  const path = await createDoc(page, "Grid Editor", DOC);
  await page.goto(`/doc/${path}`);
  await expect(page.getByRole("heading", { level: 1 }).first()).toBeVisible();
  await page.keyboard.press("e");
  const editor = page.locator(".cm-content");
  await expect(editor).toBeVisible();
  await editor.getByText("| Bo | CTO |").click();

  await page.getByRole("toolbar", { name: "Table tools" }).getByRole("button", { name: "Edit as grid" }).click();
  const dialog = page.getByRole("dialog", { name: "Edit table" });
  await expect(dialog.getByLabel("Row 2, column 2")).toHaveValue("CTO");

  // Remove a column and a row.
  await dialog.getByRole("button", { name: "Remove row 1" }).click();
  await dialog.getByRole("button", { name: "Remove column 2" }).click();
  await expect(dialog.getByLabel("Row 1, column 1")).toHaveValue("Bo");
  await dialog.getByRole("button", { name: "Save table" }).click();

  await expect(editor).toContainText("| Bo   |");
  await expect(editor).not.toContainText("Sarah");
  await expect(editor).not.toContainText("CTO");

  await page.keyboard.press("ControlOrMeta+s");
  await expect.poll(() => diskText(page, path)).not.toContain("Sarah");
});

test("Enter walks down a column and grows the table", async ({ page }) => {
  const path = await createDoc(page, "Grid Enter", DOC);
  await page.goto(`/doc/${path}`);
  await page.getByRole("cell", { name: "Sarah" }).hover();
  await page.getByRole("button", { name: "Edit table" }).click();
  const dialog = page.getByRole("dialog", { name: "Edit table" });
  await dialog.getByLabel("Row 2, column 1").focus();
  await page.keyboard.press("Enter");
  await expect(dialog.getByLabel("Row 3, column 1")).toBeFocused();
  await page.keyboard.type("Grace");
  await page.keyboard.press("ControlOrMeta+Enter");
  await expect(dialog).toHaveCount(0);
  await expect(page.getByRole("cell", { name: "Grace" })).toBeVisible();
});
