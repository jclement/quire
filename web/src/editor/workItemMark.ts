// Work-item references (AB#2433, #2433) taught to the editor's markdown
// parser, so a ticket number reads as a reference in the source the way it
// renders as a link on the page. The renderer's half is lib/workItems.ts
// and remarkQuire; the boundary rules here mirror that regex — the two must
// never disagree about what counts as one. Highlighted whether or not a
// link template is set: it is still a reference either way.
import { tags } from "@lezer/highlight";
import type { MarkdownConfig } from "@lezer/markdown";

const HASH = 35; // '#'
const UPPER_A = 65; // 'A'

/** Long enough for any real item number; bounds the lookahead slice. */
const MAX_REFERENCE_LENGTH = 24;

const REFERENCE_RE = /^(?:AB#|#)\d+(?![\p{L}\p{N}_-])/u;

export const workItemMark: MarkdownConfig = {
  defineNodes: [{ name: "WorkItem", style: tags.labelName }],
  parseInline: [
    {
      name: "WorkItem",
      parse(cx, next, pos) {
        if (next != HASH && next != UPPER_A) return -1;
        const before = cx.slice(pos - 1, pos);
        if (before !== "" && !/[\s([]/.test(before)) return -1;
        const match = REFERENCE_RE.exec(
          cx.slice(pos, Math.min(cx.end, pos + MAX_REFERENCE_LENGTH)),
        );
        if (!match) return -1;
        return cx.addElement(cx.elt("WorkItem", pos, pos + match[0].length));
      },
    },
  ],
};
