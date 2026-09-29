// A stand-in calendar provider for the Playwright suite: one secret ICS feed
// URL, the way Fastmail publishes one. The events recur daily from long ago
// in floating time, so "today" has them whatever day and time zone the
// suite runs in — and the agenda exercises recurrence expansion for real.
// Started by playwright.config.ts as a webServer beside fake-openai.
const port = Number(process.env.FAKE_ICS_PORT ?? 8354);

/** The secret part of the feed URL; the spec checks it never reappears. */
const SECRET_PATH = "/dav/e2e-s3cret-token/calendar.ics";

const ICS = [
  "BEGIN:VCALENDAR",
  "VERSION:2.0",
  "PRODID:-//quire e2e//fake calendar//EN",
  "BEGIN:VEVENT",
  "UID:design-review@e2e.example",
  "DTSTAMP:20250101T000000Z",
  "DTSTART:20250101T100000",
  "DTEND:20250101T103000",
  "RRULE:FREQ=DAILY",
  "SUMMARY:Design review",
  "LOCATION:https://meet.example.com/design",
  "ORGANIZER;CN=Owner:mailto:owner@e2e.example",
  // Not the name on her page: the email is what has to match.
  "ATTENDEE;CN=Dana W.:mailto:dana@acme.example",
  "ATTENDEE;CN=Rafael Ortiz:mailto:rafael@elsewhere.example",
  "END:VEVENT",
  "BEGIN:VEVENT",
  "UID:on-call@e2e.example",
  "DTSTAMP:20250101T000000Z",
  "DTSTART;VALUE=DATE:20250101",
  "RRULE:FREQ=DAILY",
  "SUMMARY:On call",
  "END:VEVENT",
  "END:VCALENDAR",
  "",
].join("\r\n");

Bun.serve({
  port,
  hostname: "127.0.0.1",
  fetch(req) {
    const url = new URL(req.url);
    if (url.pathname === "/health") return new Response("ok");
    if (url.pathname === SECRET_PATH) {
      return new Response(ICS, {
        headers: { "Content-Type": "text/calendar; charset=utf-8" },
      });
    }
    return new Response("not found", { status: 404 });
  },
});
console.log(`fake ics on 127.0.0.1:${port}`);
