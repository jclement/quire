// Dense task rows (~32px) shared by the task views and Today: checkbox,
// lightly-rendered text, due badge, recur badge, project chip, source-doc
// link, and a snooze popover (`s` key or the hover calendar button). Rendered
// as grouped sections (Upcoming's date buckets, Today's overdue/due/available)
// with ONE roving selection across all groups, so j/k, Enter, x, and s behave
// as a single list per page. A waiting task shows how long it has waited and
// on whom, flagged once it is stale. With `triage` (the inbox) every row also
// carries the triage actions — buttons on hover or selection, keys from
// lib/triage.ts — and `s` parks a task for someday instead of snoozing it.
import { Link as RouterLink, useNavigate } from "@tanstack/react-router";
import {
  Archive,
  CalendarClock,
  CalendarDays,
  CalendarRange,
  ChevronRight,
  FileText,
  Hourglass,
  Repeat,
  Sun,
  Sunrise,
  type LucideIcon,
} from "lucide-react";
import { Fragment, useState } from "react";
import { useEditTask, useToggleTask } from "../api/queries.ts";
import type { Task } from "../api/types.ts";
import { dueInfo, formatShortDate, todayISO } from "../lib/dates.ts";
import { docHref } from "../lib/docs.ts";
import { TRIAGE_ACTIONS, triageStep, type TriageKey } from "../lib/triage.ts";
import { splitWikilinks } from "../lib/wikilinks.ts";
import { useUi } from "../keys/UiContext.tsx";
import { useListNav } from "../keys/useListNav.ts";
import { SnoozePopover } from "./SnoozePopover.tsx";
import { TaskToNoteDialog } from "./TaskToNoteDialog.tsx";

export interface TaskGroup {
  key: string;
  /** Section heading; null renders the rows with no heading. */
  title: string | null;
  tasks: Task[];
  /** Optional heading accent (e.g. Overdue in red). */
  tone?: "danger" | "warn";
  /** Starts folded (Upcoming's Someday); folded rows are out of j/k. */
  collapsible?: boolean;
}

interface GroupedTaskListProps {
  groups: TaskGroup[];
  /** Only one list per page may own the keyboard. */
  navEnabled?: boolean;
  /** Logbook: show completion date instead of the due badge. */
  showCompletedOn?: boolean;
  /** Inbox: per-row triage buttons and keys. */
  triage?: boolean;
  /** Show who a waiting task is owed by (off where the group names them). */
  showWho?: boolean;
}

/** Which row's date popover is open, and which date it sets. */
interface OpenPicker {
  id: string;
  field: "due" | "defer";
}

const HEADING_TONE = {
  danger: "text-danger",
  warn: "text-warn",
} as const;

