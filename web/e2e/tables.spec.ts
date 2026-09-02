// Table editing in the real editor: the Reformat panel, the keybinding, Tab
// between cells, and the palette command that works from read mode.
import { expect, test } from "@playwright/test";

test.describe.configure({ mode: "serial" });

const RAGGED = "# Table Doc\n\n|Name|Role|\n|-|-|\n|Sarah Chen|Head of Platform|\n|Bo|CTO|\n";
const TIDY_HEADER = "| Name       | Role             |";

async function openInEditor(page: import("@playwright/test").Page, title: string, markdown: string) {
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

test("a panel appears inside a table and reformats it", async ({ page }) => {
  const { path, editor } = await openInEditor(page, "Table Panel", RAGGED);

  // Outside the table: no panel.
  await editor.click();
  await page.keyboard.press("ControlOrMeta+Home");
  await expect(page.getByRole("toolbar", { name: "Table tools" })).toHaveCount(0);

  // Click into the table body.
  await page.getByText("Sarah Chen", { exact: false }).first().click();
  const panel = page.getByRole("toolbar", { name: "Table tools" });
  await expect(panel).toBeVisible();

  await panel.getByRole("button", { name: "Reformat table" }).click();
  await expect(editor).toContainText(TIDY_HEADER);

  // The rewrite reaches disk through the ordinary save path.
  await page.keyboard.press("ControlOrMeta+s");
  await expect
    .poll(async () => (await page.request.get(`/api/v1/documents/${path}`)).text())
    .toContain(TIDY_HEADER);
});

test("the keybinding does the same thing", async ({ page }) => {
  const { editor } = await openInEditor(page, "Table Key", RAGGED);
  await page.getByText("Sarah Chen", { exact: false }).first().click();
  await page.keyboard.press("ControlOrMeta+Alt+t");
  await expect(editor).toContainText(TIDY_HEADER);
});

test("Tab moves to the next cell and makes a new row at the end", async ({ page }) => {
  const { editor } = await openInEditor(page, "Table Tab", "# T\n\n| a | b |\n|---|---|\n| 1 | 2 |\n");
  // Land in the last cell of the last row (a CodeMirror line is one text
  // node, so target the row, then End).
  await editor.getByText("| 1 | 2 |").click();
  await page.keyboard.press("End");
  await page.keyboard.press("ArrowLeft"); // inside the trailing "2 |"
  await page.keyboard.press("Tab");
  // A fresh blank row was appended, and typing lands in its first cell.
  await page.keyboard.type("new");
  await expect(editor).toContainText("| new |");
});

test("the palette reformats every table from read mode", async ({ page }) => {
  const res = await page.request.post("/api/v1/documents", {
    data: {
      type: "note",
      title: "Two Tables",
      markdown: "# Two Tables\n\n|a|b|\n|-|-|\n|1|2|\n\nprose\n\n|x|\n|-|\n|long value|\n",
    },
  });
  const { data } = await res.json();
  await page.goto(`/doc/${data.path}`);
  await expect(page.getByRole("heading", { level: 1 }).first()).toBeVisible();

  await page.keyboard.press("ControlOrMeta+k");
  await page.getByLabel("Command palette input").fill(">reformat");
  await page.getByRole("listbox").getByRole("option").filter({ hasText: /reformat all tables/i }).first().click();

  await expect
    .poll(async () => (await page.request.get(`/api/v1/documents/${data.path}`)).text())
    .toContain("| long value |");
  const body = await (await page.request.get(`/api/v1/documents/${data.path}`)).text();
  expect(body).toContain("| a   | b   |");
  expect(body).toContain("prose");
});
