// The journal: every daily note as one scrolling page, newest first. Today
// sits at the top (with a "start" affordance if it has no note yet) and
// history loads a page at a time as you scroll, so reviewing last week is a
// flick rather than seven clicks through Previous day.
//
// Each day renders read-only through the same Markdown component as the
// document page, so task checkboxes toggle in place; the day's heading
// links through to its editable page.
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { Link as RouterLink, useNavigate } from "@tanstack/react-router";
import { CalendarDays, Pencil, Sparkles } from "lucide-react";
import { useEffect, useRef } from "react";
import { api, ApiError, errorMessage } from "../api/client.ts";
import { queryKeys, useToggleTask } from "../api/queries.ts";
import type { Document } from "../api/types.ts";
import { Markdown } from "../components/Markdown.tsx";
import { SkeletonRows } from "../components/Skeleton.tsx";
import { formatDayHeading, todayISO } from "../lib/dates.ts";

const PAGE = 10;

export function JournalPage() {
  const today = todayISO();
  const history = useInfiniteQuery({
    queryKey: ["journal", today],
    queryFn: ({ pageParam }) => api.listDaily(pageParam, PAGE),
    initialPageParam: today,
    // The next page starts before the oldest note we have; no notes means
    // we have reached the beginning of the vault.
    getNextPageParam: (last) =>
      last.length < PAGE ? undefined : dateOf(last[last.length - 1]!.path),
  });

  // Load more when the sentinel scrolls into view.
  const sentinel = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const el = sentinel.current;
    if (!el || !history.hasNextPage) return;
    const observer = new IntersectionObserver((entries) => {
      if (
        entries.some((e) => e.isIntersecting) &&
        !history.isFetchingNextPage
      ) {
        void history.fetchNextPage();
      }
    });
    observer.observe(el);
    return () => observer.disconnect();
  }, [
    history.hasNextPage,
    history.isFetchingNextPage,
    history.fetchNextPage,
    history,
  ]);

  const days = history.data?.pages.flat() ?? [];

  return (
    <div className="flex max-w-3xl flex-col gap-4">
      <header className="flex items-center gap-2 border-b border-border pb-2 print:hidden">
        <CalendarDays className="size-4 text-muted" aria-hidden="true" />
        <h1 className="text-lg font-semibold text-heading">Journal</h1>
        <RouterLink
          to={`/daily/${today}`}
          className="ml-auto flex h-7 items-center rounded border border-border px-2 text-xs text-muted hover:bg-hover hover:text-heading"
        >
          Single day
        </RouterLink>
      </header>

      <TodayEntry date={today} />

      {history.isPending ? (
        <SkeletonRows count={6} />
      ) : history.isError ? (
        <p className="text-xs text-danger">{errorMessage(history.error)}</p>
      ) : (
        days.map((doc) => <DayEntry key={doc.path} doc={doc} />)
      )}

      <div ref={sentinel} className="h-8" aria-hidden="true">
        {history.isFetchingNextPage ? <SkeletonRows count={2} /> : null}
      </div>
      {!history.hasNextPage && days.length > 0 ? (
        <p className="pb-6 text-center text-xs text-muted">
          That's the beginning.
        </p>
      ) : null}
      {!history.isPending && days.length === 0 ? (
        <p className="text-xs text-muted">
          No earlier days yet — the journal fills in as you write.
        </p>
      ) : null}
    </div>
  );
}

/** Today: the existing note, or a one-tap way to start it. */
function TodayEntry({ date }: { date: string }) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const doc = useQuery({
    queryKey: queryKeys.document(`daily/${date}.md`),
    queryFn: () => api.getDaily(date),
    retry: false,
  });
  const start = useMutation({
    mutationFn: () => api.createDaily(date),
    onSuccess: (created) => {
      queryClient.setQueryData(queryKeys.document(created.path), created);
      void navigate({ to: `/daily/${date}`, search: { edit: true } });
    },
  });

  if (doc.isPending) return <SkeletonRows count={3} />;
  if (
    doc.isError &&
    !(doc.error instanceof ApiError && doc.error.status === 404)
  ) {
    return <p className="text-xs text-danger">{errorMessage(doc.error)}</p>;
  }
  if (doc.isError) {
    return (
      <section>
        <DayHeading date={date} />
        <button
          type="button"
          onClick={() => start.mutate()}
          disabled={start.isPending}
          className="flex h-11 w-full items-center justify-center gap-2 rounded border border-dashed border-border text-sm text-muted hover:border-accent/50 hover:bg-hover hover:text-heading"
        >
          <Sparkles className="size-4" aria-hidden="true" />
          {start.isPending ? "Starting…" : "Start today's note"}
        </button>
      </section>
    );
  }
  return <DayEntry doc={doc.data} />;
}

function DayEntry({ doc }: { doc: Document }) {
  const toggleTask = useToggleTask();
  const date = dateOf(doc.path);
  return (
    <section aria-label={formatDayHeading(date)} className="break-inside-avoid">
      <DayHeading date={date} />
      <div className="rounded border border-border bg-raised px-4 py-3">
        <Markdown
          markdown={doc.markdown}
          links={doc.links}
          tasks={doc.tasks}
          onToggleTask={(task) => toggleTask.mutate(task)}
        />
      </div>
    </section>
  );
}

function DayHeading({ date }: { date: string }) {
  const isToday = date === todayISO();
  return (
    <h2 className="mb-1.5 flex items-center gap-2 text-[10px] font-semibold uppercase tracking-wider text-muted">
      <RouterLink to={`/daily/${date}`} className="hover:text-heading">
        {formatDayHeading(date)}
      </RouterLink>
      {isToday ? (
        <span className="rounded bg-selected px-1 text-accent">today</span>
      ) : null}
      <RouterLink
        to={`/daily/${date}`}
        search={{ edit: true }}
        aria-label={`Edit ${formatDayHeading(date)}`}
        className="ml-auto flex h-6 items-center gap-1 rounded border border-border px-1.5 font-sans text-[10px] font-normal normal-case text-body hover:bg-hover hover:text-heading print:hidden"
      >
        <Pencil className="size-3" aria-hidden="true" />
        Edit
      </RouterLink>
    </h2>
  );
}

/** "daily/2026-09-02.md" → "2026-09-02". */
function dateOf(path: string): string {
  return path.replace(/^daily\//, "").replace(/\.md$/, "");
}
