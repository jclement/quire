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

export function remarkQuire() {
  return (tree: Root) => {
    transformWikilinks(tree);
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
