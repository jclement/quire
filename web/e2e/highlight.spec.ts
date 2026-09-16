// ==highlight==, the way Obsidian and most note apps write it. The transform
// runs on the parsed tree rather than the source string, so code spans are
// exempt for free — this pins that, and that `a == b` in prose stays prose.
import { expect, test } from "@playwright/test";

test("==text== renders as a highlight, and only where it should", async ({ page }) => {
  const res = await page.request.post("/api/v1/documents", {
    data: {
      type: "note",
      title: "Highlight Check",
      markdown:
        "# Highlight Check\n\n" +
        "Some ==highlighted words== in a sentence.\n\n" +
        "In code: `==not a highlight==` stays literal.\n\n" +
        "Arithmetic: if a == b then fine.\n\n" +
        "Wrapping a link: ==see [[Somewhere]] first==\n",
    },
  });
  const path = (await res.json()).data.path as string;
  await page.goto(`/doc/${path}`);
  await expect(page.getByRole("heading", { level: 1 }).first()).toBeVisible();

  const marks = page.locator(".prose-quire mark");
  await expect(marks.filter({ hasText: "highlighted words" })).toHaveCount(1);

  // A code span is literal text, and `a == b` is arithmetic — neither is a
  // highlight, so only the two real ones exist.
  await expect(page.locator("code", { hasText: "==not a highlight==" })).toBeVisible();
  await expect(page.locator(".prose-quire mark", { hasText: "not a highlight" })).toHaveCount(0);
  await expect(marks).toHaveCount(2);

  // A highlight can carry a wikilink, and the link still works.
  const wrapping = marks.filter({ hasText: "see" });
  await expect(wrapping.getByRole("button", { name: "Somewhere" })).toBeVisible();

  // It is painted, not just marked up.
  const painted = await marks.first().evaluate((el) => getComputedStyle(el).backgroundColor);
  expect(painted).not.toBe("rgba(0, 0, 0, 0)");

  await page.locator(".prose-quire").screenshot({ path: "test-results/highlight.png" });

  // The file is untouched: this is rendering, not a rewrite.
  const disk = (await (await page.request.get(`/api/v1/documents/${path}`)).json()).data.markdown;
  expect(disk).toContain("==highlighted words==");
});

test("the editor colours ==highlight== in the source too", async ({ page }) => {
  const res = await page.request.post("/api/v1/documents", {
    data: {
      type: "note",
      title: "Highlight Source",
      markdown:
        "# Highlight Source\n\n" +
        "Some ==highlighted words== here.\n\n" +
        "In code: `==not one==` stays plain.\n\n" +
        "Arithmetic: if a == b then fine.\n",
    },
  });
  const path = (await res.json()).data.path as string;
  await page.goto(`/doc/${path}?edit=true`);
  const editor = page.locator(".cm-content");
  await expect(editor).toContainText("highlighted words");
  await page.waitForTimeout(400);

  // CodeMirror paints the marked span with the same token the page uses, so
  // the source reads the way the rendered note looks.
  const painted = await page.evaluate(() => {
    const transparent = (c: string) => c === "transparent" || c === "rgba(0, 0, 0, 0)";
    const hits: string[] = [];
    for (const el of Array.from(document.querySelectorAll(".cm-content span"))) {
      const bg = getComputedStyle(el).backgroundColor;
      if (!transparent(bg) && (el.textContent ?? "").trim()) {
        hits.push((el.textContent ?? "").trim());
      }
    }
    return hits;
  });
  expect(painted.join(" ")).toContain("highlighted words");
  // `a == b` is arithmetic and a code span is literal: neither gets painted.
  expect(painted.join(" ")).not.toContain("not one");
  expect(painted.some((t) => t.includes("a == b"))).toBe(false);

  // And typing it still leaves the file exactly as written.
  const disk = (await (await page.request.get(`/api/v1/documents/${path}`)).json()).data.markdown;
  expect(disk).toContain("==highlighted words==");
});
