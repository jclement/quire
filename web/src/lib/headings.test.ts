// Tests for outline extraction. The branches that matter are the ones a naive
// line-scan gets wrong: `#` inside fenced code, frontmatter, and duplicate
// heading text producing colliding anchor ids.
import { describe, expect, test } from "bun:test";
import { extractHeadings, slugifyHeading } from "./headings.ts";

describe("slugifyHeading", () => {
  test("lowercases, strips punctuation, dashes spaces", () => {
    expect(slugifyHeading("Why *this* matters!")).toBe("why-this-matters");
    // Removed punctuation leaves a whitespace run, which collapses to one dash.
    expect(slugifyHeading("API / v1: notes")).toBe("api-v1-notes");
  });

  test("keeps letters and numbers, including non-ASCII", () => {
    expect(slugifyHeading("Étape 2")).toBe("étape-2");
  });
});

describe("extractHeadings", () => {
  test("captures level, text, and 1-based line", () => {
    const headings = extractHeadings("# Title\n\ntext\n\n## Section\n");
    expect(headings).toEqual([
      { level: 1, text: "Title", id: "title", line: 1 },
      { level: 2, text: "Section", id: "section", line: 5 },
    ]);
  });

  test("ignores # inside fenced code blocks", () => {
    const markdown = [
      "# Real",
      "```bash",
      "# not a heading",
      "```",
      "## Also real",
      "~~~",
      "### tilde-fenced, not a heading",
      "~~~",
    ].join("\n");
    expect(extractHeadings(markdown).map((heading) => heading.text)).toEqual([
      "Real",
      "Also real",
    ]);
  });

  test("skips YAML frontmatter", () => {
    const markdown = "---\ntitle: Note\ntags: [a]\n---\n\n# Heading\n";
    const headings = extractHeadings(markdown);
    expect(headings).toHaveLength(1);
    expect(headings[0]).toMatchObject({ text: "Heading", line: 6 });
  });

  test("disambiguates duplicate heading text", () => {
    const headings = extractHeadings("## Notes\n\n## Notes\n\n## Notes\n");
    expect(headings.map((heading) => heading.id)).toEqual([
      "notes",
      "notes-1",
      "notes-2",
    ]);
  });

  test("requires a space after the hashes and caps at six", () => {
    const markdown = "#nospace\n\n####### seven\n\n###### six\n";
    expect(extractHeadings(markdown).map((heading) => heading.level)).toEqual([
      6,
    ]);
  });

  test("strips closing hashes of a closed ATX heading", () => {
    expect(extractHeadings("## Middle ##\n")[0]?.text).toBe("Middle");
  });

  test("returns nothing for prose with no headings", () => {
    expect(extractHeadings("Just a paragraph.\n")).toEqual([]);
  });
});
