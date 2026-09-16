// Both bundled faces ship contextual alternates that redraw `=>` as ⇒ and
// `->` as →. The file never changes, but what you typed is not what you see.
// Ligatures are a paint-time thing with no DOM to assert on, so this checks
// the computed font features — the property that decides it.
import { expect, test } from "@playwright/test";

const SYMBOLS = "const f = (x) => x != 0; return a -> b; a >= c <= d";

test("no character pair is redrawn as a single glyph", async ({ page }) => {
  const res = await page.request.post("/api/v1/documents", {
    data: { type: "note", title: "Ligature Check", markdown: `# Ligature Check\n\n${SYMBOLS}\n` },
  });
  const path = (await res.json()).data.path as string;

  // Prose: Inter's contextual alternates are the ones drawing the arrows.
  await page.goto(`/doc/${path}`);
  await expect(page.getByText("=>", { exact: false }).first()).toBeVisible();
  const prose = await page.evaluate(() =>
    getComputedStyle(document.body).fontFeatureSettings,
  );
  expect(prose).toContain("calt");
  expect(prose).toContain("0");

  // The editor: the mono face loses standard ligatures as well, so the
  // monospace grid stays one glyph per column.
  await page.goto(`/doc/${path}?edit=true`);
  const editor = page.locator(".cm-content");
  await expect(editor).toBeVisible();
  const code = await page.evaluate(() => {
    const el = document.querySelector(".cm-content")!;
    const s = getComputedStyle(el);
    return { features: s.fontFeatureSettings, ligatures: s.fontVariantLigatures };
  });
  expect(code.features).toContain("liga");
  expect(code.features).toContain("calt");
  expect(code.ligatures).toBe("none");

  // The text itself was never touched, whatever the font did with it.
  expect(await editor.innerText()).toContain("(x) => x != 0");
});
