// The month calendar (/calendar, /calendar/YYYY-MM): one screen of what a
// month actually held — which days have a daily note, meetings, documents
// touched, tasks completed. Weeks start Monday. Empty days stay empty: this
// is a calm overview, not a heatmap, so nothing is shaded by volume.
//
// Two shapes, one grid. On md+ a cell lists titles you can click. A phone
// column is ~45px wide, where titles would be unreadable and a horizontal
// scroll unforgivable, so below md the whole cell becomes one tap target for
// that day's note and dots stand in for its documents.
//
// The day number always links to /daily/<date> — the daily page creates the
// note when there isn't one, which makes the calendar a way into journalling
// a past day. Accent means the note already exists.
import { Link as RouterLink, useNavigate } from "@tanstack/react-router";
import {
  CalendarRange,
  ChevronLeft,
  ChevronRight,
  type LucideIcon,
} from "lucide-react";
import { useEffect } from "react";
import { useCalendar } from "../api/queries.ts";
import type { CalendarDay, CalendarMonth } from "../api/types.ts";
import { todayISO } from "../lib/dates.ts";
import { docHref } from "../lib/docs.ts";
import {
  currentMonthKey,
  formatMonthLabel,
  monthGrid,
  otherTouched,
  WEEKDAY_LABELS,
} from "../lib/calendar.ts";
import { useUi } from "../keys/UiContext.tsx";
import { ErrorState } from "../components/EmptyState.tsx";
import { SkeletonRows } from "../components/Skeleton.tsx";

/** Documents listed in a cell before the rest collapse into "+N". */
const MAX_TOUCHED_SHOWN = 3;

/** Dots a phone cell shows before it stops counting. */
const MAX_DOTS = 4;

export function CalendarPage({ month }: { month: string }) {
  const calendar = useCalendar(month);
  useMonthKeys(calendar.data);

  return (
    <div className="flex flex-col gap-3">
      <MonthNav month={month} payload={calendar.data} />
      {calendar.isPending ? (
        <SkeletonRows count={6} />
      ) : calendar.isError ? (
        <ErrorState error={calendar.error} />
      ) : (
        <MonthGrid payload={calendar.data} />
      )}
    </div>
  );
}

/** `[` and `]` page months — registered only while this route is mounted. */
function useMonthKeys(payload: CalendarMonth | undefined): void {
  const navigate = useNavigate();
  const { registerKey } = useUi();
  const prev = payload?.prev;
  const next = payload?.next;

  useEffect(() => {
    if (!prev || !next) return;
    const unregisterPrev = registerKey("[", () => {
      void navigate({ to: `/calendar/${prev}` });
    });
    const unregisterNext = registerKey("]", () => {
      void navigate({ to: `/calendar/${next}` });
    });
    return () => {
      unregisterPrev();
      unregisterNext();
    };
  }, [prev, next, navigate, registerKey]);
}

function MonthNav({
  month,
  payload,
}: {
  month: string;
  payload: CalendarMonth | undefined;
}) {
  const current = currentMonthKey();
  return (
    <header className="flex items-center gap-2 border-b border-border pb-2">
      <CalendarRange
        className="size-4 shrink-0 text-muted"
        aria-hidden="true"
      />
      <h1 className="text-lg font-semibold text-heading">
        {formatMonthLabel(month)}
      </h1>
      <div className="ml-auto flex items-center gap-1">
        <NavArrow
          to={payload && `/calendar/${payload.prev}`}
          label="Previous month"
          icon={ChevronLeft}
          hint="["
        />
        <NavArrow
          to={payload && `/calendar/${payload.next}`}
          label="Next month"
          icon={ChevronRight}
          hint="]"
        />
        {month !== current ? (
          <RouterLink
            to={`/calendar/${current}`}
            className="flex h-11 items-center rounded border border-border px-3 text-xs text-muted hover:bg-hover hover:text-heading md:h-7 md:px-2"
          >
            Today
          </RouterLink>
        ) : null}
      </div>
    </header>
  );
}

/** Chevron paging; inert (but still there) until the payload names a month. */
function NavArrow({
  to,
  label,
  icon: Icon,
  hint,
}: {
  to: string | undefined;
  label: string;
  icon: LucideIcon;
  hint: string;
}) {
  const classes =
    "flex size-11 items-center justify-center rounded border border-border text-muted md:size-7";
  if (!to) {
    return (
      <span className={`${classes} opacity-40`} aria-hidden="true">
        <Icon className="size-4" />
      </span>
    );
  }
  return (
    <RouterLink
      to={to}
      aria-label={label}
      title={`${label} — ${hint}`}
      className={`${classes} hover:bg-hover hover:text-heading`}
    >
      <Icon className="size-4" aria-hidden="true" />
    </RouterLink>
  );
}

