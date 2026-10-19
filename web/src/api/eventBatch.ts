// Coalesces doc events into one flush. Every event invalidates a dozen
// query families, and each mounted query refetches in full on the server
// (the client cancelling the superseded fetch does not stop that work), so
// an agent appending ten lines, or a git pull landing two hundred files,
// used to cost that many rounds of every query on the page. Now a burst
// costs one round: the flush runs once the stream has been quiet for
// delayMs, and at least every maxWaitMs while it keeps going, so a long
// burst still shows progress.

export interface EventBatcher {
  push(path: string): void;
  /** Drops anything pending (unmount). */
  cancel(): void;
}

export function batchDocEvents(
  flush: (paths: string[]) => void,
  {
    delayMs = 150,
    maxWaitMs = 1000,
  }: { delayMs?: number; maxWaitMs?: number } = {},
): EventBatcher {
  let pending = new Set<string>();
  let timer: ReturnType<typeof setTimeout> | undefined;
  let firstAt = 0;

  const run = () => {
    timer = undefined;
    const paths = [...pending];
    pending = new Set();
    if (paths.length > 0) flush(paths);
  };

  return {
    push(path) {
      if (pending.size === 0) firstAt = Date.now();
      pending.add(path);
      if (timer !== undefined) clearTimeout(timer);
      const waited = Date.now() - firstAt;
      timer = setTimeout(
        run,
        Math.max(0, Math.min(delayMs, maxWaitMs - waited)),
      );
    },
    cancel() {
      if (timer !== undefined) clearTimeout(timer);
      timer = undefined;
      pending = new Set();
    },
  };
}
