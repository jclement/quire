// Full search page. The input is seeded from ?q= and writes back to it
// (shareable searches); the query grammar (type:meeting tag:x is:task …)
// passes through to the server verbatim. Snippets arrive with literal <mark>
// tags that we parse ourselves — no HTML injection.
import { useNavigate } from "@tanstack/react-router";
import { SearchX, Search as SearchIcon } from "lucide-react";
import { useEffect, useState } from "react";
import { useSearch as useSearchQuery } from "../api/queries.ts";
import type { SearchResult } from "../api/types.ts";
import { docHref, DOC_TYPE_INFO } from "../lib/docs.ts";
import { parseSnippet } from "../lib/snippet.ts";
import { useDebouncedValue } from "../lib/useDebouncedValue.ts";
import { useListNav } from "../keys/useListNav.ts";
import { ErrorState, EmptyState } from "../components/EmptyState.tsx";
import { SkeletonRows } from "../components/Skeleton.tsx";

const URL_SYNC_DEBOUNCE_MS = 300;

export function SearchPage({ initialQuery }: { initialQuery: string }) {
  const navigate = useNavigate();
  const [query, setQuery] = useState(initialQuery);
  const debounced = useDebouncedValue(query, URL_SYNC_DEBOUNCE_MS);
  const results = useSearchQuery(debounced.trim());

  // Keep ?q= in sync so a search survives reload/share.
  useEffect(() => {
    void navigate({
      to: "/search",
      search: debounced ? { q: debounced } : {},
      replace: true,
    });
  }, [debounced, navigate]);

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-2 border-b border-border pb-2">
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
          className="h-9 w-full bg-transparent text-sm text-heading outline-none placeholder:text-muted"
        />
      </div>
      {results.isLoading ? (
        <SkeletonRows />
      ) : results.isError ? (
        <ErrorState error={results.error} />
      ) : !results.data ? (
        <p className="py-8 text-center text-xs text-muted">
          Search titles and full text. Filters: type: tag: is:task
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
        const Icon = DOC_TYPE_INFO[result.type].icon;
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
