// The week: planning and retro, one tier above the daily capture note.
// Everything above the note is composed from the index — what landed, what
// slipped, what is still owed, and what has gone quiet — so a Friday review
// is a reading rather than a remembering.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link as RouterLink, useNavigate } from "@tanstack/react-router";
import {
  CalendarRange,
  ChevronLeft,
  ChevronRight,
  Pencil,
  Sparkles,
} from "lucide-react";
import type { ReactNode } from "react";
import { api, errorMessage } from "../api/client.ts";
import { useEffectiveArea } from "../api/queries.ts";
import type { DocMeta, Document } from "../api/types.ts";
import { docHref, DOC_TYPE_INFO, isDocType } from "../lib/docs.ts";
import { Markdown } from "../components/Markdown.tsx";
import { TaskListFlat } from "../components/TaskList.tsx";
import { SkeletonRows } from "../components/Skeleton.tsx";
import { ErrorState } from "../components/EmptyState.tsx";

export function WeeklyPage({ week }: { week: string }) {
  const area = useEffectiveArea();
  const review = useQuery({
    queryKey: ["weekly", week, area],
    queryFn: () => api.weekReview(week, area),
  });

  if (review.isPending) return <SkeletonRows count={6} />;
  if (review.isError) return <ErrorState error={review.error} />;
  const data = review.data;

  const nothing =
    data.completed.length === 0 &&
    data.slipped.length === 0 &&
    data.waiting.length === 0 &&
    data.stalled.length === 0 &&
    data.meetings.length === 0;

  return (
    <div className="flex flex-col gap-5">
      <header className="flex flex-wrap items-center gap-2 border-b border-border pb-2">
        <CalendarRange
          className="size-4 shrink-0 text-muted"
          aria-hidden="true"
        />
        <h1 className="text-lg font-semibold text-heading">{data.week}</h1>
        <span className="font-mono text-xs text-muted">
          {data.start} → {data.end}
        </span>
        <nav
          aria-label="Other weeks"
          className="ml-auto flex items-center gap-1"
        >
          <RouterLink
            to="/weekly/$week"
            params={{ week: data.prev }}
            aria-label={`Week ${data.prev}`}
            className="flex size-7 items-center justify-center rounded border border-border text-muted hover:bg-hover hover:text-heading"
          >
            <ChevronLeft className="size-3.5" aria-hidden="true" />
          </RouterLink>
          <RouterLink
            to="/weekly/$week"
            params={{ week: data.next }}
            aria-label={`Week ${data.next}`}
            className="flex size-7 items-center justify-center rounded border border-border text-muted hover:bg-hover hover:text-heading"
          >
            <ChevronRight className="size-3.5" aria-hidden="true" />
          </RouterLink>
        </nav>
      </header>

      <Section title="Landed" count={data.completed.length}>
        {data.completed.length > 0 ? (
          <TaskListFlat tasks={data.completed} showCompletedOn />
        ) : (
          <Quiet>Nothing completed in this week yet.</Quiet>
        )}
      </Section>

      {data.slipped.length > 0 ? (
        <Section title="Slipped" count={data.slipped.length}>
          <TaskListFlat tasks={data.slipped} />
        </Section>
      ) : null}

      {data.waiting.length > 0 ? (
        <Section title="Still waiting on" count={data.waiting.length}>
          <TaskListFlat tasks={data.waiting} />
        </Section>
      ) : null}

      {data.stalled.length > 0 ? (
        <Section
          title="Projects with no next action"
          count={data.stalled.length}
        >
          <p className="mb-1.5 text-xs text-muted">
            Active, and nothing open is pointing at them.
          </p>
          <DocList docs={data.stalled} />
        </Section>
      ) : null}

      {data.meetings.length > 0 ? (
        <Section title="Meetings" count={data.meetings.length}>
          <DocList docs={data.meetings} />
        </Section>
      ) : null}

      {data.touched.length > 0 ? (
        <Section title="Touched" count={data.touched.length}>
          <DocList docs={data.touched} />
        </Section>
      ) : null}

      {nothing ? (
        <Quiet>A quiet week, or one that has not happened yet.</Quiet>
      ) : null}

      <WeekNote week={data.week} note={data.note} />
    </div>
  );
}

function Section({
  title,
  count,
  children,
}: {
  title: string;
  count: number;
  children: ReactNode;
}) {
  return (
    <section aria-label={title}>
      <h2 className="mb-1.5 flex items-center gap-2 text-[10px] font-semibold tracking-wider text-muted uppercase">
        {title}
        <span className="font-mono text-[10px] normal-case">{count}</span>
      </h2>
      {children}
    </section>
  );
}

function Quiet({ children }: { children: ReactNode }) {
  return <p className="text-xs text-muted">{children}</p>;
}

function DocList({ docs }: { docs: DocMeta[] }) {
  return (
    <ul className="divide-y divide-border border-y border-border">
      {docs.map((doc) => {
        const Icon = isDocType(doc.type) ? DOC_TYPE_INFO[doc.type].icon : null;
        return (
          <li key={doc.path}>
            <RouterLink
              to={docHref(doc.path)}
              className="flex items-center gap-2 px-2 py-1.5 text-sm text-body hover:bg-hover hover:text-heading"
            >
              {Icon ? (
                <Icon
                  className="size-3.5 shrink-0 text-muted"
                  aria-hidden="true"
                />
              ) : null}
              <span className="truncate">{doc.title}</span>
            </RouterLink>
          </li>
        );
      })}
    </ul>
  );
}

/** The week's own note, created on demand like the daily one. */
function WeekNote({ week, note }: { week: string; note: Document | null }) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const start = useMutation({
    mutationFn: () => api.createWeekly(week),
    onSuccess: (doc) => {
      void queryClient.invalidateQueries({ queryKey: ["weekly"] });
      void navigate({ to: docHref(doc.path), search: { edit: true } });
    },
  });
  return (
    <section>
      <h2 className="mb-1.5 flex items-center text-[10px] font-semibold tracking-wider text-muted uppercase">
        This week's note
        {note ? (
          <RouterLink
            to={docHref(note.path)}
            search={{ edit: true }}
            className="ml-auto flex h-6 items-center gap-1 rounded border border-border px-1.5 font-sans text-[10px] font-normal normal-case text-body hover:bg-hover hover:text-heading"
          >
            <Pencil className="size-3" aria-hidden="true" />
            Edit
          </RouterLink>
        ) : null}
      </h2>
      {note ? (
        <div className="rounded border border-border bg-raised px-4 py-3">
          <Markdown markdown={note.markdown} />
        </div>
      ) : (
        <button
          type="button"
          onClick={() => start.mutate()}
          disabled={start.isPending}
          className="flex h-11 w-full items-center justify-center gap-2 rounded border border-dashed border-border text-sm text-muted hover:border-accent/50 hover:bg-hover hover:text-heading"
        >
          <Sparkles className="size-4" aria-hidden="true" />
          {start.isPending ? "Starting…" : `Start the note for ${week}`}
        </button>
      )}
      {start.isError ? (
        <p className="mt-1.5 text-xs text-danger">
          {errorMessage(start.error)}
        </p>
      ) : null}
    </section>
  );
}
