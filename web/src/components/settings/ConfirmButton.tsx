// A destructive icon button that asks once before doing the thing.
//
// Revoking a token, disconnecting an app, deleting a passkey and killing a
// share link are all irreversible and all sit next to each other in a dense
// list, which is exactly where a mis-click happens. A full modal for each
// would be heavier than the action deserves, so the button turns into its
// own confirmation and reverts if you look away.
import { useEffect, useRef, useState } from "react";

const CONFIRM_TIMEOUT_MS = 4_000;

export function ConfirmButton({
  label,
  confirmLabel,
  onConfirm,
  children,
}: {
  /** Accessible name in the resting state, e.g. "Revoke token claude". */
  label: string;
  /** Short visible prompt in the armed state, e.g. "Revoke?". */
  confirmLabel: string;
  onConfirm: () => void;
  children: React.ReactNode;
}) {
  const [armed, setArmed] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Disarm on unmount so a revoked row can't fire a stray timer.
  useEffect(
    () => () => {
      if (timer.current) clearTimeout(timer.current);
    },
    [],
  );

  const arm = () => {
    setArmed(true);
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(() => setArmed(false), CONFIRM_TIMEOUT_MS);
  };

  if (armed) {
    return (
      <button
        type="button"
        autoFocus
        onClick={() => {
          if (timer.current) clearTimeout(timer.current);
          setArmed(false);
          onConfirm();
        }}
        onBlur={() => setArmed(false)}
        className="flex h-11 shrink-0 items-center rounded border border-danger px-2 text-[10px] font-medium uppercase tracking-wide text-danger hover:bg-danger hover:text-white md:h-7 md:px-1.5"
      >
        {confirmLabel}
      </button>
    );
  }
  return (
    <button
      type="button"
      onClick={arm}
      aria-label={label}
      className="flex size-11 shrink-0 items-center justify-center rounded border border-border text-muted hover:bg-hover hover:text-danger md:size-7"
    >
      {children}
    </button>
  );
}
