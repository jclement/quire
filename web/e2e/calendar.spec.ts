// The calendar feed, end to end: a secret ICS URL is added in Settings and
// never shown back, the day's events appear as Today's agenda with invitees
// matched to person pages, "Create meeting note" makes the note once and
// then becomes "Open note", and prep shows on Today and in the note's rail.
// The feed is e2e/fake-ics.ts: a daily series in floating time, so it is on
// "today" in any zone.
import { expect, test } from "@playwright/test";

test.describe.configure({ mode: "serial" });

const FEED_PORT = Number(process.env.QUIRE_E2E_PORT ?? 8351) + 3;
const SECRET = "e2e-s3cret-token";
const FEED_URL = `http://127.0.0.1:${FEED_PORT}/dav/${SECRET}/calendar.ics`;

test.afterAll(async ({ request }) => {
  // Leave Today calendar-free for every other spec.
  const feeds = (await (await request.get("/api/v1/calendar/feeds")).json()).data as { id: string }[];
  for (const feed of feeds) await request.delete(`/api/v1/calendar/feeds/${feed.id}`);
});

test("a feed added in Settings is fetched and shown masked", async ({ page }) => {
  // The person the invitation's email should find (the invite calls her
  // "Dana W."), and something open with her for prep to surface.
  for (const doc of [
    {
      type: "person",
      title: "Dana Whitfield",
      markdown: "---\nemail: [dana@home.example, Dana@Acme.example]\n---\n# Dana Whitfield\n",
    },
    {
      type: "note",
      title: "Pricing tiers",
      markdown: "# Pricing tiers\n\n- [ ] Signed order form ⏳ [[Dana Whitfield]]\n",
    },
  ]) {
    expect((await page.request.post("/api/v1/documents", { data: doc })).status()).toBe(201);
  }

  await page.goto("/settings");
  await page.getByLabel("Calendar feed URL").fill(FEED_URL);
  await page.getByRole("button", { name: "Add feed" }).click();
  await expect(page.getByTestId("calendar-feed-status")).toContainText("2 events");
  await expect(page.getByLabel("Calendar feed URL")).toHaveValue("");

  // The secret is gone from the page and from the API once saved.
  await expect(page.locator("body")).not.toContainText(SECRET);
  const listed = await (await page.request.get("/api/v1/calendar/feeds")).text();
  expect(listed).not.toContain(SECRET);
});

test("today's agenda: create the meeting note once, then open it", async ({ page }) => {
  await page.goto("/");
  const agenda = page.getByRole("region", { name: "Agenda" });
  await expect(agenda.getByText("All day:")).toBeVisible();
  await expect(agenda.getByText("On call")).toBeVisible();

  const review = agenda.getByRole("listitem", { name: "Design review" });
  await expect(review.getByText("10:00–10:30")).toBeVisible();
  // "Dana W." matched by the second address in her email list, case-folded,
  // and shown by her page's name.
  await expect(review.getByRole("link", { name: "Dana Whitfield" })).toBeVisible();
  await expect(review.getByText("Rafael Ortiz")).toBeVisible();
  await expect(review.getByRole("link", { name: "Join" })).toHaveAttribute("href", "https://meet.example.com/design");

  // Prep, from the agenda.
  await review.getByRole("button", { name: "Prep for Design review" }).click();
  const prep = page.getByRole("dialog", { name: "Prep: Design review" });
  await expect(prep.getByRole("region", { name: "Prep: Dana Whitfield" })).toBeVisible();
  await expect(prep.getByText("First meeting on file.")).toBeVisible();
  await expect(prep.getByRole("list", { name: "Waiting on them" })).toContainText("Signed order form");
  await expect(prep.getByRole("link", { name: "Pricing tiers" })).toBeVisible();
  await expect(prep.getByText(/No page:.*Rafael Ortiz/)).toBeVisible();
  await page.keyboard.press("Escape");

  await review.getByRole("button", { name: "Create meeting note" }).click();
  await expect(page).toHaveURL(/\/doc\/meetings\/\d{4}-\d{2}-\d{2}-design-review\.md/);
  const path = decodeURIComponent(new URL(page.url()).pathname.replace(/^\/doc\//, ""));

  const doc = (await (await page.request.get(`/api/v1/documents/${path}`)).json()).data;
  expect(doc.frontmatter.event_uid).toBe("design-review@e2e.example");
  expect(doc.frontmatter.people).toEqual(["[[Dana Whitfield]]"]);
  expect(doc.frontmatter.date).toMatch(/T10:00$/);
  expect(doc.markdown).toContain("**Attendees:** Owner, Rafael Ortiz");

  // The meeting's own rail preps the same people.
  const rail = page.getByRole("region", { name: "Meeting prep" });
  await expect(rail.getByRole("link", { name: "Dana Whitfield", exact: true })).toBeVisible();
  await expect(rail.getByRole("list", { name: "Waiting on them" })).toContainText("Signed order form");

  // Back on Today the button has become the way into the note.
  await page.goto("/");
  const again = page.getByRole("region", { name: "Agenda" }).getByRole("listitem", { name: "Design review" });
  await expect(again.getByRole("link", { name: "Open note" })).toBeVisible();
  await expect(again.getByRole("button", { name: "Create meeting note" })).toHaveCount(0);
  await again.getByRole("link", { name: "Open note" }).click();
  await expect(page).toHaveURL(new RegExp(path.replace(/\./g, "\\.")));

  // Idempotent server-side too: a second create returns the same note.
  const date = path.slice("meetings/".length, "meetings/".length + 10);
  const second = await page.request.post("/api/v1/calendar/events/note", {
    data: { uid: "design-review@e2e.example", date },
  });
  expect(second.status()).toBe(200);
  expect((await second.json()).data.path).toBe(path);
});
