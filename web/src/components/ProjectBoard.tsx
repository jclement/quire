// The Projects board: one column per status, cards dragged within a column
// to order them and across columns to change their status. Loaded only when
// someone opens the board (BrowsePage), so the drag-and-drop library stays
// off every other page.
//
// A drop is written straight to the files it changes — the moved project's
// status: (where the server stamps completed:/cancelled:) and rank:, and
// the column's other ranks only when there was no room between neighbours.
// The list cache is updated first, so the card stays where it was dropped
// instead of jumping back while the writes land; the event stream then
// brings the server's version.
//
// Closed columns would otherwise grow forever, so Complete and Cancelled
// show what closed in the last RECENT_CLOSED_DAYS and fold the rest behind
// a "N older" row. Any column can be collapsed to a thin strip, which is
// remembered per device and still takes drops.
import {
  closestCorners,
  DndContext,
  DragOverlay,
  KeyboardSensor,
  PointerSensor,
  TouchSensor,
  useDroppable,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragOverEvent,
  type DragStartEvent,
} from "@dnd-kit/core";
import {
  arrayMove,
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { useQueryClient } from "@tanstack/react-query";
import { Link as RouterLink } from "@tanstack/react-router";
import { ChevronsLeftRight, ChevronsRightLeft } from "lucide-react";
import { useMemo, useRef, useState } from "react";
import { api, errorMessage } from "../api/client.ts";
import { useAreas, useAreasEnabled } from "../api/queries.ts";
import type { DocMeta, ProjectStatus } from "../api/types.ts";
import { useUi } from "../keys/UiContext.tsx";
import { areaColorVar } from "../lib/area.ts";
import { formatShortDate, todayISO } from "../lib/dates.ts";
import { docHref } from "../lib/docs.ts";
import {
  closedRecently,
  compareForBoard,
  PROJECT_STATUSES,
  rankWrites,
  RECENT_CLOSED_DAYS,
  STATUS_INFO,
  statusOf,
  type StatusInfo,
} from "../lib/projects.ts";
import { AreaDot } from "./AreaDot.tsx";

const COLLAPSED_KEY = "quire-board-collapsed";
const DEFAULT_COLLAPSED: ProjectStatus[] = ["cancelled"];

function loadCollapsed(): Set<ProjectStatus> {
  try {
    const stored = localStorage.getItem(COLLAPSED_KEY);
    if (stored) return new Set(JSON.parse(stored) as ProjectStatus[]);
  } catch {
    // Unreadable or unavailable storage: the default it is.
  }
  return new Set(DEFAULT_COLLAPSED);
}

function storeCollapsed(collapsed: Set<ProjectStatus>): void {
  try {
    localStorage.setItem(COLLAPSED_KEY, JSON.stringify([...collapsed]));
  } catch {
    // Storage unavailable: the choice just does not persist.
  }
}

type Columns = Record<ProjectStatus, string[]>;

const today = () => todayISO();
const shortDate = (day: string) => formatShortDate(day, todayISO());

export function ProjectBoard({ docs }: { docs: DocMeta[] }) {
  const queryClient = useQueryClient();
  const { toast } = useUi();
  const areas = useAreas();
  const areasEnabled = useAreasEnabled();
  const areaColors = useMemo(
    () => Object.fromEntries((areas.data ?? []).map((a) => [a.area, a.color])),
    [areas.data],
  );
  const [collapsed, setCollapsed] = useState(loadCollapsed);
  const [showOlder, setShowOlder] = useState<Set<ProjectStatus>>(new Set());
  // While a card is being dragged the board works from its own copy of the
  // columns; the rest of the time it is derived from the list.
  const [dragColumns, setDragColumns] = useState<Columns | null>(null);
  const [activePath, setActivePath] = useState<string | null>(null);
  // A drag ends in a click on whatever is under the pointer; this swallows
  // that one so dropping a card does not also open it.
  const justDragged = useRef(false);

  const byPath = useMemo(
    () => new Map(docs.map((doc) => [doc.path, doc])),
    [docs],
  );

  // Every column in board order, and how many closed ones are folded away.
  const { columns, older } = useMemo(() => {
    const all = Object.fromEntries(
      PROJECT_STATUSES.map((info) => [info.status, [] as DocMeta[]]),
    ) as Record<ProjectStatus, DocMeta[]>;
    for (const doc of docs) all[statusOf(doc)].push(doc);
    const shown = {} as Columns;
    const hidden = {} as Record<ProjectStatus, number>;
    const now = new Date();
    for (const info of PROJECT_STATUSES) {
      const sorted = all[info.status].sort(compareForBoard);
      const visible =
        info.closed && !showOlder.has(info.status)
          ? sorted.filter((doc) => closedRecently(doc, now))
          : sorted;
      shown[info.status] = visible.map((doc) => doc.path);
      hidden[info.status] = sorted.length - visible.length;
    }
    return { columns: shown, older: hidden };
  }, [docs, showOlder]);

  const live = dragColumns ?? columns;

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
    // A finger has to rest on a card before it lifts, or the board could
    // never be scrolled by touch.
    useSensor(TouchSensor, {
      activationConstraint: { delay: 200, tolerance: 6 },
    }),
    // Space picks a card up, arrows move it, Space drops, Escape cancels.
    // Enter is left to the link, so it still opens the project.
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
      keyboardCodes: { start: ["Space"], cancel: ["Escape"], end: ["Space"] },
    }),
  );

  const columnOf = (cols: Columns, id: string): ProjectStatus | null => {
    if (id in cols) return id as ProjectStatus;
    for (const info of PROJECT_STATUSES) {
      if (cols[info.status].includes(id)) return info.status;
    }
    return null;
  };

  const onDragStart = (event: DragStartEvent) => {
    setActivePath(String(event.active.id));
    setDragColumns(columns);
  };

  // Crossing into another column moves the card there at once, so the
  // column it is over opens a gap for it.
  const onDragOver = ({ active, over }: DragOverEvent) => {
    if (!over || !dragColumns) return;
    const id = String(active.id);
    const from = columnOf(dragColumns, id);
    const to = columnOf(dragColumns, String(over.id));
    if (!from || !to || from === to) return;
    const target = dragColumns[to];
    const overIndex = target.indexOf(String(over.id));
    const at = overIndex < 0 ? target.length : overIndex;
    setDragColumns({
      ...dragColumns,
      [from]: dragColumns[from].filter((path) => path !== id),
      [to]: [...target.slice(0, at), id, ...target.slice(at)],
    });
  };

  const finish = () => {
    setActivePath(null);
    setDragColumns(null);
    justDragged.current = true;
    setTimeout(() => (justDragged.current = false), 0);
  };

  const onDragEnd = ({ active, over }: DragEndEvent) => {
    const id = String(active.id);
    const cols = dragColumns;
    const moved = byPath.get(id);
    finish();
    if (!cols || !over || !moved) return;
    const status = columnOf(cols, id);
    if (!status) return;
    let order = cols[status];
    const overIndex = order.indexOf(String(over.id));
    const fromIndex = order.indexOf(id);
    if (overIndex >= 0 && overIndex !== fromIndex) {
      order = arrayMove(order, fromIndex, overIndex);
    }
    const statusChanged = statusOf(moved) !== status;
    const unmoved =
      !statusChanged && order.join("\n") === columns[status].join("\n");
    if (unmoved) return;

    // The column as it will be, with the moved project in its new status.
    const column = order
      .map((path) => byPath.get(path))
      .filter((doc): doc is DocMeta => doc !== undefined);
    const ranks = rankWrites(column, id);
    persist(moved, status, statusChanged, ranks);
  };

  const persist = (
    moved: DocMeta,
    status: ProjectStatus,
    statusChanged: boolean,
    ranks: Map<string, number>,
  ) => {
    const closing = STATUS_INFO[status].closed;
    // Optimistic: every cached list shows the result before the files do.
    queryClient.setQueriesData<DocMeta[]>({ queryKey: ["documents"] }, (list) =>
      list?.map((doc) => {
        if (!doc.project) return doc;
        const rank = ranks.get(doc.path);
        if (doc.path === moved.path && statusChanged) {
          return {
            ...doc,
            project: {
              ...doc.project,
              status,
              rank: rank ?? doc.project.rank,
              closed: closing ? today() : "",
            },
          };
        }
        return rank === undefined
          ? doc
          : { ...doc, project: { ...doc.project, rank } };
      }),
    );
    const writes = [...ranks].map(([path, rank]) =>
      api.setFrontmatter(path, {
        rank,
        ...(path === moved.path && statusChanged ? { status } : {}),
      }),
    );
    if (statusChanged && !ranks.has(moved.path)) {
      writes.push(api.setFrontmatter(moved.path, { status }));
    }
    Promise.all(writes)
      .then(() => {
        if (statusChanged) {
          toast(`${moved.title} → ${STATUS_INFO[status].label}`);
        }
      })
      .catch((error: unknown) => toast(errorMessage(error)))
      .finally(
        () => void queryClient.invalidateQueries({ queryKey: ["documents"] }),
      );
  };

  const toggleCollapsed = (status: ProjectStatus) => {
    const next = new Set(collapsed);
    if (next.has(status)) next.delete(status);
    else next.add(status);
    setCollapsed(next);
    storeCollapsed(next);
  };

  const toggleOlder = (status: ProjectStatus) => {
    const next = new Set(showOlder);
    if (next.has(status)) next.delete(status);
    else next.add(status);
    setShowOlder(next);
  };

  const active = activePath ? byPath.get(activePath) : undefined;
  return (
    <DndContext
      sensors={sensors}
      collisionDetection={closestCorners}
      onDragStart={onDragStart}
      onDragOver={onDragOver}
      onDragEnd={onDragEnd}
      onDragCancel={finish}
    >
      <div
        data-wide
        role="region"
        aria-label="Projects board"
        className="-mx-4 flex min-h-[calc(100dvh-13rem)] snap-x scroll-px-4 gap-2 overflow-x-auto px-4 pb-2 md:mx-0 md:min-h-[calc(100dvh-8rem)] md:px-0"
        onClickCapture={(event) => {
          if (justDragged.current) {
            event.preventDefault();
            event.stopPropagation();
          }
        }}
      >
        {PROJECT_STATUSES.map((info) =>
          collapsed.has(info.status) ? (
            <CollapsedColumn
              key={info.status}
              info={info}
              count={live[info.status].length + older[info.status]}
              onExpand={() => toggleCollapsed(info.status)}
            />
          ) : (
            <Column
              key={info.status}
              info={info}
              paths={live[info.status]}
              byPath={byPath}
              older={older[info.status]}
              showingOlder={showOlder.has(info.status)}
              onToggleOlder={() => toggleOlder(info.status)}
              onCollapse={() => toggleCollapsed(info.status)}
              areaColors={areasEnabled ? areaColors : null}
              activePath={activePath}
            />
          ),
        )}
      </div>
      <DragOverlay dropAnimation={{ duration: 150, easing: "ease-out" }}>
        {active ? (
          <Card
            doc={active}
            areaColors={areasEnabled ? areaColors : null}
            overlay
          />
        ) : null}
      </DragOverlay>
    </DndContext>
  );
}

