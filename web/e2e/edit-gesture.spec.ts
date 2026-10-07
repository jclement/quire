// Read mode's edit gesture: a double-click on the rendered note, or a long
// press with a finger, opens the editor — and neither steals a click from
// the things in the note that already answer one.
import { expect, test, type Page } from "@playwright/test";

test.describe.configure({ mode: "serial" });

async function openDoc(page: Page, title: string, markdown: string) {
  const res = await page.request.post("/api/v1/documents", {
    data: { type: "note", title, markdown },
  });
  const { data } = await res.json();
  await page.goto(`/doc/${data.path}`);
  const body = page.getByTestId("read-body");
  await expect(body).toBeVisible();
  return { path: data.path as string, body };
}

/** A synthetic touch: Chromium's desktop project has no finger. */
async function touch(page: Page, type: string, text: string, dx = 0) {
  await page.getByTestId("read-body").getByText(text).evaluate(
    (element, { type, dx }) => {
      const box = element.getBoundingClientRect();
      element.dispatchEvent(
        new PointerEvent(type, {
          bubbles: true,
          pointerType: "touch",
          isPrimary: true,
          clientX: box.left + 5 + dx,
          clientY: box.top + 5,
        }),
      );
    },
    { type, dx },
  );
}

test("double-clicking the rendered note opens the editor", async ({ page }) => {
  const { body } = await openDoc(page, "Gesture Doc", "# Gesture Doc\n\nsome prose here\n");
  await body.getByText("some prose here").dblclick();
  await expect(page.locator(".cm-content")).toBeVisible();
  await expect(page.locator(".cm-content")).toContainText("some prose here");
  await expect(body).toHaveCount(0);
});

test("a double-click on a checkbox or a link stays a click", async ({ page }) => {
  const { body } = await openDoc(
    page,
    "Gesture Controls",
    "# Gesture Controls\n\n- [ ] ring the plumber\n\n[elsewhere](#nowhere)\n",
  );
  await body.getByRole("checkbox").dblclick();
  await body.getByRole("link", { name: "elsewhere" }).dblclick();
  await expect(body).toBeVisible();
  await expect(page.locator(".cm-content")).toHaveCount(0);
});

test("a long press opens the editor; a short tap or a drag does not", async ({ page }) => {
  const { body } = await openDoc(page, "Gesture Touch", "# Gesture Touch\n\npress me\n");

  // A tap: down and up well inside the threshold.
  await touch(page, "pointerdown", "press me");
  await touch(page, "pointerup", "press me");
  await page.waitForTimeout(700);
  await expect(body).toBeVisible();

  // A drag: the finger travels, so it is a scroll or a selection.
  await touch(page, "pointerdown", "press me");
  await touch(page, "pointermove", "press me", 40);
  await page.waitForTimeout(700);
  await expect(body).toBeVisible();
  await touch(page, "pointerup", "press me");

  // A press held still.
  await touch(page, "pointerdown", "press me");
  await expect(page.locator(".cm-content")).toBeVisible();
  await expect(body).toHaveCount(0);
});
