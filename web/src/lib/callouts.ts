// Obsidian callout parsing: `> [!NOTE] Optional title` at the start of a
// blockquote turns it into a styled callout. This module is the pure grammar
// (testable under bun); the mdast transform that applies it lives in
// remarkQuire.ts and the visuals in components/Markdown.tsx.

export const CALLOUT_TYPES = [
  "note",
  "info",
  "tip",
  "warning",
  "danger",
  "question",
  "success",
  "example",
] as const;

export type CalloutType = (typeof CALLOUT_TYPES)[number];

/** Obsidian-compatible aliases folded into our eight canonical types. */
const CALLOUT_ALIASES: Record<string, CalloutType> = {
  abstract: "note",
  summary: "note",
  todo: "note",
  hint: "tip",
  important: "tip",
  caution: "warning",
  attention: "warning",
  error: "danger",
  bug: "danger",
  failure: "danger",
  help: "question",
  faq: "question",
  check: "success",
  done: "success",
  quote: "note",
};

export interface CalloutMarker {
  type: CalloutType;
  /** Explicit title after the marker, or "" (render the type name instead). */
  title: string;
}

const MARKER_PATTERN = /^\[!([a-z-]+)\][+-]?[ \t]*(.*)$/i;

/**
 * Parses the first line of a blockquote for a callout marker. Returns null when
 * the line is not a marker; unknown types fall back to "note" (Obsidian's
 * behavior) so authors never lose content to a typo.
 */
export function parseCalloutMarker(firstLine: string): CalloutMarker | null {
  const match = MARKER_PATTERN.exec(firstLine.trim());
  if (!match) return null;
  const rawType = match[1]!.toLowerCase();
  const known = (CALLOUT_TYPES as readonly string[]).includes(rawType)
    ? (rawType as CalloutType)
    : CALLOUT_ALIASES[rawType];
  return { type: known ?? "note", title: match[2]!.trim() };
}