function ColumnHeader({
  info,
  count,
  onCollapse,
}: {
  info: StatusInfo;
  count: number;
  onCollapse: () => void;
}) {
  return (
    <header className="flex h-8 items-center gap-1.5 px-2">
      <span
        className="size-2 shrink-0 rounded-full"
        style={{ background: areaColorVar(info.color) }}
        aria-hidden="true"
      />
      <h2 className="text-[11px] font-semibold tracking-wider text-heading uppercase">
        {info.label}
      </h2>
      <span className="font-mono text-[10px] text-muted">{count}</span>
      <button
        type="button"
        onClick={onCollapse}
        aria-label={`Collapse ${info.label}`}
        title={`Collapse ${info.label}`}
        className="ml-auto flex size-6 items-center justify-center rounded text-muted hover:bg-hover hover:text-heading"
      >
        <ChevronsRightLeft className="size-3.5" aria-hidden="true" />
      </button>
    </header>
  );
}

function Column({
  info,
  paths,
  byPath,
  older,
  showingOlder,
  onToggleOlder,
  onCollapse,
  areaColors,
  activePath,
}: {
  info: StatusInfo;
  paths: string[];
  byPath: Map<string, DocMeta>;
  older: number;
  showingOlder: boolean;
  onToggleOlder: () => void;
  onCollapse: () => void;
  areaColors: Record<string, string> | null;
  activePath: string | null;
}) {
  const { setNodeRef, isOver } = useDroppable({ id: info.status });
  return (
    <section
      aria-label={info.label}
      className="flex w-64 shrink-0 snap-start flex-col rounded-md border border-border bg-raised/60 md:w-auto md:min-w-44 md:flex-1"
      style={{ borderTopColor: areaColorVar(info.color), borderTopWidth: 2 }}
    >
      <ColumnHeader
        info={info}
        count={paths.length + (showingOlder ? 0 : older)}
        onCollapse={onCollapse}
      />
      <SortableContext items={paths} strategy={verticalListSortingStrategy}>
        <ul
          ref={setNodeRef}
          className={`flex min-h-16 flex-1 flex-col gap-1.5 px-1.5 pb-1.5 transition-colors ${
            isOver && paths.length === 0 ? "bg-hover/60" : ""
          }`}
        >
          {paths.map((path) => {
            const doc = byPath.get(path);
            return doc ? (
              <SortableCard
                key={path}
                doc={doc}
                areaColors={areaColors}
                placeholder={path === activePath}
              />
            ) : null;
          })}
          {paths.length === 0 ? (
            <li className="flex h-12 items-center justify-center rounded border border-dashed border-border text-[11px] text-muted">
              {info.closed && older > 0 ? "Nothing recent" : "Drop here"}
            </li>
          ) : null}
        </ul>
      </SortableContext>
      {info.closed && (older > 0 || showingOlder) ? (
        <button
          type="button"
          onClick={onToggleOlder}
          className="mx-1.5 mb-1.5 h-7 rounded text-[11px] text-muted hover:bg-hover hover:text-heading"
        >
          {showingOlder
            ? `Only the last ${RECENT_CLOSED_DAYS} days`
            : `${older} older`}
        </button>
      ) : null}
    </section>
  );
}

