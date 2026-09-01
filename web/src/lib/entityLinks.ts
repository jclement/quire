// Which frontmatter keys the properties strip offers as editable
// relationships, per document type, and how to read the links already stored
// under one. The server owns the writing rules (POST /link is idempotent and
// knows that `company`/`project`/`owner` are scalars while the rest are
// lists); this file only decides what a document of a given type is asked
// about, and which type each key's typeahead searches.
import type { DocType, Link } from "../api/types.ts";
import { splitWikilinks } from "./wikilinks.ts";

export interface LinkKey {
  /** The frontmatter key, e.g. "people". */
  key: string;
  /** The type its typeahead searches — one key links one kind of thing. */
  type: DocType;
  /** Scalar keys hold a single link; adding a second would replace it. */
  singular: boolean;
}

const COMPANY: LinkKey = { key: "company", type: "company", singular: true };
const PROJECT: LinkKey = { key: "project", type: "project", singular: true };
const PEOPLE: LinkKey = { key: "people", type: "person", singular: false };

// Companies are the leaf of the graph: people, projects and meetings point at
// them, so a company page needs nothing to point at itself. Notes and daily
// notes carry no entity frontmatter at all (DESIGN.md's schemas).
const LINK_KEYS: Partial<Record<DocType, LinkKey[]>> = {
  meeting: [PEOPLE, PROJECT, COMPANY],
  person: [COMPANY],
  project: [COMPANY, PEOPLE],
};

/** The relationships offered for a document type, in display order. */
export function linkKeysFor(type: DocType): LinkKey[] {
  return LINK_KEYS[type] ?? [];
}

/**
 * The link targets stored under one frontmatter key. Values arrive as
 * wikilinks — a list (`people: ["[[Sarah Chen]]"]`) or, for scalar keys, one
 * on its own — and an aliased link resolves by its target. A bare string that
 * is not a wikilink still counts: someone typed it by hand and the server
 * will wrap it on the next write.
 */
export function linkTargets(value: unknown): string[] {
  const items = Array.isArray(value) ? value : [value];
  const targets: string[] = [];
  for (const item of items) {
    if (typeof item !== "string") continue;
    let found = false;
    for (const segment of splitWikilinks(item)) {
      if (segment.kind !== "link") continue;
      targets.push(segment.target);
      found = true;
    }
    if (!found && item.trim()) targets.push(item.trim());
  }
  return targets;
}

/**
 * Wikilink text → the vault path it resolves to, from a document's own links
 * (frontmatter links are indexed, so a `company:` value is in there). Dangling
 * links are left out: their chips have nowhere to go.
 */
export function resolvedTargets(links: Link[]): Map<string, string> {
  const resolved = new Map<string, string>();
  for (const link of links) {
    if (link.target) resolved.set(link.raw.toLowerCase(), link.target);
  }
  return resolved;
}
