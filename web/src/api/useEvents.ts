// The app's single SSE connection. Listens for "doc" events on /api/v1/events
// and invalidates the TanStack Query caches that could be showing stale data
// for that path. Reconnects silently with exponential backoff so a restarting
// (or not-yet-running) backend never surfaces errors in the UI.
import { useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import type { QueryClient } from "@tanstack/react-query";
import type { DocEvent } from "./types.ts";
import { queryKeys } from "./queries.ts";

const INITIAL_RETRY_MS = 1_000;
const MAX_RETRY_MS = 30_000;

function invalidateForDocEvent(
  queryClient: QueryClient,
  event: DocEvent,
): void {
  void queryClient.invalidateQueries({
    queryKey: queryKeys.document(event.path),
  });
  // Lists, search results, task views, and Today can all reference any doc;
  // invalidation is cheap (refetch only happens for mounted queries).
  void queryClient.invalidateQueries({ queryKey: ["documents"] });
  void queryClient.invalidateQueries({ queryKey: ["search"] });
  void queryClient.invalidateQueries({ queryKey: ["tasks"] });
  void queryClient.invalidateQueries({ queryKey: queryKeys.today });
}

/** Mount once (in App). Owns the EventSource for the whole app lifetime. */
export function useDocEvents(): void {
  const queryClient = useQueryClient();

  useEffect(() => {
    let source: EventSource | null = null;
    let retryTimer: ReturnType<typeof setTimeout> | undefined;
    let retryMs = INITIAL_RETRY_MS;
    let disposed = false;

    const connect = () => {
      if (disposed) return;
      source = new EventSource("/api/v1/events");
      source.addEventListener("open", () => {
        retryMs = INITIAL_RETRY_MS;
      });
      source.addEventListener("doc", (event) => {
        try {
          const parsed = JSON.parse((event as MessageEvent).data) as DocEvent;
          invalidateForDocEvent(queryClient, parsed);
        } catch {
          // Malformed event payload: ignore rather than crash the stream.
        }
      });
      source.addEventListener("error", () => {
        // EventSource retries transient drops itself; only take over once the
        // browser has given up (readyState CLOSED, e.g. server fully down).
        if (source?.readyState !== EventSource.CLOSED) return;
        source.close();
        retryTimer = setTimeout(connect, retryMs);
        retryMs = Math.min(retryMs * 2, MAX_RETRY_MS);
      });
    };

    connect();
    return () => {
      disposed = true;
      clearTimeout(retryTimer);
      source?.close();
    };
  }, [queryClient]);
}