export function GroupedTaskList({
  groups,
  navEnabled = true,
  showCompletedOn = false,
  triage = false,
  showWho = true,
}: GroupedTaskListProps) {
  const navigate = useNavigate();
  const toggleTask = useToggleTask();
  const editTask = useEditTask();
  const { toast } = useUi();
  // Which task's date popover is open (by id; ids are unique per list).
  const [picker, setPicker] = useState<OpenPicker | null>(null);
  // The task being turned into a note, while its dialog is up.
  const [noteTask, setNoteTask] = useState<Task | null>(null);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const isFolded = (group: TaskGroup) =>
    group.collapsible === true && !expanded.has(group.key);
  const visibleGroups = groups.map((group) =>
    isFolded(group)
      ? { ...group, shown: [] }
      : { ...group, shown: group.tasks },
  );
  const allTasks = visibleGroups.flatMap((group) => group.shown);
  // Each group's offset into the flattened list, for global selection indices.
  const groupStarts: number[] = [];
  let total = 0;
  for (const group of visibleGroups) {
    groupStarts.push(total);
    total += group.shown.length;
  }

  const togglePicker = (task: Task, field: OpenPicker["field"]) =>
    setPicker((current) =>
      current?.id === task.id && current.field === field
        ? null
        : { id: task.id, field },
    );
  const runTriage = (key: TriageKey, task: Task) => {
    const step = triageStep(key, task, todayISO());
    if (step.kind === "pick") togglePicker(task, step.field);
    else if (step.kind === "note") setNoteTask(task);
    else
      editTask.mutate(
        { id: task.id, edit: step.edit },
        { onSuccess: () => toast(step.toast) },
      );
  };
  // A fresh object each render is fine: useListNav reads handlers through a
  // ref and only re-registers when the set of keys changes.
  const itemKeys = triage
    ? Object.fromEntries(
        TRIAGE_ACTIONS.map((action) => [
          action.key,
          (task: Task) => runTriage(action.key, task),
        ]),
      )
    : undefined;
  const nav = useListNav({
    items: allTasks,
    // The note dialog owns the keyboard while it is up.
    enabled: navEnabled && noteTask === null,
    onOpen: (task) => void navigate({ to: docHref(task.doc_path) }),
    onToggle: (task) => toggleTask.mutate(task),
    onSnooze: (task) => togglePicker(task, "due"),
    itemKeys,
  });

  return (
    <div className="flex flex-col gap-4">
      {visibleGroups.map((group, groupAt) => {
        const start = groupStarts[groupAt] ?? 0;
        if (group.tasks.length === 0) return null;
        const folded = isFolded(group);
        const heading = (
          <>
            {group.title}
            <span className="font-mono font-normal text-muted">
              {group.tasks.length}
            </span>
          </>
        );
        const headingClass = `mb-1 flex items-baseline gap-1.5 text-[10px] font-semibold uppercase tracking-wider ${
          group.tone ? HEADING_TONE[group.tone] : "text-muted"
        }`;
        return (
          <section key={group.key}>
            {group.collapsible ? (
              <h2 className={headingClass}>
                <button
                  type="button"
                  aria-expanded={!folded}
                  onClick={() =>
                    setExpanded((current) => {
                      const next = new Set(current);
                      if (folded) next.add(group.key);
                      else next.delete(group.key);
                      return next;
                    })
                  }
                  className="flex items-baseline gap-1.5 uppercase hover:text-heading"
                >
                  <ChevronRight
                    className={`size-3 self-center transition-transform ${folded ? "" : "rotate-90"}`}
                    aria-hidden="true"
                  />
                  {heading}
                </button>
              </h2>
            ) : group.title !== null ? (
              <h2 className={headingClass}>{heading}</h2>
            ) : null}
            {folded ? null : (
              <ul className="divide-y divide-border border-y border-border">
                {group.shown.map((task, at) => (
                  <TaskRow
                    key={task.id}
                    task={task}
                    selected={navEnabled && start + at === nav.index}
                    rowRef={nav.rowRef(start + at)}
                    onSelect={() => nav.setIndex(start + at)}
                    onToggle={() => toggleTask.mutate(task)}
                    showCompletedOn={showCompletedOn}
                    showWho={showWho}
                    picker={picker?.id === task.id ? picker.field : null}
                    onPickerOpen={(field) => {
                      nav.setIndex(start + at);
                      setPicker({ id: task.id, field });
                    }}
                    onPickerClose={() => setPicker(null)}
                    onTriage={
                      triage
                        ? (key) => {
                            nav.setIndex(start + at);
                            runTriage(key, task);
                          }
                        : undefined
                    }
                  />
                ))}
              </ul>
            )}
          </section>
        );
      })}
      {noteTask ? (
        <TaskToNoteDialog task={noteTask} onClose={() => setNoteTask(null)} />
      ) : null}
    </div>
  );
}

