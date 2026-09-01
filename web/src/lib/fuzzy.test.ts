// Tests for fuzzy ranking — the contract that matters is the palette's ordering:
// exact title first, then prefix, then subsequence; non-matches dropped.
import { describe, expect, test } from "bun:test";
import { fuzzyRank, fuzzyScore } from "./fuzzy.ts";

describe("fuzzyScore", () => {
  test("exact match beats prefix beats subsequence", () => {
    const exact = fuzzyScore("today", "today");
    const prefix = fuzzyScore("today", "today's standup");
    const scattered = fuzzyScore("today", "the old d—ay notes");
    expect(exact).toBeGreaterThan(prefix);
    expect(prefix).toBeGreaterThan(scattered);
  });

  test("is case-insensitive", () => {
    expect(fuzzyScore("Sarah", "sarah chen")).toBeGreaterThan(0);
  });

  test("returns -1 when characters are missing or out of order", () => {
    expect(fuzzyScore("xyz", "sarah chen")).toBe(-1);
    expect(fuzzyScore("nehc", "chen")).toBe(-1);
  });

  test("empty query matches everything", () => {
    expect(fuzzyScore("", "anything")).toBe(0);
  });

  test("word-boundary matches outrank mid-word matches", () => {
    // "sc" hits the initials of "Sarah Chen" but lands mid-word in "muscle".
    expect(fuzzyScore("sc", "sarah chen")).toBeGreaterThan(
      fuzzyScore("sc", "muscle"),
    );
  });
});

describe("fuzzyRank", () => {
  test("drops non-matches and orders best-first", () => {
    const titles = ["Meeting notes", "Sarah Chen", "sarah", "Acme Corp"];
    expect(fuzzyRank("sarah", titles, (title) => title)).toEqual([
      "sarah",
      "Sarah Chen",
    ]);
  });

  test("keeps input order for equal scores", () => {
    const titles = ["alpha", "alphb"];
    expect(fuzzyRank("alp", titles, (title) => title)).toEqual([
      "alpha",
      "alphb",
    ]);
  });
});
