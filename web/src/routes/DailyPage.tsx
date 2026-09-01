// Daily note page (/daily/<date>): prev/next day navigation around the shared
// DocumentScreen. A missing note gets a get-or-create button (POST /daily)
// instead of the generic 404 state — the daily note is the capture spine and
// should always be one click away.
import { Link as RouterLink, useNavigate } from "@tanstack/react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { CalendarX2, ChevronLeft, ChevronRight, Sparkles } from "lucide-react";
import { api } from "../api/client.ts";
import { queryKeys } from "../api/queries.ts";
import {
  addDaysISO,
  formatDayHeading,
  parseISODate,
  todayISO,
} from "../lib/dates.ts";
import { dailyPath } from "../lib/docs.ts";
import { DocumentScreen } from "../components/DocumentScreen.tsx";
import { EmptyState } from "../components/EmptyState.tsx";

export function DailyPage({ date, edit }: { date: string; edit: boolean }) {
  if (!parseISODate(date)) {
    return (
      <EmptyState
        icon={CalendarX2}
        title="That's not a date"
        hint={`Expected YYYY-MM-DD, got "${date}".`}
      />
    );
  }
  return (
    <div className="flex flex-col gap-3">
      <DayNav date={date} />
      <DocumentScreen
        key={dailyPath(date)}
        path={dailyPath(date)}
        initialEdit={edit}
        notFoundFallback={<StartDayButton date={date} />}
      />
    </div>
  );
}

function DayNav({ date }: { date: string }) {
  const isToday = date === todayISO();
  return (
    <nav
      aria-label="Day navigation"
      className="flex items-center gap-1 print:hidden"
    >
      <RouterLink
        to={`/daily/${addDaysISO(date, -1)}`}
        aria-label="Previous day"
        className="flex size-7 items-center justify-center rounded border border-border text-muted hover:bg-hover hover:text-heading"
      >
        <ChevronLeft className="size-4" aria-hidden="true" />
      </RouterLink>
      <RouterLink
        to={`/daily/${addDaysISO(date, 1)}`}
        aria-label="Next day"
        className="flex size-7 items-center justify-center rounded border border-border text-muted hover:bg-hover hover:text-heading"
      >
        <ChevronRight className="size-4" aria-hidden="true" />
      </RouterLink>
      {!isToday ? (
        <RouterLink
          to={`/daily/${todayISO()}`}
          className="flex h-7 items-center rounded border border-border px-2 text-xs text-muted hover:bg-hover hover:text-heading"
        >
          Today
        </RouterLink>
      ) : null}
      <span className="ml-2 text-xs text-muted">{formatDayHeading(date)}</span>
    </nav>
  );
}

function StartDayButton({ date }: { date: string }) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const create = useMutation({
    mutationFn: () => api.createDaily(date),
    onSuccess: (doc) => {
      queryClient.setQueryData(queryKeys.document(doc.path), doc);
      // Re-render straight into the editor with the fresh note.
      void navigate({
        to: `/daily/${date}`,
        search: { edit: true },
        replace: true,
      });
    },
  });
  return (
    <div className="flex flex-col gap-1.5">
      <button
        type="button"
        onClick={() => create.mutate()}
        disabled={create.isPending}
        className="flex h-11 items-center justify-center gap-2 rounded border border-dashed border-border text-sm text-muted hover:border-accent/50 hover:bg-hover hover:text-heading"
      >
        <Sparkles className="size-4" aria-hidden="true" />
        {create.isPending
          ? "Starting…"
          : `Start the note for ${formatDayHeading(date)}`}
      </button>
      {create.isError ? (
        <p className="text-xs text-danger">
          Couldn't create — {create.error.message}
        </p>
      ) : null}
    </div>
  );
}
