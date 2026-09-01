// Wikilink text parsing: splits `[[Target]]` / `[[Target|Alias]]` out of plain
// text. Pure and testable; remarkQuire.ts uses it on mdast text nodes (so code
// spans and fenced blocks are never touched), and the editor uses raw() when
// inserting completions.

export interface WikilinkTextSegment {
  kind: "text";
  text: string;
}

export interface WikilinkLinkSegment {
  kind: "link";
  /** The target part before any `|` — what the server resolves. */
  target: string;
  /** What the reader sees: the alias if present, else the target. */
  display: string;
  /** Full inner text between the brackets, e.g. "Target|Alias". */
  inner: string;
}

export type WikilinkSegment = WikilinkTextSegment | WikilinkLinkSegment;

const WIKILINK_PATTERN = /\[\[([^[\]|]+)(?:\|([^[\]]+))?\]\]/g;

/** Splits text into literal runs and wikilinks, in order. */
export function splitWikilinks(text: string): WikilinkSegment[] {
  const segments: WikilinkSegment[] = [];
  let last = 0;
  for (const match of text.matchAll(WIKILINK_PATTERN)) {
    if (match.index > last) {
      segments.push({ kind: "text", text: text.slice(last, match.index) });
    }
    const target = match[1]!.trim();
    const alias = match[2]?.trim();
    segments.push({
      kind: "link",
      target,
      display: alias || target,
      inner: match[0].slice(2, -2),
    });
    last = match.index + match[0].length;
  }
  if (last < text.length) {
    segments.push({ kind: "text", text: text.slice(last) });
  }
  return segments;
}