/** A folded column: a thin strip with its name and count that still takes
 * a dropped card. */
function CollapsedColumn({
  info,
  count,
  onExpand,
}: {
  info: StatusInfo;
  count: number;
  onExpand: () => void;
}) {
  const { setNodeRef, isOver } = useDroppable({ id: info.status });
  return (
    <section
      ref={setNodeRef}
      aria-label={`${info.label} (collapsed)`}
      className={`flex w-9 shrink-0 flex-col items-center rounded-md border border-border py-1 transition-colors ${
        isOver ? "bg-hover" : "bg-raised/60"
      }`}
      style={{ borderTopColor: areaColorVar(info.color), borderTopWidth: 2 }}
    >
      <button
        type="button"
        onClick={onExpand}
        aria-label={`Expand ${info.label}`}
        title={`Expand ${info.label}`}
        className="flex w-full flex-1 flex-col items-center gap-2 rounded py-1 text-muted hover:bg-hover hover:text-heading"
      >
        <ChevronsLeftRight className="size-3.5" aria-hidden="true" />
        <span className="font-mono text-[10px]">{count}</span>
        <span className="text-[11px] font-semibold tracking-wider uppercase [writing-mode:vertical-rl]">
          {info.label}
        </span>
      </button>
    </section>
  );
}

