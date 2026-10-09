// The connection behind useDocEvents: one EventSource on /api/v1/events,
// kept honest. A stream can die without an error — a tunnel stalls it, a
// laptop sleeps, a phone parks the tab — and the browser goes on reporting
// it open while nothing arrives, so an agent's edit never reaches the page.
// The server therefore pings every 15s, and a stream that has said nothing
// for three of those is dropped and reopened. Whatever happened in between
// was never announced and never will be (there is no replay), so every
// recovery ends in onResync.
import type { DocEvent } from "./types.ts";

/** Three missed pings (internal/api/events.go's heartbeatInterval). */
const SILENCE_MS = 45_000;
const INITIAL_RETRY_MS = 1_000;
const MAX_RETRY_MS = 30_000;
/** EventSource.CLOSED, spelled out so nothing here needs the global. */
const CLOSED = 2;

export interface DocEventStreamOptions {
  onEvent: (event: DocEvent) => void;
  /** Something may have been missed: re-read whatever is on screen. */
  onResync: () => void;
  silenceMs?: number;
  initialRetryMs?: number;
  maxRetryMs?: number;
  /** Tests substitute a fake; the app uses the browser's EventSource. */
  createSource?: () => EventSource;
}

/** Opens the stream and keeps it open. Returns the function that ends it. */
export function openDocEventStream({
  onEvent,
  onResync,
  silenceMs = SILENCE_MS,
  initialRetryMs = INITIAL_RETRY_MS,
  maxRetryMs = MAX_RETRY_MS,
  createSource = () => new EventSource("/api/v1/events"),
}: DocEventStreamOptions): () => void {
  let source: EventSource | null = null;
  let retryTimer: ReturnType<typeof setTimeout> | undefined;
  let watchdog: ReturnType<typeof setTimeout> | undefined;
  let retryMs = initialRetryMs;
  let disposed = false;
  // True from the moment a stream is lost until the next one has opened
  // and the page has re-read.
  let missed = false;
  let opened = false;
  let lastHeard = Date.now();

  const heard = () => {
    lastHeard = Date.now();
    clearTimeout(watchdog);
    watchdog = setTimeout(onSilence, silenceMs);
  };

  const onSilence = () => {
    if (disposed) return;
    // A stream that never even opened is not coming back by waiting — a
    // proxy is holding the response. Re-reading on every lap is then the
    // only refresh there is.
    if (!opened) onResync();
    missed = true;
    connect();
  };

  const connect = () => {
    if (disposed) return;
    clearTimeout(retryTimer);
    source?.close();
    opened = false;
    const current = createSource();
    source = current;
    heard();
    current.addEventListener("open", () => {
      retryMs = initialRetryMs;
      opened = true;
      heard();
      if (missed) {
        missed = false;
        onResync();
      }
    });
    current.addEventListener("ping", heard);
    current.addEventListener("doc", (event) => {
      heard();
      try {
        onEvent(JSON.parse((event as MessageEvent).data) as DocEvent);
      } catch {
        // Malformed event payload: ignore rather than crash the stream.
      }
    });
    current.addEventListener("error", () => {
      missed = true;
      // EventSource retries transient drops itself; only take over once the
      // browser has given up (readyState CLOSED, e.g. server fully down).
      if (current.readyState !== CLOSED) return;
      current.close();
      clearTimeout(watchdog);
      retryTimer = setTimeout(connect, retryMs);
      retryMs = Math.min(retryMs * 2, maxRetryMs);
    });
  };

  // Timers barely run in a background tab and not at all on a sleeping
  // machine, so the watchdog can be hours late. Coming back to the tab is
  // the moment it matters; check then instead of waiting for it.
  const onWake = () => {
    if (document.visibilityState === "hidden") return;
    if (Date.now() - lastHeard >= silenceMs) onSilence();
  };
  document.addEventListener("visibilitychange", onWake);
  window.addEventListener("online", onWake);

  connect();
  return () => {
    disposed = true;
    clearTimeout(retryTimer);
    clearTimeout(watchdog);
    document.removeEventListener("visibilitychange", onWake);
    window.removeEventListener("online", onWake);
    source?.close();
  };
}
