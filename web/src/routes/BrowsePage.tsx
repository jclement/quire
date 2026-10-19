// Per-type document lists (/browse/<type>): a sortable table of title, tags,
// and modified time, with j/k navigation and a New button opening the shared
// create dialog. Projects add a status column, and can be shown as a board
// instead (ProjectBoard, loaded on first use); which one is remembered per
// device.
import { Link as RouterLink, useNavigate } from "@tanstack/react-router";
import { ArrowDown, ArrowUp, Columns3, List, Plus } from "lucide-react";
import { lazy, Suspense, useMemo, useState } from "react";
import { useAreas, useAreasEnabled, useDocumentList } from "../api/queries.ts";
import { AreaDot } from "../components/AreaDot.tsx";
import type { DocMeta, DocType } from "../api/types.ts";
import { formatRelativeTime } from "../lib/dates.ts";
import { docHref, DOC_TYPE_INFO } from "../lib/docs.ts";
import { useUi } from "../keys/UiContext.tsx";
import { useListNav } from "../keys/useListNav.ts";
import { ErrorState, EmptyState } from "../components/EmptyState.tsx";
import { SkeletonRows } from "../components/Skeleton.tsx";
import { STATUS_INFO, statusOf } from "../lib/projects.ts";

const ProjectBoard = lazy(() =>
  import("../components/ProjectBoard.tsx").then((module) => ({
    default: module.ProjectBoard,
  })),
);

type ProjectsView = "list" | "board";
const VIEW_KEY = "quire-projects-view";

function loadView(): ProjectsView {
  try {
    return localStorage.getItem(VIEW_KEY) === "board" ? "board" : "list";
  } catch {
    return "list";
  }
}

function storeView(view: ProjectsView): void {
  try {
    localStorage.setItem(VIEW_KEY, view);
  } catch {
    // Storage unavailable: the choice just does not persist.
  }
}

type SortKey = "title" | "mtime";

export function BrowsePage({ type }: { type: DocType }) {
  const info = DOC_TYPE_INFO[type];
  const { setNewDocType } = useUi();
  const docs = useDocumentList({ type });
  const [view, setView] = useState<ProjectsView>(loadView);
  const board = type === "project" && view === "board";
  const chooseView = (next: ProjectsView) => {
    setView(next);
    storeView(next);
  };

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
        {type === "project" ? (
          <ViewSwitch view={view} onChange={chooseView} />
        ) : null}
        <button
          type="button"
          onClick={() => setNewDocType(type)}
          className={`${type === "project" ? "" : "ml-auto "}flex h-7 items-center gap-1 rounded border border-border px-2 text-xs text-body hover:bg-hover hover:text-heading`}
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
      ) : board ? (
        <Suspense fallback={<SkeletonRows />}>
          <ProjectBoard docs={docs.data} />
        </Suspense>
      ) : (
        <DocTable docs={docs.data} showStatus={type === "project"} />
      )}
    </div>
  );
}

/** List | Board, for projects. */
function ViewSwitch({
  view,
  onChange,
}: {
  view: ProjectsView;
  onChange: (view: ProjectsView) => void;
}) {
  const options = [
    { value: "list" as const, label: "List", icon: List },
    { value: "board" as const, label: "Board", icon: Columns3 },
  ];
  return (
    <div
      role="group"
      aria-label="Projects view"
      className="ml-auto flex h-7 items-center rounded border border-border"
    >
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          aria-pressed={view === option.value}
          onClick={() => onChange(option.value)}
          className={`flex h-full items-center gap-1 px-2 text-xs first:rounded-l last:rounded-r ${
            view === option.value
              ? "bg-selected text-heading"
              : "text-muted hover:bg-hover hover:text-body"
          }`}
        >
          <option.icon className="size-3.5" aria-hidden="true" />
          {option.label}
        </button>
      ))}
    </div>
  );
}

function DocTable({
  docs,
  showStatus = false,
}: {
  docs: DocMeta[];
  showStatus?: boolean;
}) {
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
          {showStatus ? (
            <th className="px-2 py-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted">
              Status
            </th>
          ) : null}
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
            {showStatus ? (
              <td className="whitespace-nowrap px-2">
                <StatusPill doc={doc} />
              </td>
            ) : null}
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

function StatusPill({ doc }: { doc: DocMeta }) {
  const info = STATUS_INFO[statusOf(doc)];
  return (
    <span className="inline-flex h-[22px] items-center gap-1 rounded border border-border bg-raised px-1.5 font-mono text-[10px] text-body">
      <AreaDot color={info.color} />
      {info.label.toLowerCase()}
    </span>
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
