// The home screen: date header, today's meetings, task sections (overdue / due
// / available / waiting) as one keyboard-navigable list, recent documents, and
// the daily note rendered inline (or a "start" affordance if it doesn't exist
// yet).
import { Link as RouterLink, useNavigate } from "@tanstack/react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { CalendarDays, Coffee, Pencil, Sparkles } from "lucide-react";
import { api } from "../api/client.ts";
import { queryKeys, useToday, useToggleTask } from "../api/queries.ts";
import type { Birthday, DocMeta, TodayPayload } from "../api/types.ts";
import {
  birthdayWhen,
  formatDayHeading,
  formatRelativeTime,
} from "../lib/dates.ts";
import { docHref, DOC_TYPE_INFO } from "../lib/docs.ts";
import { EmptyState, ErrorState } from "../components/EmptyState.tsx";
import { Markdown } from "../components/Markdown.tsx";
import { SkeletonRows } from "../components/Skeleton.tsx";
import { GroupedTaskList, type TaskGroup } from "../components/TaskList.tsx";

export function TodayPage() {
  const today = useToday();
  if (today.isPending) {
    return (
      <div className="flex flex-col gap-4">
        <div className="h-8 w-64 animate-pulse rounded bg-hover" />
        <SkeletonRows count={7} />
      </div>
    );
  }
  if (today.isError) return <ErrorState error={today.error} />;
  return <TodayView payload={today.data} />;
}

function TodayView({ payload }: { payload: TodayPayload }) {
  const groups: TaskGroup[] = [
    {
      key: "overdue",
      title: "Overdue",
      tasks: payload.overdue,
      tone: "danger",
    },
    { key: "due", title: "Due today", tasks: payload.due_today, tone: "warn" },
    { key: "available", title: "Available", tasks: payload.available },
    { key: "waiting", title: "Waiting", tasks: payload.waiting },
  ];
  const anyTasks = groups.some((group) => group.tasks.length > 0);

  return (
    <div className="flex flex-col gap-6">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight text-heading">
          {formatDayHeading(payload.date)}
        </h1>
      </header>

      <MeetingsToday meetings={payload.meetings} />
      <Birthdays birthdays={payload.birthdays} />

      {anyTasks ? (
        <GroupedTaskList groups={groups} />
      ) : (
        <EmptyState
          icon={Coffee}
          title="Nothing on the hook"
          hint="No overdue, due, or available tasks. Enjoy it."
        />
      )}

      <RecentDocs recent={payload.recent} />
      <DailyNoteSection payload={payload} />
    </div>
  );
}

function MeetingsToday({ meetings }: { meetings: DocMeta[] }) {
  if (meetings.length === 0) return null;
  return (
    <section>
      <h2 className="mb-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted">
        Meetings today
      </h2>
      <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
        {meetings.map((meeting) => (
          <RouterLink
            key={meeting.path}
            to={docHref(meeting.path)}
            className="flex min-h-11 flex-col justify-center gap-0.5 rounded border border-border bg-raised px-3 py-2 hover:border-accent/50 hover:bg-hover"
          >
            <span className="flex items-center gap-1.5 text-sm font-medium text-heading">
              <CalendarDays
                className="size-3.5 shrink-0 text-accent"
                aria-hidden="true"
              />
              <span className="truncate">{meeting.title}</span>
            </span>
            {meeting.tags.length > 0 ? (
              <span className="truncate text-[10px] text-muted">
                {meeting.tags.map((tag) => `#${tag}`).join(" ")}
              </span>
            ) : null}
          </RouterLink>
        ))}
      </div>
    </section>
  );
}

/** Upcoming birthdays, small and warm: "🎂 Sarah Chen — Friday (turns 41)". */
function Birthdays({ birthdays }: { birthdays: Birthday[] }) {
  if (birthdays.length === 0) return null;
  return (
    <section>
      <h2 className="mb-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted">
        Birthdays
      </h2>
      <ul className="divide-y divide-border border-y border-border">
        {birthdays.map((birthday) => (
          <li key={birthday.path}>
            <RouterLink
              to={docHref(birthday.path)}
              className="flex h-8 items-center gap-2 px-2 text-sm text-body hover:bg-hover hover:text-heading"
            >
              <span aria-hidden="true">🎂</span>
              <span className="truncate font-medium text-heading">
                {birthday.title}
              </span>
              <span
                className={
                  birthday.days_until === 0 ? "text-accent" : "text-muted"
                }
              >
                — {birthdayWhen(birthday.date, birthday.days_until)}
                {birthday.age !== null ? ` (turns ${birthday.age})` : ""}
              </span>
            </RouterLink>
          </li>
        ))}
      </ul>
    </section>
  );
}

function RecentDocs({ recent }: { recent: DocMeta[] }) {
  if (recent.length === 0) return null;
  return (
    <section>
      <h2 className="mb-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted">
        Recent
      </h2>
      <ul className="divide-y divide-border border-y border-border">
        {recent.map((doc) => {
          const Icon = DOC_TYPE_INFO[doc.type].icon;
          return (
            <li key={doc.path}>
              <RouterLink
                to={docHref(doc.path)}
                className="flex h-8 items-center gap-2 px-2 text-sm text-body hover:bg-hover hover:text-heading"
              >
                <Icon
                  className="size-3.5 shrink-0 text-muted"
                  aria-hidden="true"
                />
                <span className="truncate">{doc.title}</span>
                <span className="ml-auto shrink-0 text-xs text-muted">
                  {formatRelativeTime(doc.mtime)}
                </span>
              </RouterLink>
            </li>
          );
        })}
      </ul>
    </section>
  );
}

function DailyNoteSection({ payload }: { payload: TodayPayload }) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const toggleTask = useToggleTask();
  const startDaily = useMutation({
    mutationFn: () => api.createDaily(payload.date),
    onSuccess: (doc) => {
      queryClient.setQueryData(queryKeys.document(doc.path), doc);
      void queryClient.invalidateQueries({ queryKey: queryKeys.today });
      void navigate({ to: `/daily/${payload.date}`, search: { edit: true } });
    },
  });

  return (
    <section>
      <h2 className="mb-1.5 flex items-center text-[10px] font-semibold uppercase tracking-wider text-muted">
        Daily note
        {payload.daily ? (
          <RouterLink
            to={`/daily/${payload.date}`}
            search={{ edit: true }}
            className="ml-auto flex h-6 items-center gap-1 rounded border border-border px-1.5 font-sans text-[10px] font-normal normal-case text-body hover:bg-hover hover:text-heading"
          >
            <Pencil className="size-3" aria-hidden="true" />
            Edit
          </RouterLink>
        ) : null}
      </h2>
      {payload.daily ? (
        <div className="rounded border border-border bg-raised px-4 py-3">
          <Markdown
            markdown={payload.daily.markdown}
            links={payload.daily.links}
            tasks={payload.daily.tasks}
            onToggleTask={(task) => toggleTask.mutate(task)}
          />
        </div>
      ) : (
        <button
          type="button"
          onClick={() => startDaily.mutate()}
          disabled={startDaily.isPending}
          className="flex h-11 w-full items-center justify-center gap-2 rounded border border-dashed border-border text-sm text-muted hover:border-accent/50 hover:bg-hover hover:text-heading"
        >
          <Sparkles className="size-4" aria-hidden="true" />
          {startDaily.isPending ? "Starting…" : "Start today's note"}
        </button>
      )}
      {startDaily.isError ? (
        <p className="mt-1.5 text-xs text-danger">
          Couldn't create the note — {startDaily.error.message}
        </p>
      ) : null}
    </section>
  );
}
