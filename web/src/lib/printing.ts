// Print / PDF export plumbing. Printing is a real output for quire, not a
// browser accident, and two things have to happen before the page is
// snapshotted: a document has to be in read mode (printing CodeMirror is
// useless) and Mermaid diagrams have to be re-rendered in the light theme,
// because the print stylesheet forces the light palette and a dark diagram on
// white paper looks broken.
//
// Components register a hook; it is called with "print" before the dialog
// opens and "screen" after it closes. printPage() awaits the "print" pass
// first — that ordering is the whole point, since a mermaid render is async
// and the print dialog snapshots synchronously.
//
// A print started from the browser's own UI (File ▸ Print, iOS's share sheet)
// can only be caught by `beforeprint`, which cannot be awaited: synchronous
// hooks (the read-mode switch) still land in time, asynchronous ones (mermaid)
// do not. That is why ⌘P is intercepted in keys/GlobalKeys.tsx and routed
// through printPage() instead of being left to the browser.

export type PrintPhase = "print" | "screen";

/** Returning a promise makes printPage() wait for it. */
export type PrintHook = (phase: PrintPhase) => void | Promise<void>;

/** Ceiling on the whole preparation pass — a wedged hook delays the dialog by
 *  this much and no more. */
export const PREPARE_TIMEOUT_MS = 2_000;

const hooks = new Set<PrintHook>();
let prepared = false;

export function registerPrintHook(hook: PrintHook): () => void {
  hooks.add(hook);
  return () => {
    hooks.delete(hook);
  };
}

async function withTimeout(work: Promise<unknown>, timeoutMs: number) {
  let timer: ReturnType<typeof setTimeout> | undefined;
  await Promise.race([
    work,
    new Promise<void>((resolve) => {
      timer = setTimeout(resolve, timeoutMs);
    }),
  ]);
  clearTimeout(timer);
}

async function runHooks(phase: PrintPhase, timeoutMs: number): Promise<void> {
  const ran = new Set<PrintHook>();
  const drain = async () => {
    // Drained in passes rather than one batch: the read-mode hook mounts the
    // rendered view, and any diagram it mounts registers its own hook while
    // this pass is still running. Looping is what lets a print straight out of
    // edit mode still get light diagrams.
    for (;;) {
      const pending = [...hooks].filter((hook) => !ran.has(hook));
      if (pending.length === 0) return;
      for (const hook of pending) ran.add(hook);
      await Promise.all(
        pending.map(async (hook) => {
          try {
            await hook(phase);
          } catch {
            // One broken diagram must never stop the page from printing.
          }
        }),
      );
    }
  };
  await withTimeout(drain(), timeoutMs);
}

/**
 * Puts the page into its printable shape. Idempotent until the print ends, so
 * the `beforeprint` that our own window.print() fires is a no-op rather than a
 * second round of renders.
 */
export async function preparePrint(
  timeoutMs = PREPARE_TIMEOUT_MS,
): Promise<void> {
  if (prepared) return;
  prepared = true;
  await runHooks("print", timeoutMs);
}

/** Undoes preparePrint(). Driven by `afterprint`; nobody waits for it. */
export function restoreAfterPrint(timeoutMs = PREPARE_TIMEOUT_MS): void {
  if (!prepared) return;
  prepared = false;
  void runHooks("screen", timeoutMs);
}

/** The one way the app prints: prepare, then hand over to the browser. */
export async function printPage(): Promise<void> {
  await preparePrint();
  window.print();
}

/** Backstop for prints we did not start. Mounted once, from GlobalKeys. */
export function installPrintListeners(): () => void {
  const onBeforePrint = () => void preparePrint();
  const onAfterPrint = () => restoreAfterPrint();
  window.addEventListener("beforeprint", onBeforePrint);
  window.addEventListener("afterprint", onAfterPrint);
  return () => {
    window.removeEventListener("beforeprint", onBeforePrint);
    window.removeEventListener("afterprint", onAfterPrint);
  };
}
