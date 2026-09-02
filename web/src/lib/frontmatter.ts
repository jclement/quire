// Splitting a document into its frontmatter block and body, the way the
// server does (vault.SplitFrontmatter): the editor shows and edits only the
// body — frontmatter is data the app owns and edits through the UI — and
// the block is stitched back on before every save.
export interface SplitDocument {
  /** The `---` block including both fences and the trailing newline; "" when none. */
  head: string;
  body: string;
}

export function splitFrontmatter(text: string): SplitDocument {
  if (!text.startsWith("---\n") && !text.startsWith("---\r\n")) {
    return { head: "", body: text };
  }
  const lines = text.split("\n");
  for (let i = 1; i < lines.length; i++) {
    if (lines[i]!.replace(/\r$/, "") === "---") {
      const head = lines.slice(0, i + 1).join("\n") + "\n";
      return { head, body: text.slice(head.length) };
    }
  }
  // An opening fence with no close is not frontmatter; it is text.
  return { head: "", body: text };
}

/** The number of lines the frontmatter block occupies (0 when none). */
export function frontmatterLines(text: string): number {
  const { head } = splitFrontmatter(text);
  return head === "" ? 0 : head.split("\n").length - 1;
}
