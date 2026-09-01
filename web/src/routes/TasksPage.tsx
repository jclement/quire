// Task views: /inbox and /tasks/<today|upcoming|waiting|logbook>. All render
// the same dense rows; Upcoming groups by due date, Logbook shows completion
// dates. j/k + x + Enter come from GroupedTaskList.
import { Link as RouterLink } from "@tanstack/react-router";
import { Inbox as InboxIcon, PartyPopper } from "lucide-react";
import { useTasks } from "../api/queries.ts";
import type { Task, TaskView } from "../api/types.ts";
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
  const tasks = useTasks(view);
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
      <TasksBody view={view} query={tasks} />
    </div>
  );
}

function TasksBody({
  view,
  query,
}: {
  view: TaskView;
  query: ReturnType<typeof useTasks>;
}) {
  if (query.isPending) return <SkeletonRows />;
  if (query.isError) return <ErrorState error={query.error} />;
  const tasks = query.data;
  if (tasks.length === 0) {
    return (
      <EmptyState
        icon={view === "logbook" ? PartyPopper : InboxIcon}
        title={VIEW_META[view].empty}
      />
    );
  }
  if (view === "upcoming")
    return <GroupedTaskList groups={upcomingGroups(tasks)} />;
  return <TaskListFlat tasks={tasks} showCompletedOn={view === "logbook"} />;
}

/** Upcoming buckets tasks by due date; deferred-only tasks land under their
 * defer date, and anything undated trails at the end. */
function upcomingGroups(tasks: Task[]): TaskGroup[] {
  return groupByDate(tasks, (task) => task.due ?? task.defer).map((group) => ({
    key: group.date ?? "undated",
    title: group.date ? formatDayHeading(group.date) : "Someday",
    tasks: group.items,
  }));
}
