// Shared overlay chrome for the palette, quick capture, dialogs, and the keymap
// sheet. Handles backdrop click + Escape, scroll locking, and the house
// responsive behavior: full-screen/bottom-sheet on mobile, centered (or
// top-anchored) panel on desktop.
import { useEffect, type ReactNode } from "react";

export type ModalVariant = "center" | "sheet" | "palette" | "help";

interface ModalProps {
  open: boolean;
  onClose: () => void;
  variant: ModalVariant;
  label: string;
  children: ReactNode;
}

/** Panel geometry per variant: mobile-first, desktop refinements behind md:.
 * Panels stay *positioned* at every size (md:relative, never md:static) so
 * they paint above the absolutely-positioned backdrop — a static panel would
 * sit underneath it and never receive clicks. */
const PANEL_CLASSES: Record<ModalVariant, string> = {
  center:
    "absolute inset-x-0 bottom-0 rounded-t-lg pb-[env(safe-area-inset-bottom)] " +
    "md:relative md:inset-auto md:mx-auto md:mt-[20vh] md:w-full md:max-w-md md:rounded-lg md:pb-0",
  sheet:
    "absolute inset-x-0 bottom-0 rounded-t-lg pb-[env(safe-area-inset-bottom)] " +
    "md:relative md:inset-auto md:mx-auto md:mt-[20vh] md:w-full md:max-w-md md:rounded-lg md:pb-0",
  palette:
    "absolute inset-0 flex flex-col " +
    "md:relative md:inset-auto md:mx-auto md:mt-[12vh] md:h-auto md:max-h-[60vh] md:w-full md:max-w-xl md:rounded-lg",
  // Reference content: a tall sheet on mobile, a wide two-column panel on
  // desktop where the syntax/meaning pairs can sit side by side.
  help:
    "absolute inset-x-0 bottom-0 max-h-[90vh] rounded-t-lg pb-[env(safe-area-inset-bottom)] " +
    "md:relative md:inset-auto md:mx-auto md:mt-[8vh] md:w-full md:max-w-3xl md:rounded-lg md:pb-0",
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
      // Overlays never print: an open palette or dialog would otherwise cover
      // the first page (the palette closes on pick, but not before the print
      // it just triggered starts preparing).
      className="fixed inset-0 z-50 print:hidden"
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
