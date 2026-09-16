// `==highlight==` taught to the markdown parser the editor uses, so the
// source is coloured the way the rendered page paints it. The renderer's
// half lives in lib/remarkQuire.ts; without this the editor showed plain
// `==text==` while the page showed a highlight.
//
// Modelled on @lezer/markdown's own Strikethrough extension, which is the
// same shape with `~~`. The flanking rules are deliberately simpler than
// CommonMark's: a highlight opens on a non-space and closes on a non-space,
// which is exactly the regex the renderer uses — the two must never
// disagree about what counts as one.
import { Tag, tags } from "@lezer/highlight";
import type { MarkdownConfig } from "@lezer/markdown";

/** Its own tag: no built-in one means "the author marked this passage". */
export const highlightTag = Tag.define("highlight");

const HighlightDelim = { resolve: "Highlight", mark: "HighlightMark" };

const EQUALS = 61; // '='

export const highlightMark: MarkdownConfig = {
  defineNodes: [
    { name: "Highlight", style: { "Highlight/...": highlightTag } },
    { name: "HighlightMark", style: tags.processingInstruction },
  ],
  parseInline: [
    {
      name: "Highlight",
      parse(cx, next, pos) {
        // Exactly two: `===` is a setext heading rule, not a highlight.
        if (
          next != EQUALS ||
          cx.char(pos + 1) != EQUALS ||
          cx.char(pos + 2) == EQUALS
        ) {
          return -1;
        }
        const spaceBefore = /\s|^$/.test(cx.slice(pos - 1, pos));
        const spaceAfter = /\s|^$/.test(cx.slice(pos + 2, pos + 3));
        // Can open when something follows it, can close when something
        // precedes it. `a == b` is arithmetic and satisfies neither.
        return cx.addDelimiter(
          HighlightDelim,
          pos,
          pos + 2,
          !spaceAfter,
          !spaceBefore,
        );
      },
      after: "Emphasis",
    },
  ],
};
