// Heading extraction for the document outline (table of contents), and the
// slug function that gives rendered headings their anchor ids. Markdown.tsx
// slugs headings with the same function so the outline's links resolve.

export interface Heading {
  /** 1–6, from the number of leading #. */
  level: number;
  text: string;
  /** Anchor id; unique within one document. */
  id: string;
  /** 1-based source line — how the outline scrolls the editor. */
  line: number;
}

/** Minimum headings before an outline is worth showing (a lone H1 is noise). */
export const MIN_OUTLINE_HEADINGS = 2;

const ATX_HEADING = /^(#{1,6})\s+(.+?)\s*#*\s*$/;
const FENCE = /^\s{0,3}(`{3,}|~{3,})/;

/**
 * GitHub-style slug: lowercase, punctuation dropped, spaces to dashes.
 * Exported so the renderer and the outline agree on ids.
 */
export function slugifyHeading(text: string): string {
  return text
    .toLowerCase()
    .replace(/[^\p{L}\p{N}\s-]/gu, "")
    .trim()
    .replace(/\s+/g, "-");
}

/**
 * Headings in source order, skipping fenced code blocks (a `#` comment inside
 * a shell fence is not a heading) and YAML frontmatter. Duplicate slugs get a
 * numeric suffix, matching how anchors are normally disambiguated.
 */
export function extractHeadings(markdown: string): Heading[] {
  const lines = markdown.split("\n");
  const headings: Heading[] = [];
  const seen = new Map<string, number>();
  let fence: string | null = null;
  let inFrontmatter = lines[0]?.trim() === "---";

  for (const [index, line] of lines.entries()) {
    if (inFrontmatter) {
      // The opening --- is line 0; the next --- closes the block.
      if (index > 0 && line.trim() === "---") inFrontmatter = false;
      continue;
    }
    const fenceMatch = FENCE.exec(line);
    if (fenceMatch) {
      const marker = fenceMatch[1]!;
      if (fence === null) fence = marker[0]!;
      else if (marker[0] === fence) fence = null;
      continue;
    }
    if (fence !== null) continue;

    const match = ATX_HEADING.exec(line);
    if (!match) continue;
    const text = match[2]!.trim();
    const base = slugifyHeading(text) || "section";
    const count = seen.get(base) ?? 0;
    seen.set(base, count + 1);
    headings.push({
      level: match[1]!.length,
      text,
      id: count === 0 ? base : `${base}-${count}`,
      line: index + 1,
    });
  }
  return headings;
}
