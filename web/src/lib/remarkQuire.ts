// remark plugin adding quire's markdown flavor on top of GFM: wikilinks and
// Obsidian callouts. Operating at the mdast level (rather than preprocessing
// the source string) means code spans and fenced blocks are naturally exempt,
// and source line numbers stay intact for task-checkbox mapping.
import type { Blockquote, Paragraph, Root, Text } from "mdast";
import { visit } from "unist-util-visit";
import { parseCalloutMarker } from "./callouts.ts";
import { splitWikilinks } from "./wikilinks.ts";

/** URL prefix carrying the wikilink inner text to the <a> renderer. A fragment
 * survives react-markdown's default URL sanitizer where a custom scheme would
 * be stripped. */
export const WIKILINK_HREF_PREFIX = "#wikilink:";

/** URL prefix carrying a #tag to the <a> renderer, which routes it to search. */
export const TAG_HREF_PREFIX = "#tag:";

// A tag: `#` at a word boundary, then letters/digits/-/_/slash. Purely
// numeric "#123" is an issue reference in most people's muscle memory, not
// a tag, so it is excluded — the same rule Obsidian applies.
const TAG_RE = /(^|[\s(])#([\p{L}\p{N}_\/-]*\p{L}[\p{L}\p{N}_\/-]*)/gu;

export function remarkQuire() {
  return (tree: Root) => {
    transformWikilinks(tree);
    transformHashtags(tree);
    transformCallouts(tree);
  };
}

/** Splits `[[…]]` out of text nodes into link nodes the renderer can resolve. */
function transformWikilinks(tree: Root): void {
  visit(tree, "text", (node: Text, index, parent) => {
    if (!parent || index === undefined) return;
    const segments = splitWikilinks(node.value);
    if (segments.length === 1 && segments[0]?.kind === "text") return;
    const replacements = segments.map((segment) =>
      segment.kind === "text"
        ? ({ type: "text", value: segment.text } as Text)
        : {
            type: "link" as const,
            url: WIKILINK_HREF_PREFIX + encodeURIComponent(segment.inner),
            children: [{ type: "text", value: segment.display } as Text],
          },
    );
    parent.children.splice(index, 1, ...replacements);
    // Skip over what we just inserted so we don't re-visit it.
    return index + replacements.length;
  });
}

/** Turns `#tag` in prose into a link the renderer routes to a tag search. */
function transformHashtags(tree: Root): void {
  visit(tree, "text", (node: Text, index, parent) => {
    if (!parent || index === undefined) return;
    // Inside a link already (a wikilink we just made, or a real one) the
    // text is the link's label, not prose.
    if (parent.type === "link") return;
    const value = node.value;
    if (!value.includes("#")) return;
    const pieces: Array<
      Text | { type: "link"; url: string; children: Text[] }
    > = [];
    let last = 0;
    for (const match of value.matchAll(TAG_RE)) {
      const start = match.index + match[1]!.length;
      if (start > last)
        pieces.push({ type: "text", value: value.slice(last, start) } as Text);
      pieces.push({
        type: "link",
        url: TAG_HREF_PREFIX + encodeURIComponent(match[2]!),
        children: [{ type: "text", value: "#" + match[2]! } as Text],
      });
      last = start + 1 + match[2]!.length;
    }
    if (pieces.length === 0) return;
    if (last < value.length)
      pieces.push({ type: "text", value: value.slice(last) } as Text);
    parent.children.splice(index, 1, ...(pieces as never[]));
    return index + pieces.length;
  });
}

/** Tags callout blockquotes with data attributes and strips the marker line. */
function transformCallouts(tree: Root): void {
  visit(tree, "blockquote", (node: Blockquote) => {
    const paragraph = node.children[0];
    if (paragraph?.type !== "paragraph") return;
    const firstText = paragraph.children[0];
    if (firstText?.type !== "text") return;

    const newline = firstText.value.indexOf("\n");
    const firstLine =
      newline === -1 ? firstText.value : firstText.value.slice(0, newline);
    const marker = parseCalloutMarker(firstLine);
    if (!marker) return;

    node.data = {
      ...node.data,
      hProperties: {
        "data-callout": marker.type,
        "data-callout-title": marker.title,
      },
    };
    stripMarkerLine(paragraph, firstText, newline);
  });
}

/** Removes the `[!type] title` line, leaving the callout body. */
function stripMarkerLine(
  paragraph: Paragraph,
  firstText: Text,
  newline: number,
): void {
  if (newline === -1) {
    paragraph.children.shift();
    // The title line may be the whole paragraph; drop trailing break nodes so
    // an empty <p> isn't rendered.
    while (paragraph.children[0]?.type === "break") paragraph.children.shift();
  } else {
    firstText.value = firstText.value.slice(newline + 1);
  }
}
