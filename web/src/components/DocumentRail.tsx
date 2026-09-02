// The quiet margin column beside a document on lg+: the heading outline, then
// the document's backlinks. Both are margin notes, not a sidebar — dense,
// hairline-ruled, and a sibling column rather than an overlay, so it shortens
// the content instead of floating over it and the page can never scroll
// sideways. With neither section worth showing it renders nothing at all and
// the content column takes the width back.
//
// The outline follows the reader: in read mode via IntersectionObserver over
// the rendered headings, in edit/split via the editor's top visible line
// (where clicking scrolls the editor instead of the page).
import { Link as RouterLink } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import type { DocMeta, SearchResult } from "../api/types.ts";
import { DOC_TYPE_INFO, docHref, isDocType } from "../lib/docs.ts";
import type { Heading } from "../lib/headings.ts";
import { MIN_OUTLINE_HEADINGS } from "../lib/headings.ts";

interface DocumentRailProps {
  headings: Heading[];
  /** Read mode observes the DOM; edit/split is driven by `activeLine`. */
  mode: "rendered" | "source";
  /** Editor's top visible line, when mode is "source". */
  activeLine?: number;
  /** Called with the heading's source line in "source" mode. */
  onScrollToLine?: (line: number) => void;
  /** Documents linking here; the page repeats these below lg. */
  backlinks: DocMeta[];
  /** Nearest documents by meaning (semantic search on); empty otherwise. */
  related?: SearchResult[];
}

/** Left padding per heading level — H1 flush, deeper levels stepped in. */
const INDENT = ["pl-0", "pl-0", "pl-2", "pl-4", "pl-6", "pl-8", "pl-8"];

export function DocumentRail({
  headings,
  mode,
  activeLine,
  onScrollToLine,
  backlinks,
  related = [],
}: DocumentRailProps) {
  const activeId = useActiveHeading(headings, mode, activeLine);

  const showOutline = headings.length >= MIN_OUTLINE_HEADINGS;
  if (!showOutline && backlinks.length === 0 && related.length === 0) {
    return null;
  }

  const go = (heading: Heading) => {
    if (mode === "source") {
      onScrollToLine?.(heading.line);
      return;
    }
    document
      .getElementById(heading.id)
      ?.scrollIntoView({ behavior: "smooth", block: "start" });
  };

  return (
    <div className="sticky top-4 hidden w-44 shrink-0 flex-col gap-5 self-start lg:flex print:hidden">
      {showOutline ? (
        <nav aria-label="Document outline">
          <RailHeading>On this page</RailHeading>
          <ul className="border-l border-border">
            {headings.map((heading) => (
              <li key={heading.id}>
                <button
                  type="button"
                  onClick={() => go(heading)}
                  title={heading.text}
                  className={`-ml-px block w-full border-l py-0.5 pr-1 text-left text-xs leading-snug ${
                    INDENT[heading.level]
                  } ${
                    heading.id === activeId
                      ? "border-accent text-accent"
                      : "border-transparent text-muted hover:border-border hover:text-body"
                  }`}
                >
                  <span className="line-clamp-2">{heading.text}</span>
                </button>
              </li>
            ))}
          </ul>
        </nav>
      ) : null}

      {backlinks.length > 0 ? (
        <nav aria-label="Backlinks">
          <RailHeading>Linked from</RailHeading>
          <ul className="border-l border-border">
            {backlinks.map((backlink) => {
              // Lead with the document type, the way the nav and every list in
              // the app does — a column of bare titles gives no clue whether
              // you are looking at a person, a meeting or a note.
              const Icon = isDocType(backlink.type)
                ? DOC_TYPE_INFO[backlink.type].icon
                : undefined;
              return (
                <li key={backlink.path}>
                  <RouterLink
                    to={docHref(backlink.path)}
                    title={`${backlink.title} (${backlink.type})`}
                    className="-ml-px flex items-start gap-1.5 border-l border-transparent py-0.5 pr-1 pl-2 text-xs leading-snug text-muted hover:border-border hover:text-body"
                  >
                    {Icon ? (
                      <Icon
                        className="mt-0.5 size-3 shrink-0"
                        aria-hidden="true"
                      />
                    ) : null}
                    <span className="line-clamp-2">{backlink.title}</span>
                  </RouterLink>
                </li>
              );
            })}
          </ul>
        </nav>
      ) : null}

      {related.length > 0 ? (
        <nav aria-label="Related documents">
          <RailHeading>Related</RailHeading>
          <ul className="border-l border-border">
            {related.map((hit) => {
              const Icon = isDocType(hit.type)
                ? DOC_TYPE_INFO[hit.type].icon
                : undefined;
              return (
                <li key={hit.path}>
                  <RouterLink
                    to={docHref(hit.path)}
                    title={`${hit.title} (${hit.type}) — similarity ${hit.score ? hit.score.toFixed(2) : "?"}`}
                    className="-ml-px flex items-start gap-1.5 border-l border-transparent py-0.5 pr-1 pl-2 text-xs leading-snug text-muted hover:border-border hover:text-body"
                  >
                    {Icon ? (
                      <Icon
                        className="mt-0.5 size-3 shrink-0"
                        aria-hidden="true"
                      />
                    ) : null}
                    <span className="line-clamp-2">{hit.title}</span>
                  </RouterLink>
                </li>
              );
            })}
          </ul>
        </nav>
      ) : null}
    </div>
  );
}

function RailHeading({ children }: { children: string }) {
  return (
    <h2 className="mb-1.5 text-[10px] font-semibold tracking-wider text-muted uppercase">
      {children}
    </h2>
  );
}

/** The heading currently being read, by whichever signal the mode provides. */
function useActiveHeading(
  headings: Heading[],
  mode: "rendered" | "source",
  activeLine: number | undefined,
): string | null {
  const [visibleId, setVisibleId] = useState<string | null>(null);

  useEffect(() => {
    if (mode !== "rendered" || headings.length === 0) return;
    const elements = headings
      .map((heading) => document.getElementById(heading.id))
      .filter((element): element is HTMLElement => element !== null);
    if (elements.length === 0) return;

    // Headings are "current" from the moment they reach the top band of the
    // viewport; the bottom margin keeps later ones from stealing the highlight.
    const observer = new IntersectionObserver(
      (entries) => {
        const onScreen = entries.filter((entry) => entry.isIntersecting);
        if (onScreen.length === 0) return;
        const topmost = onScreen.reduce((best, entry) =>
          entry.boundingClientRect.top < best.boundingClientRect.top
            ? entry
            : best,
        );
        setVisibleId(topmost.target.id);
      },
      { rootMargin: "0px 0px -70% 0px", threshold: 0 },
    );
    for (const element of elements) observer.observe(element);
    return () => observer.disconnect();
  }, [headings, mode]);

  if (mode === "source") {
    if (activeLine === undefined) return null;
    // The last heading at or above the editor's top visible line.
    let current: string | null = null;
    for (const heading of headings) {
      if (heading.line <= activeLine) current = heading.id;
      else break;
    }
    return current ?? headings[0]?.id ?? null;
  }
  return visibleId ?? headings[0]?.id ?? null;
}
