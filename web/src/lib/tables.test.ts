// The table formatter is a pure text transform, so it gets the exhaustive
// treatment: every GFM shape, the two cases a naive split gets wrong (pipes
// in code spans and escaped pipes), and display width for wide glyphs.
import { describe, expect, test } from "bun:test";
import {
  displayWidth,
  findTableAt,
  formatAllTables,
  formatTable,
  splitCells,
} from "./tables.ts";

describe("splitCells", () => {
  test("strips the border pipes and trims", () => {
    expect(splitCells("| a | b |")).toEqual(["a", "b"]);
    expect(splitCells("a | b")).toEqual(["a", "b"]);
    expect(splitCells("|a|b|")).toEqual(["a", "b"]);
  });
  test("keeps an empty cell", () => {
    expect(splitCells("| a |  | c |")).toEqual(["a", "", "c"]);
  });
  test("a pipe inside a code span is content", () => {
    expect(splitCells("| `a|b` | c |")).toEqual(["`a|b`", "c"]);
    expect(splitCells("| ``x | y`` | z |")).toEqual(["``x | y``", "z"]);
  });
  test("an escaped pipe is content and stays escaped", () => {
    expect(splitCells("| a \\| b | c |")).toEqual(["a \\| b", "c"]);
  });
});

describe("displayWidth", () => {
  test("ascii is one per char", () => {
    expect(displayWidth("hello")).toBe(5);
  });
  test("CJK is two per glyph", () => {
    expect(displayWidth("日本語")).toBe(6);
  });
  test("emoji is two, and a ZWJ sequence is one glyph", () => {
    expect(displayWidth("🚀")).toBe(2);
    expect(displayWidth("👨‍👩‍👧")).toBe(2);
  });
  test("combining marks add nothing", () => {
    expect(displayWidth("é")).toBe(1);
  });
});

describe("formatTable", () => {
  test("pads a ragged table into a grid", () => {
    const input = "|Name|Role|\n|-|-|\n|Sarah Chen|Head of Platform|\n|Bo|CTO|";
    expect(formatTable(input)).toBe(
      [
        "| Name       | Role             |",
        "| ---------- | ---------------- |",
        "| Sarah Chen | Head of Platform |",
        "| Bo         | CTO              |",
      ].join("\n"),
    );
  });

  test("preserves alignment markers and aligns cell text", () => {
    const input =
      "| L | C | R |\n|:--|:-:|--:|\n| a | b | c |\n| long | mid | x |";
    expect(formatTable(input)).toBe(
      [
        "| L    |  C  |   R |",
        "| :--- | :-: | --: |",
        "| a    |  b  |   c |",
        "| long | mid |   x |",
      ].join("\n"),
    );
  });

  test("keeps escaped pipes and code-span pipes intact", () => {
    const input =
      "| expr | result |\n|--|--|\n| `a\\|b` | x |\n| a \\| b | y |";
    const out = formatTable(input);
    expect(out).toContain("`a\\|b`");
    expect(out).toContain("a \\| b");
    // And still splits into exactly two columns.
    for (const line of out.split("\n")) {
      expect(splitCells(line).length).toBe(2);
    }
  });

  test("wide glyphs are padded by display width, so columns line up", () => {
    const input = "| 名前 | x |\n|--|--|\n| 日本語 | y |";
    const out = formatTable(input).split("\n");
    // Every line has the same display width when the grid is straight.
    const widths = out.map(displayWidth);
    expect(new Set(widths).size).toBe(1);
  });

  test("pads short rows and keeps extra cells", () => {
    const input = "| a | b | c |\n|--|--|--|\n| 1 |\n| 1 | 2 | 3 | 4 |";
    const out = formatTable(input).split("\n");
    expect(splitCells(out[2]!)).toEqual(["1", "", "", ""]);
    expect(splitCells(out[3]!)).toEqual(["1", "2", "3", "4"]);
  });

  test("a delimiter is never shorter than three dashes", () => {
    expect(formatTable("|a|\n|-|\n|b|")).toBe("| a   |\n| --- |\n| b   |");
  });

  test("leaves non-tables alone", () => {
    expect(formatTable("just | prose | with pipes")).toBe(
      "just | prose | with pipes",
    );
    expect(formatTable("one line")).toBe("one line");
  });

  test("keeps a trailing newline if there was one", () => {
    expect(formatTable("|a|\n|-|\n|b|\n")).toEndWith("\n");
    expect(formatTable("|a|\n|-|\n|b|")).not.toEndWith("\n");
  });

  test("is idempotent", () => {
    const input = "|Name|Role|\n|-|-|\n|Sarah Chen|Head of Platform|";
    const once = formatTable(input);
    expect(formatTable(once)).toBe(once);
  });
});

describe("findTableAt", () => {
  const doc = [
    "# Title",
    "",
    "| a | b |",
    "|---|---|",
    "| 1 | 2 |",
    "| 3 | 4 |",
    "",
    "prose after",
  ].join("\n");

  test("finds the table around a cursor inside it", () => {
    const inside = doc.indexOf("| 1 |") + 2;
    const range = findTableAt(doc, inside)!;
    expect(range).not.toBeNull();
    expect(doc.slice(range.from, range.to)).toBe(
      "| a | b |\n|---|---|\n| 1 | 2 |\n| 3 | 4 |",
    );
  });

  test("works from the header line and the last line", () => {
    expect(findTableAt(doc, doc.indexOf("| a |"))).not.toBeNull();
    expect(findTableAt(doc, doc.indexOf("| 3 |") + 1)).not.toBeNull();
  });

  test("returns null outside a table", () => {
    expect(findTableAt(doc, 0)).toBeNull();
    expect(findTableAt(doc, doc.indexOf("prose"))).toBeNull();
  });

  test("a pipe line with no delimiter row is not a table", () => {
    const text = "a | b\nc | d";
    expect(findTableAt(text, 2)).toBeNull();
  });
});

describe("formatAllTables", () => {
  test("formats every table and leaves everything else byte-identical", () => {
    const text = [
      "# Doc",
      "",
      "|a|b|",
      "|-|-|",
      "|1|2|",
      "",
      "Some prose with a | pipe in it.",
      "",
      "|x|",
      "|-|",
      "|long value|",
    ].join("\n");
    const out = formatAllTables(text);
    expect(out).toContain("| a   | b   |");
    expect(out).toContain("| long value |");
    expect(out).toContain("Some prose with a | pipe in it.");
    expect(out.split("\n").length).toBe(text.split("\n").length);
  });
});
