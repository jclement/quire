// Today's agenda: the day's events from the subscribed calendar feeds,
// all-day ones on a line of their own above the timed list. Each timed
// event offers its meeting note — "Create meeting note" makes it from the
// meeting template (attendees matched to person pages) and opens it; once
// it exists the button is "Open note" — and "Prep", the per-attendee
// context in a dialog.
//
// Times are shown as the server wrote them: RFC 3339 in the app's time
// zone, so the clock on the agenda is the one every other date in the app
// is reckoned in, not whatever zone this browser happens to be in.
import { Link as RouterLink, useNavigate } from "@tanstack/react-router";
import {
  ClipboardList,
  ExternalLink,
  FilePlus,
  FileText,
  X,
} from "lucide-react";
import { useState } from "react";
import { errorMessage } from "../api/client.ts";
import { useNoteFromEvent } from "../api/calendar.ts";
import type { CalendarEvent, EventAttendee } from "../api/types.ts";
import { docHref } from "../lib/docs.ts";
import { MeetingPrepView } from "./MeetingPrep.tsx";
import { Modal } from "./Modal.tsx";

/** Attendees named on a row before the rest become "+N". */
const ATTENDEES_SHOWN = 4;

export function Agenda({ events }: { events: CalendarEvent[] }) {
  const [prepFor, setPrepFor] = useState<CalendarEvent | null>(null);
  // Read once per mount: dimming past events is a glance aid, not a clock,
  // and coming back to Today re-mounts it with a fresh reading.
  const [now] = useState(() => Date.now());
  if (events.length === 0) return null;
  const allDay = events.filter((event) => event.all_day);
  const timed = events.filter((event) => !event.all_day);

  return (
    <section aria-label="Agenda">
      <h2 className="mb-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted">
        Agenda
      </h2>
      {allDay.length > 0 ? (
        <p className="mb-1.5 flex flex-wrap gap-1.5 text-xs text-muted">
          <span>All day:</span>
          {allDay.map((event) => (
            <span
              key={`${event.uid}-${event.recurrence_id}`}
              className="rounded border border-border px-1.5 text-body"
            >
              {event.title}
            </span>
          ))}
        </p>
      ) : null}
      {timed.length > 0 ? (
        <ul className="divide-y divide-border border-y border-border">
          {timed.map((event) => (
            <AgendaRow
              key={`${event.uid}-${event.recurrence_id}-${event.start}`}
              event={event}
              past={Date.parse(event.end) < now}
              onPrep={() => setPrepFor(event)}
            />
          ))}
        </ul>
      ) : null}
      <Modal
        open={prepFor !== null}
        onClose={() => setPrepFor(null)}
        variant="help"
        label={prepFor ? `Prep: ${prepFor.title}` : "Prep"}
      >
        {prepFor ? (
          <div className="flex max-h-[80vh] flex-col">
            <div className="flex items-baseline gap-2 border-b border-border px-3 py-2">
              <ClipboardList
                className="size-4 shrink-0 self-center text-muted"
                aria-hidden="true"
              />
              <h3 className="truncate text-sm font-semibold text-heading">
                {prepFor.title}
              </h3>
              <span className="ml-auto shrink-0 font-mono text-xs text-muted">
                {timeRange(prepFor)}
              </span>
              {/* Focused on open, so Escape (handled by Modal) reaches the
                  dialog rather than the page behind it. */}
              <button
                type="button"
                autoFocus
                onClick={() => setPrepFor(null)}
                aria-label="Close prep"
                className="flex size-7 shrink-0 items-center justify-center self-center rounded text-muted hover:bg-hover hover:text-heading"
              >
                <X className="size-4" aria-hidden="true" />
              </button>
            </div>
            <div className="overflow-y-auto px-3 py-3">
              <MeetingPrepView
                target={{ uid: prepFor.uid, date: prepFor.date }}
                variant="full"
              />
            </div>
          </div>
        ) : null}
      </Modal>
    </section>
  );
}

