// Settings → Calendar: the ICS feeds Today's agenda is read from. A feed
// URL is a password to the whole calendar, so the form sends it once and
// the list only ever shows the masked form the server returns; the input
// is cleared as soon as the add succeeds.
import { CalendarClock, Plus, RefreshCw, Trash2 } from "lucide-react";
import { useState } from "react";
import { errorMessage } from "../../api/client.ts";
import {
  calendarApi,
  useCalendarFeeds,
  useFeedMutation,
} from "../../api/calendar.ts";
import type { CalendarFeed } from "../../api/types.ts";
import { formatRelativeTime } from "../../lib/dates.ts";
import { noAutofill } from "../../lib/noAutofill.ts";
import { useUi } from "../../keys/UiContext.tsx";
import { SkeletonRows } from "../Skeleton.tsx";
import { ConfirmButton } from "./ConfirmButton.tsx";

export function CalendarSettings() {
  const { toast } = useUi();
  const feeds = useCalendarFeeds();
  const [url, setUrl] = useState("");
  const add = useFeedMutation(calendarApi.addFeed);
  const remove = useFeedMutation(calendarApi.removeFeed);
  const refresh = useFeedMutation(() => calendarApi.refresh());
  const failure = add.error ?? remove.error ?? refresh.error;

  return (
    <section className="flex flex-col gap-2 border-t border-border pt-4">
      <h2 className="text-sm font-semibold text-heading">Calendar</h2>
      <p className="text-xs text-muted">
        Today's agenda comes from your calendar's secret ICS link (Fastmail:
        Settings → Calendars → Share → secret link). Anyone with the link can
        read the calendar, so it is stored privately and shown here masked.
        Feeds refresh every ten minutes.
      </p>

      {feeds.isPending ? (
        <SkeletonRows count={1} />
      ) : feeds.isError ? (
        <p className="text-xs text-danger">{errorMessage(feeds.error)}</p>
      ) : feeds.data.length === 0 ? (
        <p className="border-y border-border py-3 text-xs text-muted">
          No calendar feeds.
        </p>
      ) : (
        <ul className="divide-y divide-border border-y border-border">
          {feeds.data.map((feed) => (
            <FeedRow
              key={feed.id}
              feed={feed}
              onRemove={() =>
                remove.mutate(feed.id, {
                  onSuccess: () => toast("Calendar feed removed"),
                })
              }
            />
          ))}
        </ul>
      )}

      <form
        onSubmit={(event) => {
          event.preventDefault();
          if (!url.trim()) return;
          add.mutate(url.trim(), {
            onSuccess: () => {
              setUrl("");
              toast("Calendar feed added");
            },
          });
        }}
        className="flex flex-wrap items-center gap-2"
      >
        <input
          type="url"
          value={url}
          onChange={(event) => setUrl(event.target.value)}
          placeholder="https://… or webcal://…"
          aria-label="Calendar feed URL"
          {...noAutofill("calendar-feed-url")}
          className="field-bare h-8 min-w-0 flex-1 rounded border border-border bg-raised px-2 font-mono text-xs text-heading outline-none placeholder:text-muted focus:border-accent"
        />
        <button
          type="submit"
          disabled={add.isPending || !url.trim()}
          className="flex h-8 items-center gap-1.5 rounded border border-border bg-accent px-2.5 text-xs font-medium text-white hover:opacity-90 disabled:opacity-50"
        >
          <Plus className="size-3.5" aria-hidden="true" />
          {add.isPending ? "Adding…" : "Add feed"}
        </button>
        {feeds.data && feeds.data.length > 0 ? (
          <button
            type="button"
            onClick={() => refresh.mutate(undefined)}
            disabled={refresh.isPending}
            className="flex h-8 items-center gap-1.5 rounded border border-border px-2.5 text-xs text-body hover:bg-hover hover:text-heading disabled:opacity-50"
          >
            <RefreshCw
              className={`size-3.5 ${refresh.isPending ? "animate-spin" : ""}`}
              aria-hidden="true"
            />
            Refresh
          </button>
        ) : null}
      </form>
      {failure ? (
        <p className="text-xs text-danger">{errorMessage(failure)}</p>
      ) : null}
    </section>
  );
}

function FeedRow({
  feed,
  onRemove,
}: {
  feed: CalendarFeed;
  onRemove: () => void;
}) {
  return (
    <li className="flex min-h-9 flex-wrap items-center gap-x-2 gap-y-0.5 px-2 py-1">
      <CalendarClock
        className="size-3.5 shrink-0 text-muted"
        aria-hidden="true"
      />
      <span className="min-w-0 truncate font-mono text-xs text-body">
        {feed.url}
      </span>
      <span
        className={`ml-auto shrink-0 text-xs ${feed.error ? "text-danger" : "text-muted"}`}
        data-testid="calendar-feed-status"
      >
        <FeedStatus feed={feed} />
      </span>
      <ConfirmButton
        label={`Remove calendar feed ${feed.url}`}
        confirmLabel="Remove?"
        onConfirm={onRemove}
      >
        <Trash2 className="size-3.5" aria-hidden="true" />
      </ConfirmButton>
      {feed.error ? (
        <p className="basis-full pl-5 text-xs text-danger">{feed.error}</p>
      ) : null}
    </li>
  );
}

function FeedStatus({ feed }: { feed: CalendarFeed }) {
  if (feed.error) {
    return (
      <>
        failing ({feed.failures}×)
        {feed.last_success
          ? ` · last worked ${formatRelativeTime(feed.last_success)}`
          : ""}
      </>
    );
  }
  if (feed.fetching) return <>fetching…</>;
  if (!feed.last_success) return <>not fetched yet</>;
  return (
    <>
      {feed.events} events · fetched {formatRelativeTime(feed.last_success)}
    </>
  );
}
