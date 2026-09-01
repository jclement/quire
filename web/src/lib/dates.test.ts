// Tests for the date helpers — the edges that matter are day-boundary math
// (local, not UTC) and the grouping/ordering contract Upcoming relies on.
import { describe, expect, test } from "bun:test";
import {
  addDaysISO,
  daysFromToday,
  dueInfo,
  formatDayHeading,
  groupByDate,
  nextSaturdayISO,
  parseISODate,
  todayISO,
} from "./dates.ts";

describe("parseISODate", () => {
  test("parses as local midnight, not UTC", () => {
    const date = parseISODate("2026-09-01");
    expect(date?.getFullYear()).toBe(2026);
    expect(date?.getMonth()).toBe(8);
    expect(date?.getDate()).toBe(1);
    expect(date?.getHours()).toBe(0);
  });

  test("rejects non-date strings", () => {
    expect(parseISODate("not-a-date")).toBeNull();
    expect(parseISODate("2026-09-01T10:00:00Z")).toBeNull();
  });
});

describe("addDaysISO", () => {
  test("crosses month and year boundaries", () => {
    expect(addDaysISO("2026-08-31", 1)).toBe("2026-09-01");
    expect(addDaysISO("2026-12-31", 1)).toBe("2027-01-01");
    expect(addDaysISO("2026-03-01", -1)).toBe("2026-02-28");
  });
});

describe("nextSaturdayISO", () => {
  test("finds the coming Saturday", () => {
    // 2026-09-01 is a Tuesday; the next Saturday is the 5th.
    expect(nextSaturdayISO("2026-09-01")).toBe("2026-09-05");
  });

  test("from a Saturday returns the following Saturday, not the same day", () => {
    expect(nextSaturdayISO("2026-09-05")).toBe("2026-09-12");
  });
});

describe("dueInfo", () => {
  const today = "2026-09-01";

  test("overdue counts days and flags red", () => {
    expect(dueInfo("2026-08-29", today)).toEqual({
      label: "3d overdue",
      tone: "overdue",
    });
  });

  test("today and tomorrow get words", () => {
    expect(dueInfo(today, today)?.tone).toBe("today");
    expect(dueInfo("2026-09-02", today)?.label).toBe("tomorrow");
  });

  test("within a week shows day count, beyond shows a date", () => {
    expect(dueInfo("2026-09-04", today)?.label).toBe("3d");
    expect(dueInfo("2026-09-12", today)?.label).toBe("Sep 12");
  });

  test("garbage dates return null instead of throwing", () => {
    expect(dueInfo("someday", today)).toBeNull();
  });
});

describe("daysFromToday", () => {
  test("is negative for the past and zero for today", () => {
    expect(daysFromToday("2026-08-30", "2026-09-01")).toBe(-2);
    expect(daysFromToday("2026-09-01", "2026-09-01")).toBe(0);
  });
});

describe("groupByDate", () => {
  interface Item {
    id: number;
    due: string | null;
  }
  const items: Item[] = [
    { id: 1, due: "2026-09-03" },
    { id: 2, due: "2026-09-02" },
    { id: 3, due: null },
    { id: 4, due: "2026-09-02" },
  ];

  test("sorts groups by date with undated last, preserving item order", () => {
    const groups = groupByDate(items, (item) => item.due);
    expect(groups.map((group) => group.date)).toEqual([
      "2026-09-02",
      "2026-09-03",
      null,
    ]);
    expect(groups[0]?.items.map((item) => item.id)).toEqual([2, 4]);
  });

  test("empty input yields no groups", () => {
    expect(groupByDate([], () => null)).toEqual([]);
  });
});

describe("formatDayHeading", () => {
  test("renders weekday + month + day", () => {
    expect(formatDayHeading("2026-09-01")).toBe("Tuesday, September 1");
  });
});

describe("todayISO", () => {
  test("formats a fixed date", () => {
    expect(todayISO(new Date(2026, 0, 5))).toBe("2026-01-05");
  });
});
