// Every tag in the vault, sized by use. A tag is a search filter, so each
// one links straight to `tag:x` search — there is no separate tag page to
// keep in sync with search results.
import { useQuery } from "@tanstack/react-query";
import { Link as RouterLink } from "@tanstack/react-router";
import { Hash } from "lucide-react";
import { api, errorMessage } from "../api/client.ts";
import { EmptyState } from "../components/EmptyState.tsx";
import { SkeletonRows } from "../components/Skeleton.tsx";

export function TagsPage() {
  const tags = useQuery({ queryKey: ["tags"], queryFn: api.listTags });
  return (
    <div className="flex max-w-3xl flex-col gap-3">
      <header className="flex items-center gap-2 border-b border-border pb-2">
        <Hash className="size-4 text-muted" aria-hidden="true" />
        <h1 className="text-lg font-semibold text-heading">Tags</h1>
        {tags.data ? (
          <span className="text-xs text-muted">{tags.data.length}</span>
        ) : null}
      </header>
      {tags.isPending ? (
        <SkeletonRows count={4} />
      ) : tags.isError ? (
        <p className="text-xs text-danger">{errorMessage(tags.error)}</p>
      ) : tags.data.length === 0 ? (
        <EmptyState
          icon={Hash}
          title="No tags yet"
          hint="Write #a-tag in any note, or add tags: [x, y] to its frontmatter."
        />
      ) : (
        <ul className="flex flex-wrap gap-2">
          {tags.data.map((t) => (
            <li key={t.tag}>
              <RouterLink
                to="/search"
                search={{ q: `tag:${t.tag}` }}
                className="flex h-8 items-center gap-1.5 rounded border border-border px-2.5 text-sm text-body hover:bg-hover hover:text-heading"
              >
                <span className="font-mono text-accent">#{t.tag}</span>
                <span className="text-xs text-muted">{t.count}</span>
              </RouterLink>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
