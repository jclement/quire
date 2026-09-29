// Inbox triage: a selected task takes one key per decision, and the ways
// out of the inbox (someday, note) land where they should.
import { expect, test, type Page } from "@playwright/test";

const iso = (d: Date) =>
  `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;

async function markdownOf(page: Page, path: string): Promise<string> {
  const res = await page.request.get(`/api/v1/documents/${path}`);
  return (await res.json()).data.markdown as string;
}

test("triage keys schedule, park, delegate and file inbox tasks", async ({ page }) => {
  const res = await page.request.post("/api/v1/documents", {
    data: {
      type: "note",
      title: "Triage Fixture",
      markdown:
        "# Triage Fixture\n\n" +
        "- [ ] triage-alpha renew the domain\n" +
        "- [ ] triage-bravo learn the cello\n" +
        "- [ ] triage-charlie quote from the vendor\n" +
        "- [ ] triage-delta the onboarding flow repeats the same screen twice\n",
    },
  });
  const path = (await res.json()).data.path as string;

  await page.goto("/inbox");
  // The nav nags with a count.
  await expect(page.getByLabel(/\d+ in Inbox/)).toBeVisible();

  // t: due today.
  await page.getByText("triage-alpha renew the domain").click();
  await page.keyboard.press("t");
  await expect(page.getByText("Due today", { exact: true }).last()).toBeVisible();
  await expect.poll(() => markdownOf(page, path)).toContain(`triage-alpha renew the domain 📅 ${iso(new Date())}`);
  await expect(page.getByText("triage-alpha renew the domain")).toHaveCount(0);

  // s: someday — gone from the inbox, folded into Upcoming.
  await page.getByText("triage-bravo learn the cello").click();
  await page.keyboard.press("s");
  await expect(page.getByText("triage-bravo learn the cello")).toHaveCount(0);
  await expect.poll(() => markdownOf(page, path)).toContain("triage-bravo learn the cello #someday");

  // p: waiting, stamped with today's date.
  await page.getByText("triage-charlie quote from the vendor").click();
  await page.keyboard.press("p");
  await expect.poll(() => markdownOf(page, path)).toContain(`triage-charlie quote from the vendor ⏳ ${iso(new Date())}`);

  // n: make it a note, with the proposed title edited first.
  await page.getByText("triage-delta the onboarding flow").click();
  await page.keyboard.press("n");
  const title = page.getByLabel("Note title");
  await expect(title).toHaveValue("triage-delta the onboarding flow repeats the same screen");
  await title.fill("Triage Onboarding Repeat");
  await page.keyboard.press("Enter");
  await expect(page.getByText("Filed as note “Triage Onboarding Repeat”")).toBeVisible();
  await expect.poll(() => markdownOf(page, path)).toContain("- [[Triage Onboarding Repeat]]");
  expect(await markdownOf(page, "notes/triage-onboarding-repeat.md")).toContain(
    "triage-delta the onboarding flow repeats the same screen twice",
  );

  await page.goto("/tasks/upcoming");
  const someday = page.getByRole("button", { name: /Someday/ });
  await expect(someday).toHaveAttribute("aria-expanded", "false");
  await expect(page.getByText("triage-bravo learn the cello")).toHaveCount(0);
  await someday.click();
  await expect(page.getByText("triage-bravo learn the cello")).toBeVisible();
});
