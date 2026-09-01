// Shared overlay chrome for the palette, quick capture, dialogs, and the keymap
// sheet. Handles backdrop click + Escape, scroll locking, and the house
// responsive behavior: full-screen/bottom-sheet on mobile, centered (or
// top-anchored) panel on desktop.
import { useEffect, type ReactNode } from "react";

export type ModalVariant = "center" | "sheet" | "palette";

interface ModalProps {
  open: boolean;
  onClose: () => void;
  variant: ModalVariant;
  label: string;
  children: ReactNode;
}

/** Panel geometry per variant: mobile-first, desktop refinements behind md:. */
const PANEL_CLASSES: Record<ModalVariant, string> = {
  center:
    "absolute inset-x-0 bottom-0 rounded-t-lg pb-[env(safe-area-inset-bottom)] " +
    "md:static md:mx-auto md:mt-[20vh] md:w-full md:max-w-md md:rounded-lg md:pb-0",
  sheet:
    "absolute inset-x-0 bottom-0 rounded-t-lg pb-[env(safe-area-inset-bottom)] " +
    "md:static md:mx-auto md:mt-[20vh] md:w-full md:max-w-md md:rounded-lg md:pb-0",
  palette:
    "absolute inset-0 flex flex-col " +
    "md:static md:mx-auto md:mt-[12vh] md:h-auto md:max-h-[60vh] md:w-full md:max-w-xl md:rounded-lg",
};

export function Modal({ open, onClose, variant, label, children }: ModalProps) {
  // Lock page scroll while any overlay is up.
  useEffect(() => {
    if (!open) return;
    const previous = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = previous;
    };
  }, [open]);

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50"
      onKeyDown={(event) => {
        if (event.key === "Escape") {
          event.stopPropagation();
          onClose();
        }
      }}
    >
      <div
        className="absolute inset-0 bg-black/40"
        aria-hidden="true"
        onClick={onClose}
      />
      <div
        role="dialog"
        aria-modal="true"
        aria-label={label}
        className={`border border-border bg-raised shadow-xl ${PANEL_CLASSES[variant]}`}
      >
        {children}
      </div>
    </div>
  );
}
