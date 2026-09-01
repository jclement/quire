// Roving list selection for j/k + Enter + x. A view calls useListNav with its
// items and becomes "the active list": GlobalKeys routes movement keys here.
// Selection is an index with a focused row element (visible focus ring, screen
// readers follow along); the hook never steals focus until the user actually
// presses a movement key.
import { useCallback, useEffect, useRef, useState } from "react";
import { useUi } from "./UiContext.tsx";

export interface UseListNavOptions<T> {
  items: T[];
  onOpen: (item: T) => void;
  onToggle?: (item: T) => void;
  /** Set false while another list on the same page should own the keys. */
  enabled?: boolean;
}

export interface ListNav {
  index: number;
  setIndex: (index: number) => void;
  /** Attach to each row: ref={nav.rowRef(i)} tabIndex={-1}. */
  rowRef: (index: number) => (el: HTMLElement | null) => void;
}

export function useListNav<T>({
  items,
  onOpen,
  onToggle,
  enabled = true,
}: UseListNavOptions<T>): ListNav {
  const { listNavRef } = useUi();
  // Selection is clamped at read time rather than synced by effect, so a
  // shrinking list never leaves a dangling index.
  const [rawIndex, setRawIndex] = useState(0);
  const index = Math.min(rawIndex, Math.max(0, items.length - 1));
  const rowsRef = useRef(new Map<number, HTMLElement>());
  // Latest values live in a ref (written post-render) so the handlers
  // registered with the key context never go stale.
  const stateRef = useRef({ items, index, onOpen, onToggle });
  useEffect(() => {
    stateRef.current = { items, index, onOpen, onToggle };
  });

  const move = useCallback((delta: number) => {
    const { items: current, index: at } = stateRef.current;
    if (current.length === 0) return;
    const next = Math.min(current.length - 1, Math.max(0, at + delta));
    setRawIndex(next);
    const row = rowsRef.current.get(next);
    row?.focus({ preventScroll: true });
    row?.scrollIntoView({ block: "nearest" });
  }, []);

  const hasToggle = onToggle !== undefined;
  useEffect(() => {
    if (!enabled) return;
    const handlers = {
      move,
      open: () => {
        const { items: current, index: at, onOpen: open } = stateRef.current;
        const item = current[at];
        if (item !== undefined) open(item);
      },
      toggle: hasToggle
        ? () => {
            const {
              items: current,
              index: at,
              onToggle: toggle,
            } = stateRef.current;
            const item = current[at];
            if (item !== undefined) toggle?.(item);
          }
        : undefined,
    };
    listNavRef.current = handlers;
    return () => {
      if (listNavRef.current === handlers) listNavRef.current = null;
    };
  }, [enabled, move, hasToggle, listNavRef]);

  const rowRef = useCallback(
    (rowIndex: number) => (el: HTMLElement | null) => {
      if (el) rowsRef.current.set(rowIndex, el);
      else rowsRef.current.delete(rowIndex);
    },
    [],
  );

  return { index, setIndex: setRawIndex, rowRef };
}
