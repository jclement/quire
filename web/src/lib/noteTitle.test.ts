import { describe, expect, test } from "bun:test";
import { noteTitleFrom } from "./noteTitle.ts";

// Mirrors TestNoteTitleFrom in internal/service/tasknote_test.go — the two
// must agree, or the dialog proposes one title and an agent gets another.
describe("noteTitleFrom", () => {
  test.each([
    [
      "The sidebar collapses when you resize and the [[Search|search box]] loses focus, which is annoying #ux",
      "The sidebar collapses when you resize and the",
    ],
    ["Idea: a [[Frances Bagley]] intro.", "Idea: a Frances Bagley intro"],
    ["  short  ", "short"],
    ["#ux **bold** `code` thought", "bold code thought"],
  ])("%s", (input, want) => {
    expect(noteTitleFrom(input)).toBe(want);
  });
});
