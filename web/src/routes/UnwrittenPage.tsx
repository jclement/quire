// Names the vault keeps referring to that have no document yet — a to-do
// list for the graph that writes itself. Each row creates the missing
// document as whichever type it should have been, then opens it.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link as RouterLink, useNavigate } from "@tanstack/react-router";
import { Plus, SquareDashed } from "lucide-react";
import { useState } from "react";
import { api, errorMessage } from "../api/client.ts";
import { useDefaultArea } from "../api/queries.ts";
import type { DocType } from "../api/types.ts";
import { docHref, DOC_TYPE_INFO } from "../lib/docs.ts";
import { EmptyState } from "../components/EmptyState.tsx";
import { SkeletonRows } from "../components/Skeleton.tsx";
import { useUi } from "../keys/UiContext.tsx";

/** Offered in the order a dangling name usually turns out to be one. */
const TYPES: DocType[] = ["person", "company", "project", "note", "meeting"];

export function UnwrittenPage() {
  const unwritten = useQuery({
    queryKey: ["unwritten"],
    queryFn: () => api.unwritten(),
  });
  return (
    <div className="flex max-w-3xl flex-col gap-3">
      <header className="flex items-center gap-2 border-b border-border pb-2">
        <SquareDashed className="size-4 text-muted" aria-hidden="true" />
        <h1 className="text-lg font-semibold text-heading">Unwritten</h1>
        {unwritten.data ? (
          <span className="text-xs text-muted">{unwritten.data.length}</span>
        ) : null}
      </header>
      <p className="text-xs text-muted">
        Names your notes link to that have no document behind them yet.
      </p>
      {unwritten.isPending ? (
        <SkeletonRows count={4} />
      ) : unwritten.isError ? (
        <p className="text-xs text-danger">{errorMessage(unwritten.error)}</p>
      ) : unwritten.data.length === 0 ? (
        <EmptyState
          icon={SquareDashed}
          title="Nothing unwritten"
          hint="Every [[link]] in the vault resolves to a document."
        />
      ) : (
        <ul className="divide-y divide-border border-y border-border">
          {unwritten.data.map((entry) => (
            <UnwrittenRow key={entry.name} entry={entry} />
          ))}
        </ul>
      )}
    </div>
  );
}

function UnwrittenRow({
  entry,
}: {
  entry: {
    name: string;
    refs: number;
    sources: { path: string; title: string }[];
  };
}) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { toast } = useUi();
  const area = useDefaultArea();
  const [open, setOpen] = useState(false);

  const create = useMutation({
    mutationFn: (type: DocType) =>
      api.createDocument(type, entry.name, undefined, area),
    onSuccess: (doc) => {
      void queryClient.invalidateQueries({ queryKey: ["unwritten"] });
      void queryClient.invalidateQueries({ queryKey: ["documents"] });
      toast(`Created ${doc.title}`);
      void navigate({ to: docHref(doc.path), search: { edit: true } });
    },
    onError: (error) => toast(errorMessage(error)),
  });

  return (
    <li className="flex flex-wrap items-center gap-x-3 gap-y-1 px-2 py-2">
      <span className="font-medium text-heading">{entry.name}</span>
      <span className="font-mono text-[10px] text-muted">
        {entry.refs} {entry.refs === 1 ? "mention" : "mentions"}
      </span>
      <span className="flex min-w-0 flex-1 flex-wrap items-center gap-1 text-xs text-muted">
        {entry.sources.map((source) => (
          <RouterLink
            key={source.path}
            to={docHref(source.path)}
            className="truncate rounded bg-hover px-1.5 py-0.5 hover:text-heading"
          >
            {source.title}
          </RouterLink>
        ))}
      </span>
      <div className="relative">
        <button
          type="button"
          onClick={() => setOpen(!open)}
          disabled={create.isPending}
          aria-label={`Create ${entry.name}`}
          aria-expanded={open}
          aria-haspopup="menu"
          className="flex h-7 items-center gap-1 rounded border border-border px-2 text-xs text-body hover:bg-hover hover:text-heading disabled:opacity-50"
        >
          <Plus className="size-3.5" aria-hidden="true" />
          Create
        </button>
        {open ? (
          <>
            <div
              className="fixed inset-0 z-10"
              aria-hidden="true"
              onClick={() => setOpen(false)}
            />
            <div
              role="menu"
              aria-label={`Create ${entry.name} as`}
              className="absolute top-full right-0 z-20 mt-1 w-40 rounded border border-border bg-raised py-1 shadow-lg"
            >
              {TYPES.map((type) => {
                const info = DOC_TYPE_INFO[type];
                return (
                  <button
                    key={type}
                    type="button"
                    role="menuitem"
                    onClick={() => {
                      setOpen(false);
                      create.mutate(type);
                    }}
                    className="flex h-8 w-full items-center gap-2 px-3 text-left text-xs text-body hover:bg-hover hover:text-heading"
                  >
                    <info.icon
                      className="size-3.5 shrink-0 text-muted"
                      aria-hidden="true"
                    />
                    {info.label}
                  </button>
                );
              })}
            </div>
          </>
        ) : null}
      </div>
    </li>
  );
}
