// Date helpers for the task and daily-note UI. Vault dates are ISO 8601 local
// time (per DESIGN.md — UTC in frontmatter is hostile to grep), so day-precision
// values ("YYYY-MM-DD") are always constructed and compared as *local* dates,
// never via Date.parse, which would treat them as UTC midnight.

/** One day in milliseconds; used for whole-day difference arithmetic. */
const DAY_MS = 86_400_000;

/** Parses "YYYY-MM-DD" as a local date. Returns null for anything else. */
export function parseISODate(iso: string): Date | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(iso);
  if (!match) return null;
  return new Date(Number(match[1]), Number(match[2]) - 1, Number(match[3]));
}

/** Formats a local Date as "YYYY-MM-DD". */
export function toISODate(date: Date): string {
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${date.getFullYear()}-${month}-${day}`;
}

/** Today's date as "YYYY-MM-DD" (local). */
export function todayISO(now: Date = new Date()): string {
  return toISODate(now);
}

/** Returns `iso` shifted by `days` whole days. */
export function addDaysISO(iso: string, days: number): string {
  const date = parseISODate(iso);
  if (!date) return iso;
  date.setDate(date.getDate() + days);
  return toISODate(date);
}

/** The next Saturday strictly after `fromISO` — the quick-capture "Weekend" chip. */
export function nextSaturdayISO(fromISO: string): string {
  const date = parseISODate(fromISO);
  if (!date) return fromISO;
  const daysUntil = ((6 - date.getDay() + 7) % 7) || 7;
  date.setDate(date.getDate() + daysUntil);
  return toISODate(date);
}

/** Whole days from `todayIso` to `iso`; negative means past. */
export function daysFromToday(iso: string, todayIso: string): number | null {
  const date = parseISODate(iso);
  const today = parseISODate(todayIso);
  if (!date || !today) return null;
  return Math.round((date.getTime() - today.getTime()) / DAY_MS);
}

export type DueTone = "overdue" | "today" | "future";

export interface DueInfo {
  label: string;
  tone: DueTone;
}

/**
 * Human label + severity for a due date, e.g. "3d overdue" / "today" /
 * "tomorrow" / "Sep 12". Drives the red/amber due badges on task rows.
 */
export function dueInfo(due: string, todayIso: string): DueInfo | null {
  const days = daysFromToday(due, todayIso);
  if (days === null) return null;
  if (days < 0) return { label: `${-days}d overdue`, tone: "overdue" };
  if (days === 0) return { label: "today", tone: "today" };
  if (days === 1) return { label: "tomorrow", tone: "future" };
  if (days < 7) return { label: `${days}d`, tone: "future" };
  return { label: formatShortDate(due, todayIso), tone: "future" };
}

/** "Sep 12", with the year appended only when it differs from today's. */
export function formatShortDate(iso: string, todayIso: string): string {
  const date = parseISODate(iso);
  if (!date) return iso;
  const sameYear = iso.slice(0, 4) === todayIso.slice(0, 4);
  return date.toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    ...(sameYear ? {} : { year: "numeric" }),
  });
}

/** "Monday, September 1" — the Today page and daily-note heading. */
export function formatDayHeading(iso: string): string {
  const date = parseISODate(iso);
  if (!date) return iso;
  return date.toLocaleDateString("en-US", {
    weekday: "long",
    month: "long",
    day: "numeric",
  });
}

/**
 * Groups items by an ISO day key, ordered by date ascending with undated items
 * last — the Upcoming view's shape.
 */
export function groupByDate<T>(
  items: T[],
  getDate: (item: T) => string | null,
): { date: string | null; items: T[] }[] {
  const byDate = new Map<string | null, T[]>();
  for (const item of items) {
    const key = getDate(item);
    const bucket = byDate.get(key);
    if (bucket) bucket.push(item);
    else byDate.set(key, [item]);
  }
  const dated = [...byDate.entries()]
    .filter((entry): entry is [string, T[]] => entry[0] !== null)
    .sort((a, b) => a[0].localeCompare(b[0]))
    .map(([date, grouped]) => ({ date: date as string | null, items: grouped }));
  const undated = byDate.get(null);
  return undated ? [...dated, { date: null, items: undated }] : dated;
}

/** Relative timestamp for mtimes: "just now", "5m ago", "2h ago", "3d ago", "Aug 12". */
export function formatRelativeTime(iso: string, now: Date = new Date()): string {
  const then = new Date(iso);
  if (Number.isNaN(then.getTime())) return iso;
  const seconds = Math.floor((now.getTime() - then.getTime()) / 1000);
  if (seconds < 60) return "just now";
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
  if (seconds < 86_400) return `${Math.floor(seconds / 3600)}h ago`;
  if (seconds < 7 * 86_400) return `${Math.floor(seconds / 86_400)}d ago`;
  return formatShortDate(toISODate(then), todayISO(now));
}
