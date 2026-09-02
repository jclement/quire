// Per-type document lists (/browse/<type>): a sortable table of title, tags,
// and modified time, with j/k navigation and a New button opening the shared
// create dialog.
import { Link as RouterLink, useNavigate } from "@tanstack/react-router";
import { ArrowDown, ArrowUp, Plus } from "lucide-react";
import { useMemo, useState } from "react";
import { useAreas, useAreasEnabled, useDocumentList } from "../api/queries.ts";
import { AreaDot } from "../components/AreaDot.tsx";
import type { DocMeta, DocType } from "../api/types.ts";
import { formatRelativeTime } from "../lib/dates.ts";
import { docHref, DOC_TYPE_INFO } from "../lib/docs.ts";
import { useUi } from "../keys/UiContext.tsx";
import { useListNav } from "../keys/useListNav.ts";
import { ErrorState, EmptyState } from "../components/EmptyState.tsx";
import { SkeletonRows } from "../components/Skeleton.tsx";

type SortKey = "title" | "mtime";

export function BrowsePage({ type }: { type: DocType }) {
  const info = DOC_TYPE_INFO[type];
  const { setNewDocType } = useUi();
  const docs = useDocumentList({ type });

  return (
    <div className="flex flex-col gap-3">
      <header className="flex items-center gap-2 border-b border-border pb-2">
        <info.icon className="size-4 text-muted" aria-hidden="true" />
        <h1 className="text-lg font-semibold text-heading">{info.plural}</h1>
        {docs.data ? (
          <span className="font-mono text-xs text-muted">
            {docs.data.length}
          </span>
        ) : null}
        <button
          type="button"
          onClick={() => setNewDocType(type)}
          className="ml-auto flex h-7 items-center gap-1 rounded border border-border px-2 text-xs text-body hover:bg-hover hover:text-heading"
        >
          <Plus className="size-3.5" aria-hidden="true" />
          New {info.label}
        </button>
      </header>
      {docs.isPending ? (
        <SkeletonRows />
      ) : docs.isError ? (
        <ErrorState error={docs.error} />
      ) : docs.data.length === 0 ? (
        <EmptyState
          icon={info.icon}
          title={`No ${info.plural.toLowerCase()} yet`}
          hint={`Create one with the New ${info.label} button or the palette.`}
        />
      ) : (
        <DocTable docs={docs.data} />
      )}
    </div>
  );
}

function DocTable({ docs }: { docs: DocMeta[] }) {
  const navigate = useNavigate();
  const areas = useAreas();
  const areasEnabled = useAreasEnabled();
  const areaColors = Object.fromEntries(
    (areas.data ?? []).map((a) => [a.area, a.color]),
  );
  const [sortKey, setSortKey] = useState<SortKey>("mtime");
  const [ascending, setAscending] = useState(false);

  const sorted = useMemo(() => {
    const copy = [...docs];
    copy.sort((a, b) => {
      const compared =
        sortKey === "title"
          ? a.title.localeCompare(b.title)
          : a.mtime.localeCompare(b.mtime);
      return ascending ? compared : -compared;
    });
    return copy;
  }, [docs, sortKey, ascending]);

  const nav = useListNav({
    items: sorted,
    onOpen: (doc) => void navigate({ to: docHref(doc.path) }),
  });

  const toggleSort = (key: SortKey) => {
    if (sortKey === key) setAscending(!ascending);
    else {
      setSortKey(key);
      // New sort column starts in its natural direction.
      setAscending(key === "title");
    }
  };

  return (
    <table className="w-full border-y border-border text-sm">
      <thead>
        <tr className="border-b border-border text-left">
          <SortHeader
            label="Title"
            active={sortKey === "title"}
            ascending={ascending}
            onClick={() => toggleSort("title")}
          />
          <th className="hidden px-2 py-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted sm:table-cell">
            Tags
          </th>
          <SortHeader
            label="Modified"
            active={sortKey === "mtime"}
            ascending={ascending}
            onClick={() => toggleSort("mtime")}
            alignRight
          />
        </tr>
      </thead>
      <tbody className="divide-y divide-border">
        {sorted.map((doc, at) => (
          <tr
            key={doc.path}
            ref={nav.rowRef(at)}
            tabIndex={-1}
            onClick={() => {
              nav.setIndex(at);
              void navigate({ to: docHref(doc.path) });
            }}
            className={`h-8 cursor-pointer outline-none ${
              at === nav.index ? "bg-selected" : "hover:bg-hover"
            }`}
          >
            <td className="max-w-0 truncate px-2 text-body">
              {/* A real anchor, not just a row click handler: this is how a
                  row is opened in a new tab, middle-clicked, copied as a
                  link, or reached by a screen reader. The row's onClick
                  stays for click-anywhere convenience. */}
              <RouterLink
                to={docHref(doc.path)}
                onClick={(event) => event.stopPropagation()}
                className="flex items-center gap-1.5 truncate outline-none hover:underline"
              >
                {areasEnabled && doc.area ? (
                  <AreaDot
                    color={areaColors[doc.area]}
                    title={`Area: ${doc.area}`}
                  />
                ) : null}
                <span className="truncate">{doc.title}</span>
              </RouterLink>
            </td>
            <td className="hidden truncate px-2 text-xs text-muted sm:table-cell">
              {doc.tags.map((tag) => (
                <RouterLink
                  key={tag}
                  to="/search"
                  search={{ q: `tag:${tag}` }}
                  onClick={(event) => event.stopPropagation()}
                  className="mr-1.5 font-mono text-accent hover:underline"
                >
                  #{tag}
                </RouterLink>
              ))}
            </td>
            <td className="whitespace-nowrap px-2 text-right text-xs text-muted">
              {formatRelativeTime(doc.mtime)}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function SortHeader({
  label,
  active,
  ascending,
  onClick,
  alignRight = false,
}: {
  label: string;
  active: boolean;
  ascending: boolean;
  onClick: () => void;
  alignRight?: boolean;
}) {
  const Arrow = ascending ? ArrowUp : ArrowDown;
  return (
    <th className={`px-0 py-0 ${alignRight ? "text-right" : ""}`}>
      <button
        type="button"
        onClick={onClick}
        className={`flex h-7 items-center gap-1 px-2 text-[10px] font-semibold uppercase tracking-wider hover:text-heading ${
          active ? "text-heading" : "text-muted"
        } ${alignRight ? "ml-auto" : ""}`}
      >
        {label}
        {active ? <Arrow className="size-3" aria-hidden="true" /> : null}
      </button>
    </th>
  );
}
