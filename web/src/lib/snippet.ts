// Search snippet parsing. The server marks hits with literal <mark>…</mark>
// tags inside otherwise-plain text; we parse those markers ourselves and render
// everything else as text, so no server HTML ever reaches
// dangerouslySetInnerHTML.

export interface SnippetSegment {
  text: string;
  /** True when this run was inside <mark> tags. */
  mark: boolean;
}

const MARK_PATTERN = /<mark>(.*?)<\/mark>/gs;

/** Splits a snippet into plain and highlighted runs, dropping empty ones. */
export function parseSnippet(snippet: string): SnippetSegment[] {
  const segments: SnippetSegment[] = [];
  let last = 0;
  for (const match of snippet.matchAll(MARK_PATTERN)) {
    if (match.index > last) {
      segments.push({ text: snippet.slice(last, match.index), mark: false });
    }
    if (match[1]!.length > 0) segments.push({ text: match[1]!, mark: true });
    last = match.index + match[0].length;
  }
  if (last < snippet.length) {
    segments.push({ text: snippet.slice(last), mark: false });
  }
  return segments;
}