function SortableCard({
  doc,
  areaColors,
  placeholder,
}: {
  doc: DocMeta;
  areaColors: Record<string, string> | null;
  placeholder: boolean;
}) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: doc.path });
  return (
    <li
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={isDragging || placeholder ? "opacity-30" : ""}
      {...attributes}
      {...listeners}
    >
      <Card doc={doc} areaColors={areaColors} />
    </li>
  );
}

function Card({
  doc,
  areaColors,
  overlay = false,
}: {
  doc: DocMeta;
  areaColors: Record<string, string> | null;
  overlay?: boolean;
}) {
  const project = doc.project;
  const closed = STATUS_INFO[statusOf(doc)].closed;
  const due = project?.due ?? "";
  const overdue = !closed && due !== "" && due < today();
  const meta: { key: string; text: string; className?: string }[] = [];
  if (project?.company) meta.push({ key: "company", text: project.company });
  if (due && !closed) {
    meta.push({
      key: "due",
      text: `due ${shortDate(due)}`,
      className: overdue ? "text-danger" : undefined,
    });
  }
  if (closed && project?.closed) {
    meta.push({ key: "closed", text: shortDate(project.closed) });
  }
  return (
    <RouterLink
      to={docHref(doc.path)}
      draggable={false}
      className={`block cursor-grab touch-manipulation rounded border border-border bg-surface px-2 py-1.5 text-left outline-none select-none hover:border-muted focus-visible:ring-2 focus-visible:ring-accent active:cursor-grabbing ${
        overlay
          ? "rotate-1 cursor-grabbing shadow-lg ring-1 ring-accent/40"
          : ""
      } ${closed ? "opacity-75" : ""}`}
    >
      <span className="flex items-start gap-1.5">
        {areaColors && doc.area ? (
          <AreaDot
            color={areaColors[doc.area]}
            title={`Area: ${doc.area}`}
            className="mt-1.5"
          />
        ) : null}
        <span
          className={`line-clamp-2 text-[13px] leading-snug text-heading ${
            statusOf(doc) === "cancelled" ? "line-through decoration-muted" : ""
          }`}
        >
          {doc.title}
        </span>
      </span>
      {meta.length > 0 || doc.tags.length > 0 ? (
        <span className="mt-0.5 flex flex-wrap items-center gap-x-1.5 font-mono text-[10px] text-muted">
          {meta.map((item, i) => (
            <span key={item.key} className={item.className}>
              {i > 0 ? "· " : ""}
              {item.text}
            </span>
          ))}
          {doc.tags.slice(0, 3).map((tag) => (
            <span key={tag} className="text-accent">
              #{tag}
            </span>
          ))}
        </span>
      ) : null}
    </RouterLink>
  );
}
