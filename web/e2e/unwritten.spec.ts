// Names the vault refers to that have no document yet: an inventory page
// that creates them as the right type, and a dangling link in read mode
// that writes itself when clicked.
import { expect, test, type Page } from "@playwright/test";

async function createDoc(page: Page, title: string, markdown: string) {
  const res = await page.request.post("/api/v1/documents", {
    data: { type: "note", title, markdown },
  });
  return (await res.json()).data.path as string;
}

test("the Unwritten page lists dangling names and creates them", async ({ page }) => {
  await createDoc(
    page,
    "Mentions Ghosts",
    "# Mentions Ghosts\n\nSpoke to [[Wilma Ghost]] about [[Phantom Corp]].\n",
  );
  await createDoc(page, "Mentions Again", "# Mentions Again\n\n[[Wilma Ghost]] again.\n");

  await expect
    .poll(async () => {
      const res = await page.request.get("/api/v1/unwritten");
      const { data } = await res.json();
      return (data as { name: string }[]).map((u) => u.name).join(",");
    })
    .toContain("Wilma Ghost");

  await page.goto("/unwritten");
  await expect(page.getByRole("heading", { name: "Unwritten" })).toBeVisible();
  const row = page.getByRole("listitem").filter({ hasText: "Wilma Ghost" });
  await expect(row).toContainText("2 mentions");
  await expect(row.getByRole("link", { name: "Mentions Again" })).toBeVisible();

  await row.getByRole("button", { name: "Create Wilma Ghost" }).click();
  await page.getByRole("menuitem", { name: "Person" }).click();
  await expect(page.getByRole("heading", { name: "Wilma Ghost" }).first()).toBeVisible();
  const doc = await (await page.request.get("/api/v1/documents/people/wilma-ghost.md")).json();
  expect(doc.data.type).toBe("person");

  // Written up, so it leaves the list.
  await page.goto("/unwritten");
  await expect(page.getByRole("listitem").filter({ hasText: "Wilma Ghost" })).toHaveCount(0);
  await expect(page.getByRole("listitem").filter({ hasText: "Phantom Corp" })).toHaveCount(1);
});

test("a dangling link in read mode creates the note when clicked", async ({ page }) => {
  const path = await createDoc(
    page,
    "Dangling Reader",
    "# Dangling Reader\n\nAsk [[Nobody Yet]] about it.\n",
  );
  await page.goto(`/doc/${path}`);
  await expect(page.getByRole("heading", { level: 1 }).first()).toBeVisible();
  const link = page.getByRole("button", { name: "Nobody Yet" });
  await expect(link).toBeVisible();
  await link.click();
  await expect(page.getByRole("heading", { name: "Nobody Yet" }).first()).toBeVisible();
  await expect(page).toHaveURL(/nobody-yet/);

  // The original link now resolves, so it is a link again rather than a button.
  await page.goto(`/doc/${path}`);
  await expect(page.getByRole("link", { name: "Nobody Yet" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Nobody Yet" })).toHaveCount(0);
});
