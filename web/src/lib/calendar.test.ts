// Tests for the calendar month grid. What matters here is alignment: weeks
// start Monday, the padding is right for a month beginning on any weekday, and
// no day is dropped or duplicated across a leap February or a month boundary.
import { describe, expect, test } from "bun:test";
import type { CalendarDay } from "../api/types.ts";
import {
  currentMonthKey,
  formatMonthLabel,
  isMonthKey,
  mondayIndex,
  monthGrid,
  monthKey,
  otherTouched,
} from "./calendar.ts";

/** A month's worth of empty days, exactly as the API sends them. */
function daysOf(month: string, count: number): CalendarDay[] {
  return Array.from({ length: count }, (_, at) => ({
    date: `${month}-${String(at + 1).padStart(2, "0")}`,
    has_daily: false,
    touched: [],
    meetings: [],
    completed_tasks: 0,
  }));
}

function dates(weeks: (CalendarDay | null)[][]): (string | null)[] {
  return weeks.flat().map((day) => day?.date ?? null);
}

describe("mondayIndex", () => {
  test("Monday is 0 and Sunday is 6", () => {
    // 2026-06-01 is a Monday; 2026-02-01 is a Sunday.
    expect(mondayIndex("2026-06-01")).toBe(0);
    expect(mondayIndex("2026-02-01")).toBe(6);
  });
});

describe("monthGrid", () => {
  test("pads the first week up to the month's starting weekday", () => {
    // 2026-09-01 is a Tuesday, so one blank leads.
    const weeks = monthGrid(daysOf("2026-09", 30));
    expect(weeks[0]?.[0]).toBeNull();
    expect(weeks[0]?.[1]?.date).toBe("2026-09-01");
  });

  test("a month starting on Monday has no leading blanks", () => {
    const weeks = monthGrid(daysOf("2026-06", 30));
    expect(weeks[0]?.[0]?.date).toBe("2026-06-01");
  });

  test("a month starting on Sunday fills the whole first row", () => {
    // 2026-02-01 is a Sunday: six blanks, then the 1st in the last column.
    const weeks = monthGrid(daysOf("2026-02", 28));
    expect(dates(weeks).slice(0, 7)).toEqual([
      null,
      null,
      null,
      null,
      null,
      null,
      "2026-02-01",
    ]);
  });

  test("keeps a leap February's 29th and pads the last week", () => {
    // 2024-02-01 is a Thursday: 3 blanks + 29 days = 32 cells → 5 weeks.
    const weeks = monthGrid(daysOf("2024-02", 29));
    expect(weeks).toHaveLength(5);
    expect(dates(weeks).filter((date) => date !== null)).toHaveLength(29);
    expect(weeks[4]?.[3]?.date).toBe("2024-02-29");
    expect(weeks[4]?.[4]).toBeNull();
  });

  test("every week has seven cells and days keep the payload's order", () => {
    const days = daysOf("2026-11", 30);
    const weeks = monthGrid(days);
    for (const week of weeks) expect(week).toHaveLength(7);
    expect(dates(weeks).filter((date) => date !== null)).toEqual(
      days.map((day) => day.date),
    );
  });

  test("an empty payload produces no weeks at all", () => {
    expect(monthGrid([])).toEqual([]);
  });
});

describe("isMonthKey", () => {
  test("accepts YYYY-MM and rejects anything else", () => {
    expect(isMonthKey("2026-09")).toBe(true);
    expect(isMonthKey("2026-13")).toBe(false);
    expect(isMonthKey("2026-00")).toBe(false);
    expect(isMonthKey("2026-09-01")).toBe(false);
    expect(isMonthKey("september")).toBe(false);
  });
});

describe("monthKey", () => {
  test("takes the month off a day, and off today for the current month", () => {
    expect(monthKey("2026-09-30")).toBe("2026-09");
    expect(currentMonthKey(new Date(2026, 11, 31))).toBe("2026-12");
  });
});

describe("formatMonthLabel", () => {
  test("renders month and year", () => {
    expect(formatMonthLabel("2026-09")).toBe("September 2026");
  });

  test("passes garbage through instead of throwing", () => {
    expect(formatMonthLabel("nope")).toBe("nope");
  });
});

describe("otherTouched", () => {
  const day: CalendarDay = {
    date: "2026-09-01",
    has_daily: true,
    touched: [
      { path: "daily/2026-09-01.md", title: "2026-09-01", type: "daily" },
      { path: "meetings/standup.md", title: "Standup", type: "meeting" },
      { path: "notes/apollo.md", title: "Apollo", type: "note" },
    ],
    meetings: [
      { path: "meetings/standup.md", title: "Standup", type: "meeting" },
    ],
    completed_tasks: 2,
  };

  test("drops the day's own note and anything already listed as a meeting", () => {
    expect(otherTouched(day).map((doc) => doc.path)).toEqual([
      "notes/apollo.md",
    ]);
  });
});
