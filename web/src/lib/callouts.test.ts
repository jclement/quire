// Tests for the callout marker grammar — case-insensitivity, aliases, the
// unknown-type fallback, and lines that must NOT parse as callouts.
import { describe, expect, test } from "bun:test";
import { parseCalloutMarker } from "./callouts.ts";

describe("parseCalloutMarker", () => {
  test("parses type and title, case-insensitively", () => {
    expect(parseCalloutMarker("[!NOTE] Remember this")).toEqual({
      type: "note",
      title: "Remember this",
    });
    expect(parseCalloutMarker("[!warning]")).toEqual({
      type: "warning",
      title: "",
    });
  });

  test("folds Obsidian aliases into canonical types", () => {
    expect(parseCalloutMarker("[!hint] Try it")?.type).toBe("tip");
    expect(parseCalloutMarker("[!bug]")?.type).toBe("danger");
    expect(parseCalloutMarker("[!faq]")?.type).toBe("question");
  });

  test("unknown types fall back to note, keeping the title", () => {
    expect(parseCalloutMarker("[!wat] Still shown")).toEqual({
      type: "note",
      title: "Still shown",
    });
  });

  test("tolerates the foldable +/- suffix", () => {
    expect(parseCalloutMarker("[!tip]- Collapsed")).toEqual({
      type: "tip",
      title: "Collapsed",
    });
  });

  test("plain blockquote text is not a callout", () => {
    expect(parseCalloutMarker("just a quote")).toBeNull();
    expect(parseCalloutMarker("[link] not a callout")).toBeNull();
    expect(parseCalloutMarker("some [!note] not at start")).toBeNull();
  });
});
