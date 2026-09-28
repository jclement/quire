// Fullscreen viewer for a rendered image or mermaid diagram. Rendered through
// a portal so no ancestor's overflow or transform can clip it, and it handles
// its own Escape rather than joining the UiContext Escape stack: Markdown also
// renders on the public share page, which has no UiProvider above it.
import { X } from "lucide-react";
import { useEffect, type ReactNode } from "react";
import { createPortal } from "react-dom";

interface LightboxProps {
  label: string;
  onClose: () => void;
  children: ReactNode;
}

export function Lightbox({ label, onClose, children }: LightboxProps) {
  useEffect(() => {
    const previous = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    // Capture phase, so the Escape closes the viewer and nothing else — not
    // edit mode, not the page's own Escape stack.
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      event.preventDefault();
      event.stopImmediatePropagation();
      onClose();
    };
    window.addEventListener("keydown", onKeyDown, true);
    return () => {
      document.body.style.overflow = previous;
      window.removeEventListener("keydown", onKeyDown, true);
    };
  }, [onClose]);

  return createPortal(
    <div
      role="dialog"
      aria-modal="true"
      aria-label={label}
      // Any click closes: there is nothing inside to interact with. Stopped
      // here because React bubbles portal events to the opener's ancestors.
      onClick={(event) => {
        event.stopPropagation();
        onClose();
      }}
      className="fixed inset-0 z-[60] flex cursor-zoom-out items-center justify-center bg-black/80 p-4 pt-12 print:hidden md:p-10 md:pt-14"
    >
      <button
        type="button"
        onClick={onClose}
        aria-label="Close"
        autoFocus
        className="absolute top-3 right-3 flex size-8 items-center justify-center rounded border border-white/20 bg-black/40 text-white/80 hover:text-white"
      >
        <X className="size-4" aria-hidden="true" />
      </button>
      {children}
    </div>,
    document.body,
  );
}
