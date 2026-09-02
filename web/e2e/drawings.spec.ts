// Excalidraw drawings: inserted from the palette, drawn in the full-screen
// editor, saved as an SVG the note embeds like any image, and reopened from
// an "Edit drawing" button on that image.
import { expect, test, type Page } from "@playwright/test";

test.describe.configure({ mode: "serial" });

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

async function runCommand(page: Page, label: string) {
  await page.keyboard.press("ControlOrMeta+k");
  await page.getByLabel("Command palette input").fill(`>${label}`);
  await page.getByRole("listbox").getByRole("option", { name: label }).first().click();
}

/** Draws a rectangle on the Excalidraw canvas with the keyboard tool + a drag. */
async function drawRectangle(page: Page) {
  const canvas = page.locator(".excalidraw__canvas.interactive");
  await expect(canvas).toBeVisible();
  const box = (await canvas.boundingBox())!;
  // The tool is a visually hidden radio inside a label; click the label.
  await page.locator('label:has([data-testid="toolbar-rectangle"])').click();
  const x = box.x + box.width / 2;
  const y = box.y + box.height / 2;
  await page.mouse.move(x - 80, y - 50);
  await page.mouse.down();
  await page.mouse.move(x + 80, y + 50, { steps: 8 });
  await page.mouse.up();
}

test("insert a drawing from read mode, draw, save, and it embeds as an SVG", async ({
  page,
}) => {
  const path = await createDoc(page, "Sketch Read", "# Sketch\n\nSome prose.\n");
  await page.goto(`/doc/${path}`);
  await expect(page.getByRole("heading", { level: 1 }).first()).toBeVisible();

  await runCommand(page, "Insert drawing");
  const dialog = page.getByRole("dialog", { name: "Drawing" });
  await expect(dialog).toBeVisible();

  // The embed already exists in the document before anything is drawn.
  await expect
    .poll(() => diskText(page, path))
    .toMatch(/!\[Drawing\]\(attachments\/\d{4}\/\d{2}\/drawing-[0-9a-f]{6}\.excalidraw\.svg\)/);
  const svgPath = (await diskText(page, path)).match(/\((attachments\/[^)]+\.svg)\)/)![1]!;

  await drawRectangle(page);
  await dialog.getByRole("button", { name: "Save drawing" }).click();
  await expect(dialog).toHaveCount(0);
  await expect(page.getByText("Drawing saved")).toBeVisible();

  // The render on disk is a real SVG with something in it.
  const svg = await (await page.request.get(`/api/v1/files/${svgPath}`)).text();
  expect(svg).toContain("<svg");
  expect(svg).not.toContain("Empty drawing");
  expect(svg).not.toContain("<script");

  // And the note shows it as an image with the edit affordance.
  const img = page.locator(`img[data-drawing]`);
  await expect(img).toBeVisible();
  await img.hover();
  await page.getByRole("button", { name: "Edit drawing" }).click();
  await expect(dialog).toBeVisible();
  await expect(page.locator(".excalidraw__canvas.interactive")).toBeVisible();
  await dialog.getByRole("button", { name: "Cancel" }).click();
  await expect(dialog).toHaveCount(0);
});

test("cancelling after a change asks before discarding", async ({ page }) => {
  const path = await createDoc(page, "Sketch Cancel", "# Sketch\n");
  await page.goto(`/doc/${path}`);
  await runCommand(page, "Insert drawing");
  const dialog = page.getByRole("dialog", { name: "Drawing" });
  await drawRectangle(page);
  await dialog.getByRole("button", { name: "Cancel" }).click();
  await expect(dialog.getByText("Discard changes?")).toBeVisible();
  await dialog.getByRole("button", { name: "Discard" }).click();
  await expect(dialog).toHaveCount(0);
  const svgPath = (await diskText(page, path)).match(/\((attachments\/[^)]+\.svg)\)/)![1]!;
  const svg = await (await page.request.get(`/api/v1/files/${svgPath}`)).text();
  expect(svg).toContain("Empty drawing");
});

test("in the editor the embed lands at the cursor", async ({ page }) => {
  const path = await createDoc(page, "Sketch Edit", "# Sketch\n\nfirst\n\nlast\n");
  await page.goto(`/doc/${path}`);
  await expect(page.getByRole("heading", { level: 1 }).first()).toBeVisible();
  await page.keyboard.press("e");
  const editor = page.locator(".cm-content");
  await editor.getByText("first").click();
  await runCommand(page, "Insert drawing");
  const dialog = page.getByRole("dialog", { name: "Drawing" });
  await expect(dialog).toBeVisible();
  await dialog.getByRole("button", { name: "Cancel" }).click();
  await expect(editor).toContainText("![Drawing](attachments/");
  const text = await editor.innerText();
  expect(text.indexOf("first")).toBeLessThan(text.indexOf("![Drawing]"));
  expect(text.indexOf("![Drawing]")).toBeLessThan(text.indexOf("last"));
});
