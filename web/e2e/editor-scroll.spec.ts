// Typing at the end of a long note. The editor grows to its full height and
// the page is what scrolls, so CodeMirror follows the cursor by scrolling an
// ancestor — landing flush against the last pixel row on desktop, and behind
// the fixed nav bar on a phone, which is not an ancestor and which it cannot
// see. Both are fixed by asking for room (scrollMargins) and making room
// (padding below the last line), so this checks the cursor keeps its
// clearance rather than checking either mechanism.
import { expect, test, type Page } from "@playwright/test";

/** Comfortably more than one line, comfortably less than the gutter we ask
 *  for, so the test states the intent without pinning the exact number. */
const MIN_CLEARANCE = 48;

const LONG =
  "# Long Doc\n\n" +
  Array.from({ length: 200 }, (_, i) => `line ${i + 1} of the body text`).join("\n") +
  "\n";

/** Where the line being typed sits, and what would cover it. */
async function cursorClearance(page: Page) {
  return page.evaluate(() => {
    const lines = document.querySelectorAll(".cm-line");
    const last = lines[lines.length - 1] as HTMLElement;
    const bottom = last.getBoundingClientRect().bottom;
    // The phone's nav bar is fixed over the page, so the floor is its top
    // edge rather than the bottom of the window.
    let floor = window.innerHeight;
    for (const el of Array.from(document.body.querySelectorAll("*"))) {
      if (getComputedStyle(el).position !== "fixed") continue;
      const r = (el as HTMLElement).getBoundingClientRect();
      if (r.height > 0 && r.bottom >= window.innerHeight - 2 && r.top > window.innerHeight / 2) {
        floor = Math.min(floor, r.top);
      }
    }
    return Math.round(floor - bottom);
  });
}

test("the line you are typing keeps clear of the bottom", async ({ page }) => {
  const res = await page.request.post("/api/v1/documents", {
    data: { type: "note", title: "Editor Scroll", markdown: LONG },
  });
  const path = (await res.json()).data.path as string;

  for (const [label, width, height] of [
    ["desktop", 1280, 800],
    ["phone", 390, 780],
  ] as const) {
    await page.setViewportSize({ width, height });
    await page.goto(`/doc/${path}?edit=true`);
    const editor = page.locator(".cm-content");
    await editor.waitFor();
    await editor.click();
    await page.keyboard.press("ControlOrMeta+End");
    await page.waitForTimeout(200);
    for (let i = 0; i < 6; i++) {
      await page.keyboard.type(`\ntyped line ${i}`);
      await page.waitForTimeout(100);
    }
    await page.waitForTimeout(500);

    const clearance = await cursorClearance(page);
    expect(clearance, `${label}: the line being typed is ${clearance}px from the floor`).toBeGreaterThanOrEqual(MIN_CLEARANCE);
    // And it is really the line just typed, not an empty tail.
    await expect(editor).toContainText("typed line 5");
  }
});
