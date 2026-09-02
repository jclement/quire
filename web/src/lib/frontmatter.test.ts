import { describe, expect, test } from "bun:test";
import { frontmatterLines, splitFrontmatter } from "./frontmatter.ts";

describe("splitFrontmatter", () => {
  test("separates the block and stitches back exactly", () => {
    const text = "---\ntitle: X\ntags: [a]\n---\n# X\n\nbody\n";
    const { head, body } = splitFrontmatter(text);
    expect(head).toBe("---\ntitle: X\ntags: [a]\n---\n");
    expect(body).toBe("# X\n\nbody\n");
    expect(head + body).toBe(text);
    expect(frontmatterLines(text)).toBe(4);
  });
  test("no frontmatter, or an unclosed fence, is all body", () => {
    expect(splitFrontmatter("# plain\n")).toEqual({
      head: "",
      body: "# plain\n",
    });
    expect(splitFrontmatter("---\nnot closed\n")).toEqual({
      head: "",
      body: "---\nnot closed\n",
    });
    expect(frontmatterLines("# plain")).toBe(0);
  });
});
