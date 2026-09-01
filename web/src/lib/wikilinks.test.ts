// Tests for wikilink splitting — aliases, adjacency, and malformed brackets
// that must stay literal text.
import { describe, expect, test } from "bun:test";
import { splitWikilinks } from "./wikilinks.ts";

describe("splitWikilinks", () => {
  test("plain text passes through untouched", () => {
    expect(splitWikilinks("no links here")).toEqual([
      { kind: "text", text: "no links here" },
    ]);
  });

  test("extracts links with surrounding text", () => {
    expect(splitWikilinks("see [[Sarah Chen]] today")).toEqual([
      { kind: "text", text: "see " },
      { kind: "link", target: "Sarah Chen", display: "Sarah Chen", inner: "Sarah Chen" },
      { kind: "text", text: " today" },
    ]);
  });

  test("alias controls the display text", () => {
    const segments = splitWikilinks("[[people/sarah-chen|Sarah]]");
    expect(segments).toEqual([
      {
        kind: "link",
        target: "people/sarah-chen",
        display: "Sarah",
        inner: "people/sarah-chen|Sarah",
      },
    ]);
  });

  test("handles adjacent links", () => {
    const segments = splitWikilinks("[[A]][[B]]");
    expect(segments.map((seg) => seg.kind)).toEqual(["link", "link"]);
  });

  test("unclosed or empty brackets stay literal", () => {
    expect(splitWikilinks("a [[dangling link")).toEqual([
      { kind: "text", text: "a [[dangling link" },
    ]);
    expect(splitWikilinks("[[]]")).toEqual([{ kind: "text", text: "[[]]" }]);
  });
});
