// Tests for snippet parsing — the safety property is that only <mark> is
// interpreted; any other markup stays literal text.
import { describe, expect, test } from "bun:test";
import { parseSnippet } from "./snippet.ts";

describe("parseSnippet", () => {
  test("splits marked runs from plain text", () => {
    expect(parseSnippet("meet <mark>Sarah</mark> at noon")).toEqual([
      { text: "meet ", mark: false },
      { text: "Sarah", mark: true },
      { text: " at noon", mark: false },
    ]);
  });

  test("handles multiple marks and mark-at-edges", () => {
    expect(parseSnippet("<mark>a</mark>b<mark>c</mark>")).toEqual([
      { text: "a", mark: true },
      { text: "b", mark: false },
      { text: "c", mark: true },
    ]);
  });

  test("other HTML is left as literal text", () => {
    expect(parseSnippet("<b>bold</b> <script>x</script>")).toEqual([
      { text: "<b>bold</b> <script>x</script>", mark: false },
    ]);
  });

  test("unclosed mark stays literal", () => {
    expect(parseSnippet("<mark>oops")).toEqual([
      { text: "<mark>oops", mark: false },
    ]);
  });
});
