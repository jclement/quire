// Full search page. The input is seeded from ?q= and writes back to it
// (shareable searches); the query grammar (type:meeting tag:x is:task …)
// passes through to the server verbatim. Snippets arrive with literal <mark>
// tags that we parse ourselves — no HTML injection.
import { useNavigate } from "@tanstack/react-router";
import {
  CheckSquare,
  SearchX,
  Search as SearchIcon,
  Sparkles,
} from "lucide-react";
import { useEffect, useState } from "react";
import {
  useSearch as useSearchQuery,
  useSemanticEnabled,
} from "../api/queries.ts";
import type { SearchMode, SearchResult } from "../api/types.ts";
import { docHref, DOC_TYPE_INFO } from "../lib/docs.ts";
import { parseSnippet } from "../lib/snippet.ts";
import { useDebouncedValue } from "../lib/useDebouncedValue.ts";
import { noAutofill } from "../lib/noAutofill.ts";
import { useListNav } from "../keys/useListNav.ts";
import { ErrorState, EmptyState } from "../components/EmptyState.tsx";
import { SkeletonRows } from "../components/Skeleton.tsx";

const URL_SYNC_DEBOUNCE_MS = 300;

export function SearchPage({
  initialQuery,
  initialMode = "text",
}: {
  initialQuery: string;
  initialMode?: SearchMode;
}) {
  const navigate = useNavigate();
  const [query, setQuery] = useState(initialQuery);
  const semanticAvailable = useSemanticEnabled();
  const [mode, setMode] = useState<SearchMode>(initialMode);
  const effectiveMode: SearchMode = semanticAvailable ? mode : "text";
  const debounced = useDebouncedValue(query, URL_SYNC_DEBOUNCE_MS);
  const results = useSearchQuery(debounced.trim(), effectiveMode);

  // Keep ?q= (and ?mode=) in sync so a search survives reload/share.
  useEffect(() => {
    void navigate({
      to: "/search",
      search: {
        ...(debounced ? { q: debounced } : {}),
        ...(effectiveMode === "semantic" ? { mode: "semantic" as const } : {}),
      },
      replace: true,
    });
  }, [debounced, effectiveMode, navigate]);

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-2 border-b border-border pb-2 focus-within:border-accent">
        <SearchIcon className="size-4 shrink-0 text-muted" aria-hidden="true" />
        <input
          id="global-search-input"
          // Autofocus is the point of a search page; safe on a dedicated route.
          // eslint-disable-next-line jsx-a11y/no-autofocus
          autoFocus
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          onKeyDown={(event) => {
            // Escape hands control back to list navigation.
            if (event.key === "Escape") event.currentTarget.blur();
          }}
          placeholder="Search… (type:meeting sarah tag:x is:task)"
          aria-label="Search query"
          {...noAutofill("search")}
          className="field-bare h-9 w-full bg-transparent text-sm text-heading outline-none placeholder:text-muted"
        />
        {semanticAvailable ? (
          <button
            type="button"
            role="switch"
            aria-checked={effectiveMode === "semantic"}
            aria-label="Semantic search"
            title="Search by meaning (embeddings) instead of exact words"
            onClick={() =>
              setMode(effectiveMode === "semantic" ? "text" : "semantic")
            }
            className={`flex h-7 shrink-0 items-center gap-1 rounded border px-2 text-xs ${
              effectiveMode === "semantic"
                ? "border-accent bg-accent/10 text-accent"
                : "border-border text-muted hover:text-heading"
            }`}
          >
            <Sparkles className="size-3.5" aria-hidden="true" />
            Semantic
          </button>
        ) : null}
      </div>
      {results.isLoading ? (
        <SkeletonRows />
      ) : results.isError ? (
        <ErrorState error={results.error} />
      ) : !results.data ? (
        <p className="py-8 text-center text-xs text-muted">
          {effectiveMode === "semantic"
            ? "Describe what you're looking for — results are ranked by meaning, not exact words."
            : "Search titles and full text. Filters: type: tag: is:task"}
        </p>
      ) : results.data.length === 0 ? (
        <EmptyState
          icon={SearchX}
          title="No results"
          hint="Try fewer words or a filter."
        />
      ) : (
        <ResultList results={results.data} />
      )}
    </div>
  );
}

function ResultList({ results }: { results: SearchResult[] }) {
  const navigate = useNavigate();
  const nav = useListNav({
    items: results,
    onOpen: (result) => void navigate({ to: docHref(result.path) }),
  });
  return (
    <ul className="divide-y divide-border border-y border-border">
      {results.map((result, at) => {
        // `is:task` hits come back as type "task", which is not a document
        // type — before the generated types this indexed to undefined and
        // crashed the results list.
        const Icon =
          result.type === "task"
            ? CheckSquare
            : DOC_TYPE_INFO[result.type].icon;
        return (
          <li
            key={result.path}
            ref={nav.rowRef(at)}
            tabIndex={-1}
            onClick={() => {
              nav.setIndex(at);
              void navigate({ to: docHref(result.path) });
            }}
            className={`cursor-pointer px-2 py-2 outline-none ${
              at === nav.index ? "bg-selected" : "hover:bg-hover"
            }`}
          >
            <p className="flex items-center gap-2 text-sm font-medium text-heading">
              <Icon
                className="size-3.5 shrink-0 text-muted"
                aria-hidden="true"
              />
              <span className="truncate">{result.title}</span>
              <span className="ml-auto shrink-0 font-mono text-[10px] uppercase text-muted">
                {result.type}
              </span>
            </p>
            <p className="mt-0.5 line-clamp-2 pl-5.5 text-xs text-muted">
              <Snippet snippet={result.snippet} />
            </p>
          </li>
        );
      })}
    </ul>
  );
}

function Snippet({ snippet }: { snippet: string }) {
  return (
    <>
      {parseSnippet(snippet).map((segment, at) =>
        segment.mark ? (
          <mark key={at} className="rounded-sm bg-selected px-0.5 text-heading">
            {segment.text}
          </mark>
        ) : (
          <span key={at}>{segment.text}</span>
        ),
      )}
    </>
  );
}
