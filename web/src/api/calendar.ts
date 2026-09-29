// The calendar feed and meeting prep: API calls and their TanStack Query
// hooks, kept together so the feature reads as one unit. Feed URLs go to
// the server once and come back masked; nothing here ever holds one for
// longer than the add form does.
import {
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query";
import { jsonInit, request } from "./client.ts";
import { queryKeys } from "./queries.ts";
import type {
  CalendarEvent,
  CalendarFeed,
  Document,
  MeetingPrep,
} from "./types.ts";

export const calendarKeys = {
  feeds: ["calendar-feeds"] as const,
  prep: (target: PrepTarget) =>
    [
      "meeting-prep",
      "path" in target ? target.path : `${target.uid}@${target.date}`,
    ] as const,
};

/** A meeting note by path, or a calendar occurrence by uid and date. */
export type PrepTarget = { path: string } | { uid: string; date: string };

export const calendarApi = {
  feeds: () => request<CalendarFeed[]>("/api/v1/calendar/feeds"),
  /** Subscribes and fetches at once; the list says whether it worked. */
  addFeed: (url: string) =>
    request<CalendarFeed[]>(
      "/api/v1/calendar/feeds",
      jsonInit("POST", { url }),
    ),
  removeFeed: (id: string) =>
    request<void>(`/api/v1/calendar/feeds/${encodeURIComponent(id)}`, {
      method: "DELETE",
    }),
  refresh: () =>
    request<CalendarFeed[]>("/api/v1/calendar/refresh", { method: "POST" }),
  events: (from: string, to: string) =>
    request<CalendarEvent[]>(
      `/api/v1/calendar/events?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`,
    ),
  /** 201 with a new note, or 200 with the occurrence's existing one. */
  noteFromEvent: (uid: string, date: string) =>
    request<Document>(
      "/api/v1/calendar/events/note",
      jsonInit("POST", { uid, date }),
    ),
  meetingPrep: (target: PrepTarget) =>
    request<MeetingPrep>(
      "path" in target
        ? `/api/v1/meeting-prep?path=${encodeURIComponent(target.path)}`
        : `/api/v1/meeting-prep?event_uid=${encodeURIComponent(target.uid)}&date=${encodeURIComponent(target.date)}`,
    ),
};

/** Polls while a just-added feed's first fetch is still running (adding
 * answers after a few seconds rather than waiting out a slow feed). */
export function useCalendarFeeds() {
  const queryClient = useQueryClient();
  return useQuery({
    queryKey: calendarKeys.feeds,
    queryFn: async () => {
      const previous = queryClient.getQueryData<CalendarFeed[]>(
        calendarKeys.feeds,
      );
      const feeds = await calendarApi.feeds();
      // A first fetch just landed: Today and the month have new events.
      if (previous?.some((f) => f.fetching) && !feeds.some((f) => f.fetching)) {
        invalidateCalendar(queryClient);
      }
      return feeds;
    },
    refetchInterval: (query) =>
      query.state.data?.some((feed) => feed.fetching) ? 1_000 : false,
  });
}

/** Feed list changes re-render Today and the month dots too. */
function invalidateCalendar(queryClient: QueryClient): void {
  void queryClient.invalidateQueries({ queryKey: queryKeys.today });
  void queryClient.invalidateQueries({ queryKey: ["calendar"] });
}

/** Add, remove and refresh share one shape: the new feed list comes back. */
export function useFeedMutation<Input>(
  fn: (input: Input) => Promise<CalendarFeed[] | void>,
) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: (feeds) => {
      if (feeds) queryClient.setQueryData(calendarKeys.feeds, feeds);
      else void queryClient.invalidateQueries({ queryKey: calendarKeys.feeds });
      invalidateCalendar(queryClient);
    },
  });
}

export function useMeetingPrep(target: PrepTarget | null) {
  return useQuery({
    queryKey: calendarKeys.prep(target ?? { path: "" }),
    queryFn: () => calendarApi.meetingPrep(target!),
    enabled: target !== null,
    staleTime: 30_000,
  });
}

/** "Create meeting note" on an agenda event; resolves to the note. */
export function useNoteFromEvent() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (event: CalendarEvent) =>
      calendarApi.noteFromEvent(event.uid, event.date),
    onSuccess: (doc) => {
      queryClient.setQueryData(queryKeys.document(doc.path), doc);
      void queryClient.invalidateQueries({ queryKey: queryKeys.today });
      void queryClient.invalidateQueries({ queryKey: ["documents"] });
    },
  });
}