function AgendaRow({
  event,
  past,
  onPrep,
}: {
  event: CalendarEvent;
  past: boolean;
  onPrep: () => void;
}) {
  const navigate = useNavigate();
  const createNote = useNoteFromEvent();

  return (
    <li
      aria-label={event.title}
      className="flex flex-wrap items-start gap-x-3 gap-y-1 px-2 py-1.5"
    >
      <span
        className={`w-24 shrink-0 pt-px font-mono text-xs ${past ? "text-muted" : "text-heading"}`}
      >
        {timeRange(event)}
      </span>
      <div className="min-w-0 flex-1">
        <div
          className={`truncate text-sm font-medium ${past ? "text-muted" : "text-heading"}`}
        >
          {event.title}
        </div>
        <EventWhere event={event} />
        <Attendees attendees={event.attendees} />
        {createNote.isError ? (
          <p className="text-xs text-danger">
            Couldn't create the note — {errorMessage(createNote.error)}
          </p>
        ) : null}
      </div>
      {/* Own row on a phone, aligned under the title: beside it, two
          buttons leave the title a few characters. */}
      <div className="flex w-full shrink-0 items-center gap-1.5 pl-27 md:w-auto md:pl-0">
        <button
          type="button"
          onClick={onPrep}
          aria-label={`Prep for ${event.title}`}
          className="flex h-11 items-center gap-1 rounded border border-border px-2 text-xs text-body hover:bg-hover hover:text-heading md:h-7"
        >
          <ClipboardList className="size-3.5" aria-hidden="true" />
          Prep
        </button>
        {event.note_path ? (
          <RouterLink
            to={docHref(event.note_path)}
            className="flex h-11 items-center gap-1 rounded border border-border px-2 text-xs text-body hover:bg-hover hover:text-heading md:h-7"
          >
            <FileText className="size-3.5" aria-hidden="true" />
            Open note
          </RouterLink>
        ) : (
          <button
            type="button"
            disabled={createNote.isPending}
            onClick={() =>
              createNote.mutate(event, {
                onSuccess: (doc) => void navigate({ to: docHref(doc.path) }),
              })
            }
            className="flex h-11 items-center gap-1 rounded border border-border px-2 text-xs text-body hover:bg-hover hover:text-heading disabled:opacity-50 md:h-7"
          >
            <FilePlus className="size-3.5" aria-hidden="true" />
            {createNote.isPending ? "Creating…" : "Create meeting note"}
          </button>
        )}
      </div>
    </li>
  );
}

/** Location text, and the join link when there is one. */
function EventWhere({ event }: { event: CalendarEvent }) {
  const place = event.location && event.location !== event.link;
  if (!place && !event.link) return null;
  return (
    <div className="flex min-w-0 items-center gap-2 text-xs text-muted">
      {place ? <span className="truncate">{event.location}</span> : null}
      {event.link ? (
        <a
          href={event.link}
          target="_blank"
          rel="noreferrer noopener"
          className="flex shrink-0 items-center gap-0.5 text-accent hover:underline"
        >
          <ExternalLink className="size-3" aria-hidden="true" />
          Join
        </a>
      ) : null}
    </div>
  );
}

/** Invitees, people with a page as links; the organizer is left out when
 * others are listed, since it is usually the owner. */
function Attendees({ attendees }: { attendees: EventAttendee[] }) {
  const others = attendees.filter((a) => !a.organizer);
  const list = others.length > 0 ? others : attendees;
  if (list.length === 0) return null;
  const shown = list.slice(0, ATTENDEES_SHOWN);
  return (
    <div className="truncate text-xs text-muted">
      {shown.map((attendee, index) => (
        <span key={attendee.email || attendee.name}>
          {index > 0 ? ", " : ""}
          {attendee.person ? (
            <RouterLink
              to={docHref(attendee.person.path)}
              className="text-body hover:text-accent"
            >
              {attendee.person.title}
            </RouterLink>
          ) : (
            attendee.name || attendee.email
          )}
        </span>
      ))}
      {list.length > shown.length ? ` +${list.length - shown.length}` : ""}
    </div>
  );
}

/** "09:00–09:30" from the server's zone-local RFC 3339 strings. */
function timeRange(event: CalendarEvent): string {
  if (event.all_day) return "All day";
  const start = event.start.slice(11, 16);
  const end = event.end.slice(11, 16);
  return end && end !== start ? `${start}–${end}` : start;
}
