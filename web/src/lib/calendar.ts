// Month-grid construction for the calendar page: the API sends every day of a
// month in order, and this turns that flat list into Monday-first weeks with
// the blank cells that align them. Weeks start Monday (a work calendar, not a
// US wall calendar). Pure, so the edges that are easy to get wrong — leap
// Februaries, months that begin on a Sunday — are unit-tested rather than
// eyeballed in the UI.
import type { CalendarDay, CalendarDoc } from "../api/types.ts";
import { parseISODate, todayISO } from "./dates.ts";
import { dailyPath } from "./docs.ts";

/** Column headers, Monday first — the week start this app uses. */
export const WEEKDAY_LABELS = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];

const MONTH_KEY = /^\d{4}-(0[1-9]|1[0-2])$/;

/** True for a well-formed "YYYY-MM" — the /calendar/$month route param. */
export function isMonthKey(value: string): boolean {
  return MONTH_KEY.test(value);
}

/** The month a "YYYY-MM-DD" day belongs to. */
export function monthKey(iso: string): string {
  return iso.slice(0, 7);
}

/** This month, local — where /calendar and the "Today" button land. */
export function currentMonthKey(now: Date = new Date()): string {
  return monthKey(todayISO(now));
}

/** 0 = Monday … 6 = Sunday, for a "YYYY-MM-DD" date. */
export function mondayIndex(iso: string): number {
  const date = parseISODate(iso);
  if (!date) return 0;
  return (date.getDay() + 6) % 7;
}

/** "September 2026" — the month heading. */
export function formatMonthLabel(month: string): string {
  const date = parseISODate(`${month}-01`);
  if (!date) return month;
  return date.toLocaleDateString("en-US", { month: "long", year: "numeric" });
}

/**
 * The payload's days as weeks of exactly seven cells, Monday first, padded
 * with nulls at both ends. Days are never reordered, dropped, or invented —
 * the payload decides which days exist, so a 29-day February needs no special
 * case here.
 */
export function monthGrid(days: CalendarDay[]): (CalendarDay | null)[][] {
  const first = days[0];
  if (!first) return [];
  const cells: (CalendarDay | null)[] = [
    ...Array<null>(mondayIndex(first.date)).fill(null),
    ...days,
  ];
  while (cells.length % 7 !== 0) cells.push(null);
  const weeks: (CalendarDay | null)[][] = [];
  for (let at = 0; at < cells.length; at += 7) {
    weeks.push(cells.slice(at, at + 7));
  }
  return weeks;
}

/**
 * The touched documents worth listing in a day's cell. Meetings get their own
 * line and the daily note is the day number itself, so both would otherwise
 * show up twice — a day's own note is touched on that day almost by
 * definition.
 */
export function otherTouched(day: CalendarDay): CalendarDoc[] {
  const shown = new Set(day.meetings.map((meeting) => meeting.path));
  shown.add(dailyPath(day.date));
  return day.touched.filter((doc) => !shown.has(doc.path));
}
