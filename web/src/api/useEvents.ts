// The app's single SSE connection. Listens for "doc" events on /api/v1/events
// and invalidates the TanStack Query caches that could be showing stale data
// for that path. The connection itself — reconnecting, and noticing a stream
// that has silently died — is docEvents.ts.
import { useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import type { QueryClient } from "@tanstack/react-query";
import type { DocEvent } from "./types.ts";
import { openDocEventStream } from "./docEvents.ts";
import { batchDocEvents } from "./eventBatch.ts";
import { queryKeys } from "./queries.ts";

function invalidateForDocEvents(
  queryClient: QueryClient,
  paths: string[],
): void {
  for (const path of paths) {
    void queryClient.invalidateQueries({ queryKey: queryKeys.document(path) });
  }
  // Lists, search results, task views, Today, and the month calendar can all
  // reference any doc; invalidation is cheap (refetch only happens for mounted
  // queries).
  void queryClient.invalidateQueries({ queryKey: ["documents"] });
  void queryClient.invalidateQueries({ queryKey: ["search"] });
  void queryClient.invalidateQueries({ queryKey: ["tasks"] });
  void queryClient.invalidateQueries({ queryKey: queryKeys.today });
  void queryClient.invalidateQueries({ queryKey: ["calendar"] });
  // The journal, the tag list, the decision log and the unwritten names are
  // derived from every document, so a change to any of them can change each. Missing these left the journal
  // stale after an external edit — caught by its own E2E test.
  void queryClient.invalidateQueries({ queryKey: ["journal"] });
  void queryClient.invalidateQueries({ queryKey: ["tags"] });
  void queryClient.invalidateQueries({ queryKey: ["decisions"] });
  void queryClient.invalidateQueries({ queryKey: ["unwritten"] });
  void queryClient.invalidateQueries({ queryKey: ["areas"] });
  void queryClient.invalidateQueries({ queryKey: ["templates"] });
  // Any write can change someone's last meeting or open tasks.
  void queryClient.invalidateQueries({ queryKey: ["meeting-prep"] });
}

/** Mount once (in App). Owns the event stream for the whole app lifetime. */
export function useDocEvents(): void {
  const queryClient = useQueryClient();

  useEffect(() => {
    // A burst of events (an agent's edits, a git pull) is one round of
    // refetches, not one per event — see eventBatch.ts.
    const batch = batchDocEvents((paths) =>
      invalidateForDocEvents(queryClient, paths),
    );
    const stop = openDocEventStream({
      onEvent: (event: DocEvent) => batch.push(event.path),
      onResync: () => {
        batch.cancel();
        void queryClient.invalidateQueries();
      },
    });
    return () => {
      stop();
      batch.cancel();
    };
  }, [queryClient]);
}
