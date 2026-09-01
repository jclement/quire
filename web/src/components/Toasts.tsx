// The toast stack: transient confirmations pushed via useUi().toast(). Fixed
// above the mobile bottom nav, bottom-left on desktop; purely informational,
// so pointer events pass through the container.
import { useUi } from "../keys/UiContext.tsx";

export function Toasts() {
  const { toasts } = useUi();
  if (toasts.length === 0) return null;
  return (
    <div
      aria-live="polite"
      className="pointer-events-none fixed inset-x-0 bottom-[calc(env(safe-area-inset-bottom)+4rem)] z-50 flex flex-col items-center gap-1.5 md:bottom-4 md:items-start md:px-4"
    >
      {toasts.map((toast) => (
        <p
          key={toast.id}
          className="rounded border border-border bg-raised px-3 py-1.5 text-xs text-heading shadow-lg"
        >
          {toast.message}
        </p>
      ))}
    </div>
  );
}
