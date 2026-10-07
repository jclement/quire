// Read mode's "I want to change this" gesture: a double-click with a mouse,
// a long press with a finger. Both land on the rendered note itself, so
// anything that already answers a click or a press — links, checkboxes,
// buttons, images (the lightbox) — keeps its own meaning.
import { useEffect, useRef, type PointerEvent, type MouseEvent } from "react";

/** How long a finger rests before it counts as a long press. */
export const LONG_PRESS_MS = 500;
/** A finger that drifts further than this is scrolling or selecting. */
const MOVE_TOLERANCE_PX = 10;

const INTERACTIVE =
  "a, button, input, textarea, select, label, summary, img, video, audio, [role='button']";

/** Whether the event landed on something with a click meaning of its own. */
export function isInteractiveTarget(target: EventTarget | null): boolean {
  return target instanceof Element && target.closest(INTERACTIVE) !== null;
}

/** Handlers to spread onto the element that should answer the gesture. */
export function useEditGesture(onEdit: () => void) {
  const press = useRef<{ timer: number; x: number; y: number } | null>(null);
  const cancel = () => {
    if (!press.current) return;
    window.clearTimeout(press.current.timer);
    press.current = null;
  };
  useEffect(() => cancel, []);

  return {
    onDoubleClick: (event: MouseEvent) => {
      if (isInteractiveTarget(event.target)) return;
      // The second click selected a word; it would not survive the switch.
      window.getSelection()?.removeAllRanges();
      onEdit();
    },
    onPointerDown: (event: PointerEvent) => {
      cancel();
      if (event.pointerType !== "touch" || !event.isPrimary) return;
      if (isInteractiveTarget(event.target)) return;
      press.current = {
        x: event.clientX,
        y: event.clientY,
        timer: window.setTimeout(() => {
          press.current = null;
          window.getSelection()?.removeAllRanges();
          onEdit();
        }, LONG_PRESS_MS),
      };
    },
    onPointerMove: (event: PointerEvent) => {
      const start = press.current;
      if (!start) return;
      if (
        Math.hypot(event.clientX - start.x, event.clientY - start.y) >
        MOVE_TOLERANCE_PX
      ) {
        cancel();
      }
    },
    // A scroll taking over the touch arrives as pointercancel.
    onPointerUp: cancel,
    onPointerCancel: cancel,
    onPointerLeave: cancel,
  };
}