/** Convenience for the flat views (inbox, today, waiting, logbook). */
export function TaskListFlat(props: {
  tasks: Task[];
  navEnabled?: boolean;
  showCompletedOn?: boolean;
  triage?: boolean;
}) {
  return (
    <GroupedTaskList
      groups={[{ key: "all", title: null, tasks: props.tasks }]}
      navEnabled={props.navEnabled}
      showCompletedOn={props.showCompletedOn}
      triage={props.triage}
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

/** The icon for each triage key's row button. */
const TRIAGE_ICON: Record<TriageKey, LucideIcon> = {
  t: Sun,
  m: Sunrise,
  w: CalendarRange,
  d: CalendarDays,
  f: CalendarClock,
  p: Hourglass,
  s: Archive,
  n: FileText,
};

interface TaskRowProps {
  task: Task;
  selected: boolean;
  rowRef: (el: HTMLElement | null) => void;
  onSelect: () => void;
  onToggle: () => void;
  showCompletedOn: boolean;
  showWho: boolean;
  /** Which date popover is open on this row, if any. */
  picker: "due" | "defer" | null;
  onPickerOpen: (field: "due" | "defer") => void;
  onPickerClose: () => void;
  /** Set on the inbox: the row shows the triage buttons. */
  onTriage?: (key: TriageKey) => void;
}

function TaskRow({
  task,
  selected,
  rowRef,
  onSelect,
  onToggle,
  showCompletedOn,
  showWho,
  picker,
  onPickerOpen,
  onPickerClose,
  onTriage,
}: TaskRowProps) {
  const due = task.due ? dueInfo(task.due, todayISO()) : null;
  // Hidden until the row is hovered or selected; always there while a
  // popover hangs off it. Selection is what a tap does, so on a phone the
  // actions appear on the tapped row.
  const reveal =
    picker !== null || selected
      ? ""
      : "opacity-0 group-hover:opacity-100 focus-visible:opacity-100";
  return (
    <li
      ref={rowRef}
      tabIndex={-1}
      onClick={onSelect}
      className={`group relative flex min-h-8 items-center gap-2.5 px-2 py-1 outline-none ${
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
      {task.recur ? (
        <Repeat
          className="size-3 shrink-0 text-muted"
          aria-label={`Repeats: ${task.recur}`}
        >
          <title>{task.recur}</title>
        </Repeat>
      ) : null}
      {onTriage ? (
        <span
          className={`flex shrink-0 items-center ${selected ? "" : "max-md:hidden"} ${reveal}`}
        >
          {TRIAGE_ACTIONS.map((action) => {
            const Icon = TRIAGE_ICON[action.key];
            return (
              <button
                key={action.key}
                type="button"
                onClick={(event) => {
                  event.stopPropagation();
                  onTriage(action.key);
                }}
                aria-label={`${action.label}: ${task.text}`}
                title={`${action.label} (${action.key})`}
                className="flex size-6 items-center justify-center rounded text-muted hover:bg-border hover:text-heading max-md:size-8"
              >
                <Icon className="size-3.5" aria-hidden="true" />
              </button>
            );
          })}
        </span>
      ) : (
        <button
          type="button"
          onClick={(event) => {
            event.stopPropagation();
            if (picker) onPickerClose();
            else onPickerOpen("due");
          }}
          aria-label={`Snooze: ${task.text}`}
          className={`hidden size-6 shrink-0 items-center justify-center rounded text-muted hover:bg-border hover:text-heading md:flex ${reveal}`}
        >
          <CalendarClock className="size-3.5" aria-hidden="true" />
        </button>
      )}
      {picker ? (
        <SnoozePopover task={task} field={picker} onClose={onPickerClose} />
      ) : null}
      {task.waiting_for ? (
        <WaitingBadge waiting={task.waiting_for} showWho={showWho} />
      ) : null}
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

/** How long a wait has run ("⏳ 12d"), red once stale, and who owes it. */
function WaitingBadge({
  waiting,
  showWho,
}: {
  waiting: NonNullable<Task["waiting_for"]>;
  showWho: boolean;
}) {
  const days = waiting.days;
  const who = showWho ? waiting.on : null;
  if (days === null && !who) return null;
  return (
    <span
      className={`flex shrink-0 items-center gap-1 rounded border px-1.5 py-px font-mono text-[10px] ${
        waiting.stale
          ? "border-danger/40 text-danger"
          : "border-border text-muted"
      }`}
      title={
        waiting.since
          ? `Waiting since ${waiting.since}${waiting.stale ? " — time to chase" : ""}`
          : "Waiting"
      }
    >
      {days !== null ? <span>⏳ {days}d</span> : null}
      {who ? <span className="max-w-28 truncate font-sans">{who}</span> : null}
    </span>
  );
}

/** Task text with wikilinks, `code`, and **bold** rendered lightly — no block
 * markdown, this is a one-line row. */
export function InlineTaskText({ text }: { text: string }) {
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
