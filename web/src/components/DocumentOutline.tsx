// The floating heading outline (table of contents) beside a document on lg+.
// A quiet margin note, not a sidebar: sticky, dense, indented by level, with
// the section you're reading in accent. In read mode it follows the rendered
// headings via IntersectionObserver; in edit/split it follows the editor's top
// visible line and clicking scrolls the editor instead.
import { useEffect, useState } from "react";
import type { Heading } from "../lib/headings.ts";
import { MIN_OUTLINE_HEADINGS } from "../lib/headings.ts";

interface DocumentOutlineProps {
  headings: Heading[];
  /** Read mode observes the DOM; edit/split is driven by `activeLine`. */
  mode: "rendered" | "source";
  /** Editor's top visible line, when mode is "source". */
  activeLine?: number;
  /** Called with the heading's source line in "source" mode. */
  onScrollToLine?: (line: number) => void;
}

/** Left padding per heading level — H1 flush, deeper levels stepped in. */
const INDENT = ["pl-0", "pl-0", "pl-2", "pl-4", "pl-6", "pl-8", "pl-8"];

export function DocumentOutline({
  headings,
  mode,
  activeLine,
  onScrollToLine,
}: DocumentOutlineProps) {
  const activeId = useActiveHeading(headings, mode, activeLine);

  if (headings.length < MIN_OUTLINE_HEADINGS) return null;

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
    <nav
      aria-label="Document outline"
      className="sticky top-4 hidden w-44 shrink-0 self-start lg:block"
    >
      <h2 className="mb-1.5 text-[10px] font-semibold tracking-wider text-muted uppercase">
        On this page
      </h2>
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
