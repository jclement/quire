// Dense task rows (~32px) shared by the task views and Today: checkbox,
// lightly-rendered text, due badge, project chip, source-doc link. Rendered as
// grouped sections (Upcoming's date buckets, Today's overdue/due/available)
// with ONE roving selection across all groups, so j/k, Enter, and x behave as
// a single list per page.
import { Link as RouterLink, useNavigate } from "@tanstack/react-router";
import { Fragment, useMemo } from "react";
import { useToggleTask } from "../api/queries.ts";
import type { Task } from "../api/types.ts";
import { dueInfo, formatShortDate, todayISO } from "../lib/dates.ts";
import { docHref } from "../lib/docs.ts";
import { splitWikilinks } from "../lib/wikilinks.ts";
import { useListNav } from "../keys/useListNav.ts";

export interface TaskGroup {
  key: string;
  /** Section heading; null renders the rows with no heading. */
  title: string | null;
  tasks: Task[];
  /** Optional heading accent (e.g. Overdue in red). */
  tone?: "danger" | "warn";
}

interface GroupedTaskListProps {
  groups: TaskGroup[];
  /** Only one list per page may own the keyboard. */
  navEnabled?: boolean;
  /** Logbook: show completion date instead of the due badge. */
  showCompletedOn?: boolean;
}

const HEADING_TONE = {
  danger: "text-danger",
  warn: "text-warn",
} as const;

export function GroupedTaskList({
  groups,
  navEnabled = true,
  showCompletedOn = false,
}: GroupedTaskListProps) {
  const navigate = useNavigate();
  const toggleTask = useToggleTask();
  const allTasks = useMemo(
    () => groups.flatMap((group) => group.tasks),
    [groups],
  );
  // Each group's offset into the flattened list, for global selection indices.
  const groupStarts = useMemo(() => {
    const starts: number[] = [];
    let total = 0;
    for (const group of groups) {
      starts.push(total);
      total += group.tasks.length;
    }
    return starts;
  }, [groups]);
  const nav = useListNav({
    items: allTasks,
    enabled: navEnabled,
    onOpen: (task) => void navigate({ to: docHref(task.doc_path) }),
    onToggle: (task) => toggleTask.mutate(task),
  });

  return (
    <div className="flex flex-col gap-4">
      {groups.map((group, groupAt) => {
        const start = groupStarts[groupAt] ?? 0;
        if (group.tasks.length === 0) return null;
        return (
          <section key={group.key}>
            {group.title !== null ? (
              <h2
                className={`mb-1 flex items-baseline gap-1.5 text-[10px] font-semibold uppercase tracking-wider ${
                  group.tone ? HEADING_TONE[group.tone] : "text-muted"
                }`}
              >
                {group.title}
                <span className="font-mono font-normal text-muted">
                  {group.tasks.length}
                </span>
              </h2>
            ) : null}
            <ul className="divide-y divide-border border-y border-border">
              {group.tasks.map((task, at) => (
                <TaskRow
                  key={task.id}
                  task={task}
                  selected={navEnabled && start + at === nav.index}
                  rowRef={nav.rowRef(start + at)}
                  onSelect={() => nav.setIndex(start + at)}
                  onToggle={() => toggleTask.mutate(task)}
                  showCompletedOn={showCompletedOn}
                />
              ))}
            </ul>
          </section>
        );
      })}
    </div>
  );
}

/** Convenience for the flat views (inbox, today, waiting, logbook). */
export function TaskListFlat(props: {
  tasks: Task[];
  navEnabled?: boolean;
  showCompletedOn?: boolean;
}) {
  return (
    <GroupedTaskList
      groups={[{ key: "all", title: null, tasks: props.tasks }]}
      navEnabled={props.navEnabled}
      showCompletedOn={props.showCompletedOn}
    />
  );
}

// ---- Rows ----

