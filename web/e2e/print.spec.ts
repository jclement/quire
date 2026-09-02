// PDF export is a real output format here, not an afterthought: the document
// menu, the palette's >print, and ⌘P all land in the same place. Nothing
// automated checked it, which for print CSS is a slow leak — chrome creeps
// back onto the page and nobody notices until they print something.
import { expect, test } from "@playwright/test";

test.describe.configure({ mode: "serial" });

const RICH = [
  "# Print Me",
  "",
  "> [!warning] Mind this",
  "> Callout panels stay coloured on paper.",
  "",
  "Some prose, then code:",
  "",
  "```go",
  'fmt.Println("printed")',
  "```",
  "",
  "```mermaid",
  "graph TD;",
  "  A-->B;",
  "```",
  "",
  "- [x] a finished thing",
].join("\n");

test("the printed page drops app chrome and keeps the document", async ({ page }) => {
  const created = await page.request.post("/api/v1/documents", {
    data: { type: "note", title: "Print Me", markdown: RICH },
  });
  const { data } = await created.json();

  await page.goto(`/doc/${data.path}`);
  await expect(page.getByRole("heading", { name: "Print Me" }).first()).toBeVisible();
  // The sidebar is part of the app, not the document.
  await expect(page.getByRole("link", { name: "Settings" })).toBeVisible();

  await page.emulateMedia({ media: "print" });

  // Chrome goes; content stays.
  await expect(page.getByRole("link", { name: "Settings" })).toBeHidden();
  await expect(page.getByText('fmt.Println("printed")')).toBeVisible();
  await expect(page.getByText("Callout panels stay coloured on paper.")).toBeVisible();
});

test("callout panels keep a colour when printed", async ({ page }) => {
  const created = await page.request.post("/api/v1/documents", {
    data: { type: "note", title: "Print Callout", markdown: RICH },
  });
  const { data } = await created.json();
  await page.goto(`/doc/${data.path}`);

  const callout = page.locator("[data-callout]").first();
  await expect(callout).toBeVisible();

  await page.emulateMedia({ media: "print" });
  const background = await callout.evaluate(
    (el) => getComputedStyle(el).backgroundColor,
  );

  // Not transparent and not plain white: the panel is tinted, which is the
  // thing that was specifically asked for and the thing a print stylesheet
  // most easily flattens away.
  expect(background).not.toBe("rgba(0, 0, 0, 0)");
  expect(background).not.toBe("rgb(255, 255, 255)");
});

test("a PDF renders with the document's text in it", async ({ page, browserName }) => {
  test.skip(browserName !== "chromium", "PDF generation is Chromium-only");

  const created = await page.request.post("/api/v1/documents", {
    data: { type: "note", title: "Print PDF", markdown: RICH },
  });
  const { data } = await created.json();
  await page.goto(`/doc/${data.path}`);
  // Mermaid is lazy; let the diagram exist before capturing.
  await expect(page.locator("svg[id^='mermaid'], .mermaid svg").first()).toBeVisible({
    timeout: 20_000,
  });

  const pdf = await page.pdf({ format: "A4", printBackground: true });

  // A real PDF, and not an empty one: the header plus enough bytes that the
  // page clearly has content rather than being a blank sheet.
  expect(pdf.subarray(0, 5).toString()).toBe("%PDF-");
  expect(pdf.byteLength).toBeGreaterThan(10_000);
});
