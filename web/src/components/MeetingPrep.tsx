// Meeting prep: for each attendee with a person page, when you last met,
// what is open that mentions them, what you are waiting on them for, their
// company, and what else you have written about them lately. Two shapes of
// one view — the narrow margin column beside a meeting note ("rail"), and
// the dialog behind "Prep" on Today's agenda ("full").
import { Link as RouterLink } from "@tanstack/react-router";
import { Hourglass, Square } from "lucide-react";
import { errorMessage } from "../api/client.ts";
import { useMeetingPrep, type PrepTarget } from "../api/calendar.ts";
import type { MeetingPrep, PrepPerson, Task } from "../api/types.ts";
import { docHref } from "../lib/docs.ts";
import { SkeletonRows } from "./Skeleton.tsx";

type Variant = "rail" | "full";

/** Tasks listed per person before the rest collapse into "+N more". */
const TASKS_SHOWN: Record<Variant, number> = { rail: 3, full: 8 };

export function MeetingPrepView({
  target,
  variant,
}: {
  target: PrepTarget;
  variant: Variant;
}) {
  const prep = useMeetingPrep(target);
  if (prep.isPending) return <SkeletonRows count={3} />;
  if (prep.isError) {
    return <p className="text-xs text-danger">{errorMessage(prep.error)}</p>;
  }
  return <PrepBody prep={prep.data} variant={variant} />;
}

function PrepBody({ prep, variant }: { prep: MeetingPrep; variant: Variant }) {
  if (prep.people.length === 0 && prep.unmatched.length === 0) {
    return (
      <p className="text-xs text-muted">
        No attendees yet — add people to this meeting to see their context.
      </p>
    );
  }
  return (
    <div className="flex flex-col gap-3">
      {prep.people.map((person) => (
        <PersonPrep
          key={person.person.path}
          person={person}
          variant={variant}
        />
      ))}
      {prep.unmatched.length > 0 ? (
        <p className="text-xs text-muted">
          <span className="font-medium">No page:</span>{" "}
          {prep.unmatched.join(", ")}
        </p>
      ) : null}
    </div>
  );
}

function PersonPrep({
  person,
  variant,
}: {
  person: PrepPerson;
  variant: Variant;
}) {
  const nothing =
    !person.last_meeting &&
    person.open_tasks.length === 0 &&
    person.waiting.length === 0 &&
    person.recent_notes.length === 0;
  return (
    <section aria-label={`Prep: ${person.person.title}`} className="text-xs">
      <div className="flex flex-wrap items-baseline gap-x-1.5">
        <RouterLink
          to={docHref(person.person.path)}
          className="font-medium text-heading hover:text-accent"
        >
          {person.person.title}
        </RouterLink>
        {person.company ? (
          person.company.path ? (
            <RouterLink
              to={docHref(person.company.path)}
              className="text-muted hover:text-body"
            >
              {person.company.name}
            </RouterLink>
          ) : (
            <span className="text-muted">{person.company.name}</span>
          )
        ) : null}
      </div>
      <div className="mt-0.5 flex flex-col gap-0.5 border-l border-border pl-2">
        {person.last_meeting ? (
          <p className="text-muted">
            Last met{" "}
            <RouterLink
              to={docHref(person.last_meeting.path)}
              className="text-body hover:text-accent"
            >
              {person.last_meeting.title}
            </RouterLink>{" "}
            <span className="whitespace-nowrap font-mono text-[10px]">
              {person.last_meeting.date.slice(0, 10)}
            </span>
          </p>
        ) : (
          <p className="text-muted">First meeting on file.</p>
        )}
        <PrepTasks
          tasks={person.waiting}
          waiting
          limit={TASKS_SHOWN[variant]}
        />
        <PrepTasks tasks={person.open_tasks} limit={TASKS_SHOWN[variant]} />
        {person.recent_notes.length > 0 ? (
          <p className="text-muted">
            Notes:{" "}
            {person.recent_notes.map((note, index) => (
              <span key={note.path}>
                {index > 0 ? ", " : ""}
                <RouterLink
                  to={docHref(note.path)}
                  className="text-body hover:text-accent"
                >
                  {note.title}
                </RouterLink>
              </span>
            ))}
          </p>
        ) : null}
        {nothing ? <p className="text-muted">Nothing open with them.</p> : null}
      </div>
    </section>
  );
}

function PrepTasks({
  tasks,
  waiting = false,
  limit,
}: {
  tasks: Task[];
  waiting?: boolean;
  limit: number;
}) {
  if (tasks.length === 0) return null;
  const Icon = waiting ? Hourglass : Square;
  const shown = tasks.slice(0, limit);
  return (
    <ul aria-label={waiting ? "Waiting on them" : "Open tasks"}>
      {shown.map((task) => (
        <li key={task.id}>
          <RouterLink
            to={docHref(task.doc_path)}
            title={`${task.text} — in ${task.doc_title}`}
            className="flex items-start gap-1 text-body hover:text-accent"
          >
            <Icon
              className={`mt-0.5 size-3 shrink-0 ${waiting ? "text-warn" : "text-muted"}`}
              aria-hidden="true"
            />
            <span className="line-clamp-2">
              {task.text}
              {waiting && task.waiting_for?.days != null ? (
                <span
                  title={
                    task.waiting_for.since
                      ? `waiting since ${task.waiting_for.since}`
                      : undefined
                  }
                  className={`ml-1 whitespace-nowrap font-mono text-[10px] ${task.waiting_for.stale ? "text-danger" : "text-muted"}`}
                >
                  {task.waiting_for.days}d
                </span>
              ) : null}
              {task.due ? (
                <span className="ml-1 whitespace-nowrap font-mono text-[10px] text-muted">
                  {task.due.slice(5)}
                </span>
              ) : null}
            </span>
          </RouterLink>
        </li>
      ))}
      {tasks.length > shown.length ? (
        <li className="pl-4 text-muted">+{tasks.length - shown.length} more</li>
      ) : null}
    </ul>
  );
}