function MonthGrid({ payload }: { payload: CalendarMonth }) {
  const today = todayISO();
  const cells = monthGrid(payload.days).flat();
  return (
    <div className="grid grid-cols-7 border-t border-l border-border">
      {WEEKDAY_LABELS.map((label) => (
        <div
          key={label}
          className="border-r border-b border-border px-1 py-1 text-center text-[10px] font-semibold tracking-wider text-muted uppercase"
        >
          <span className="md:hidden">{label.slice(0, 1)}</span>
          <span className="hidden md:inline">{label}</span>
        </div>
      ))}
      {cells.map((day, at) => (
        <div
          key={day?.date ?? `blank-${at}`}
          className="border-r border-b border-border"
        >
          {day ? <DayCell day={day} isToday={day.date === today} /> : null}
        </div>
      ))}
    </div>
  );
}

function DayCell({ day, isToday }: { day: CalendarDay; isToday: boolean }) {
  const others = otherTouched(day);
  const shown = others.slice(0, MAX_TOUCHED_SHOWN);
  const overflow = others.length - shown.length;
  const dots = [
    ...day.meetings.map(() => true),
    ...others.map(() => false),
  ].slice(0, MAX_DOTS);

  return (
    <>
      {/* Phone: the cell is the day's note, with dots for what else it held. */}
      <RouterLink
        to={`/daily/${day.date}`}
        aria-label={`Note for ${day.date}`}
        className="flex min-h-16 flex-col items-center gap-1 p-1 hover:bg-hover md:hidden"
      >
        <DayNumber day={day} isToday={isToday} />
        <span className="flex flex-wrap items-center justify-center gap-0.5">
          {dots.map((isMeeting, at) => (
            <span
              key={at}
              className={`size-1.5 rounded-full ${
                isMeeting ? "bg-accent" : "bg-muted/60"
              }`}
            />
          ))}
        </span>
        {day.completed_tasks > 0 ? (
          <span className="font-mono text-[10px] text-muted">
            ✓{day.completed_tasks}
          </span>
        ) : null}
      </RouterLink>

      {/* Desktop: titles, clickable. */}
      <div className="hidden min-h-24 flex-col gap-0.5 p-1 md:flex">
        <div className="flex items-center gap-1">
          <RouterLink
            to={`/daily/${day.date}`}
            title={
              day.has_daily
                ? "Open this day's note"
                : "Start a note for this day"
            }
            className="rounded-full hover:bg-hover"
          >
            <DayNumber day={day} isToday={isToday} />
          </RouterLink>
          {day.completed_tasks > 0 ? (
            <span
              title={`${day.completed_tasks} tasks completed`}
              className="ml-auto rounded-sm bg-hover px-1 font-mono text-[10px] text-muted"
            >
              ✓{day.completed_tasks}
            </span>
          ) : null}
        </div>
        {day.meetings.map((meeting) => (
          <RouterLink
            key={meeting.path}
            to={docHref(meeting.path)}
            title={meeting.title}
            className="truncate border-l-2 border-accent/60 pl-1 text-[11px] leading-tight text-body hover:bg-hover hover:text-accent"
          >
            {meeting.title}
          </RouterLink>
        ))}
        {shown.map((doc) => (
          <RouterLink
            key={doc.path}
            to={docHref(doc.path)}
            title={doc.title}
            className="truncate pl-1 text-[11px] leading-tight text-muted hover:bg-hover hover:text-body"
          >
            {doc.title}
          </RouterLink>
        ))}
        {overflow > 0 ? (
          <span className="pl-1 text-[10px] text-muted">+{overflow}</span>
        ) : null}
      </div>
    </>
  );
}

/** The date, marked twice over: filled when it's today, accent when the day
 * already has a note. */
function DayNumber({ day, isToday }: { day: CalendarDay; isToday: boolean }) {
  return (
    <span
      className={`flex size-5 items-center justify-center rounded-full text-[11px] ${
        isToday
          ? "bg-accent font-semibold text-white"
          : day.has_daily
            ? "font-medium text-accent"
            : "text-muted"
      }`}
    >
      {Number(day.date.slice(8, 10))}
    </span>
  );
}
