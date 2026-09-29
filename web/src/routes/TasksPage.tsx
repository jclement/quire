// Task views: /inbox and /tasks/<today|upcoming|waiting|logbook>. All render
// the same dense rows; the Inbox carries the triage keys, Upcoming groups by
// due date with a folded Someday section at the end, Waiting groups by who
// the wait is owed by, Logbook shows completion dates. j/k + x + Enter come
// from GroupedTaskList.
import { Link as RouterLink } from "@tanstack/react-router";
import { Inbox as InboxIcon, PartyPopper } from "lucide-react";
import { useTasks, useWaitingGroups } from "../api/queries.ts";
import type { Task, TaskView, WaitingGroup } from "../api/types.ts";
import { formatDayHeading, groupByDate } from "../lib/dates.ts";
import { ErrorState, EmptyState } from "../components/EmptyState.tsx";
import { SkeletonRows } from "../components/Skeleton.tsx";
import {
  GroupedTaskList,
  TaskListFlat,
  type TaskGroup,
} from "../components/TaskList.tsx";

const VIEW_META: Record<TaskView, { title: string; empty: string }> = {
  inbox: {
    title: "Inbox",
    empty: "Nothing to file. Capture something with c.",
  },
  today: { title: "Today", empty: "No tasks due or available today." },
  upcoming: { title: "Upcoming", empty: "Nothing scheduled ahead." },
  waiting: { title: "Waiting", empty: "Not waiting on anyone." },
  // Not a tab: Someday folds into Upcoming (see upcomingGroups).
  someday: { title: "Someday", empty: "Nothing parked." },
  logbook: {
    title: "Logbook",
    empty: "Nothing completed yet — go do a thing.",
  },
};

const VIEW_TABS: { view: TaskView; to: string }[] = [
  { view: "inbox", to: "/inbox" },
  { view: "today", to: "/tasks/today" },
  { view: "upcoming", to: "/tasks/upcoming" },
  { view: "waiting", to: "/tasks/waiting" },
  { view: "logbook", to: "/tasks/logbook" },
];

export function TasksPage({ view }: { view: TaskView }) {
  const tasks = useTasks(view, view !== "waiting");
  // Upcoming carries the someday list; fetched only there.
  const someday = useTasks("someday", view === "upcoming");
  return (
    <div className="flex flex-col gap-3">
      <header className="flex items-center gap-3 border-b border-border pb-2">
        <h1 className="text-lg font-semibold text-heading">
          {VIEW_META[view].title}
        </h1>
        <nav
          aria-label="Task views"
          className="ml-auto flex gap-0.5 overflow-x-auto"
        >
          {VIEW_TABS.map((tab) => (
            <RouterLink
              key={tab.view}
              to={tab.to}
              className="flex h-7 items-center rounded px-2 text-xs text-muted hover:bg-hover hover:text-heading"
              activeProps={{ className: "bg-selected text-heading" }}
            >
              {VIEW_META[tab.view].title}
            </RouterLink>
          ))}
        </nav>
      </header>
      {view === "inbox" ? <TriageHint /> : null}
      {view === "waiting" ? (
        <WaitingBody />
      ) : (
        <TasksBody
          view={view}
          query={tasks}
          someday={view === "upcoming" ? (someday.data ?? []) : []}
        />
      )}
    </div>
  );
}

function TasksBody({
  view,
  query,
  someday,
}: {
  view: TaskView;
  query: ReturnType<typeof useTasks>;
  someday: Task[];
}) {
  if (query.isPending) return <SkeletonRows />;
  if (query.isError) return <ErrorState error={query.error} />;
  const tasks = query.data;
  if (tasks.length === 0 && someday.length === 0) {
    return (
      <EmptyState
        icon={view === "logbook" ? PartyPopper : InboxIcon}
        title={VIEW_META[view].empty}
      />
    );
  }
  if (view === "upcoming")
    return <GroupedTaskList groups={upcomingGroups(tasks, someday)} />;
  return (
    <TaskListFlat
      tasks={tasks}
      showCompletedOn={view === "logbook"}
      triage={view === "inbox"}
    />
  );
}

/** Upcoming buckets tasks by due date; deferred-only tasks land under their
 * defer date, and anything undated trails at the end. The #someday list
 * folds in last, collapsed: it is reviewed now and then, not worked from,
 * so it earns a section rather than a tab of its own. */
function upcomingGroups(tasks: Task[], someday: Task[]): TaskGroup[] {
  const dated: TaskGroup[] = groupByDate(
    tasks,
    (task) => task.due ?? task.defer,
  ).map((group) => ({
    key: group.date ?? "undated",
    title: group.date ? formatDayHeading(group.date) : "Undated",
    tasks: group.items,
  }));
  return [
    ...dated,
    { key: "someday", title: "Someday", tasks: someday, collapsible: true },
  ];
}

/** One line under the Inbox heading: the keys that empty it. */
function TriageHint() {
  return (
    <p className="-mt-1 text-xs text-muted">
      Select with j/k, then <Key>t</Key> today · <Key>m</Key> tomorrow ·{" "}
      <Key>w</Key> next week · <Key>d</Key> date · <Key>f</Key> defer ·{" "}
      <Key>p</Key> waiting · <Key>s</Key> someday · <Key>n</Key> note ·{" "}
      <Key>x</Key> done
    </p>
  );
}

function Key({ children }: { children: string }) {
  return (
    <kbd className="rounded border border-border bg-hover px-1 font-mono text-[10px] text-heading">
      {children}
    </kbd>
  );
}

/** Waiting, grouped by who owes it: people and companies, then other
 * names, then the unassigned. Oldest first in each; stale ones in red. */
function WaitingBody() {
  const query = useWaitingGroups();
  if (query.isPending) return <SkeletonRows />;
  if (query.isError) return <ErrorState error={query.error} />;
  if (query.data.length === 0) {
    return <EmptyState icon={InboxIcon} title={VIEW_META.waiting.empty} />;
  }
  return <GroupedTaskList groups={waitingGroups(query.data)} showWho={false} />;
}

function waitingGroups(groups: WaitingGroup[]): TaskGroup[] {
  return groups.map((group) => ({
    key: group.path ?? `name:${group.name}`,
    title: group.name || "Unassigned",
    tasks: group.tasks,
    tone: group.stale > 0 ? "danger" : undefined,
  }));
}
