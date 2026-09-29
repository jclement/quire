// Work-item references in prose and task text: "AB#2433" (Azure Boards'
// own spelling) and a purely numeric "#2433". The numeric form is exactly
// what is *not* a tag — a tag needs a letter, as in Obsidian and in the
// server's index — so the two never compete for the same text. Whether they
// become links is the owner's call in Settings (a URL template with {id});
// with none set they stay plain text.

/** URL prefix carrying a work-item id from remarkQuire to the <a> renderer. */
export const WORK_ITEM_HREF_PREFIX = "#workitem:";

// Starts at a boundary (line start, space, or an opening bracket) so
// "page#123" is an anchor and "ZAB#1" is a word; ends before any word
// character so "#123abc" and "#12-3" stay text.
const WORK_ITEM_RE = /(^|[\s([])(AB#|#)(\d+)(?![\p{L}\p{N}_-])/gu;

export type WorkItemSegment =
  | { kind: "text"; text: string }
  | { kind: "workitem"; id: string; label: string };

/** Splits text into plain runs and work-item references, in order. */
export function splitWorkItems(text: string): WorkItemSegment[] {
  const out: WorkItemSegment[] = [];
  let last = 0;
  for (const match of text.matchAll(WORK_ITEM_RE)) {
    const start = match.index + match[1]!.length;
    if (start > last) out.push({ kind: "text", text: text.slice(last, start) });
    const label = match[2]! + match[3]!;
    out.push({ kind: "workitem", id: match[3]!, label });
    last = start + label.length;
  }
  if (last < text.length || out.length === 0) {
    out.push({ kind: "text", text: text.slice(last) });
  }
  return out;
}

/** The item's URL from the Settings template, or null when links are off. */
export function workItemUrl(template: string, id: string): string | null {
  if (!template) return null;
  return template.replaceAll("{id}", encodeURIComponent(id));
}