/** Priority dot color by the 1-high/2-med/3-low scale; none for 0. */
const PRIORITY_DOT: Record<number, string> = {
  1: "bg-danger",
  2: "bg-warn",
  3: "bg-muted",
};

const DUE_BADGE = {
  overdue: "text-danger border-danger/40",
  today: "text-warn border-warn/40",
  future: "text-muted border-border",
} as const;

interface TaskRowProps {
  task: Task;
  selected: boolean;
  rowRef: (el: HTMLElement | null) => void;
  onSelect: () => void;
  onToggle: () => void;
  showCompletedOn: boolean;
}

function TaskRow({
  task,
  selected,
  rowRef,
  onSelect,
  onToggle,
  showCompletedOn,
}: TaskRowProps) {
  const due = task.due ? dueInfo(task.due, todayISO()) : null;
  return (
    <li
      ref={rowRef}
      tabIndex={-1}
      onClick={onSelect}
      className={`flex min-h-8 items-center gap-2.5 px-2 py-1 outline-none ${
        selected ? "bg-selected" : "hover:bg-hover"
      }`}
    >
      <input
        type="checkbox"
        checked={task.done}
        onChange={onToggle}
        onClick={(event) => event.stopPropagation()}
        aria-label={`Toggle: ${task.text}`}
        className="size-3.5 shrink-0 cursor-pointer rounded-sm accent-(--accent)"
      />
      {PRIORITY_DOT[task.priority] ? (
        <span
          className={`size-1.5 shrink-0 rounded-full ${PRIORITY_DOT[task.priority]}`}
          title={
            ["", "high priority", "medium priority", "low priority"][
              task.priority
            ]
          }
        />
      ) : null}
      <span
        className={`min-w-0 flex-1 truncate text-sm ${
          task.done ? "text-muted line-through" : "text-body"
        }`}
      >
        <InlineTaskText text={task.text} />
      </span>
      {showCompletedOn && task.completed_on ? (
        <span className="shrink-0 font-mono text-[10px] text-muted">
          ✓ {formatShortDate(task.completed_on, todayISO())}
        </span>
      ) : due ? (
        <span
          className={`shrink-0 rounded border px-1.5 py-px font-mono text-[10px] ${DUE_BADGE[due.tone]}`}
        >
          {due.label}
        </span>
      ) : null}
      {task.project ? (
        <span className="hidden shrink-0 rounded-full border border-border px-1.5 py-px text-[10px] text-muted sm:inline">
          {task.project}
        </span>
      ) : null}
      <RouterLink
        to={docHref(task.doc_path)}
        onClick={(event) => event.stopPropagation()}
        className="hidden max-w-32 shrink-0 truncate text-xs text-muted hover:text-accent hover:underline md:inline"
      >
        {task.doc_title}
      </RouterLink>
    </li>
  );
}

/** Task text with wikilinks, `code`, and **bold** rendered lightly — no block
 * markdown, this is a one-line row. */
function InlineTaskText({ text }: { text: string }) {
  return (
    <>
      {splitWikilinks(text).map((segment, at) =>
        segment.kind === "link" ? (
          <span key={at} className="text-accent">
            {segment.display}
          </span>
        ) : (
          <Fragment key={at}>{renderEmphasis(segment.text)}</Fragment>
        ),
      )}
    </>
  );
}

function renderEmphasis(text: string) {
  return text.split(/(`[^`]+`|\*\*[^*]+\*\*)/g).map((part, at) => {
    if (part.startsWith("`") && part.endsWith("`")) {
      return (
        <code
          key={at}
          className="rounded bg-code-bg px-1 font-mono text-[0.85em]"
        >
          {part.slice(1, -1)}
        </code>
      );
    }
    if (part.startsWith("**") && part.endsWith("**")) {
      return (
        <strong key={at} className="font-semibold text-heading">
          {part.slice(2, -2)}
        </strong>
      );
    }
    return <Fragment key={at}>{part}</Fragment>;
  });
}
