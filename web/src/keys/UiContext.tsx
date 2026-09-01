// Shared UI state for the keyboard framework and global overlays: which overlay
// is open, which list currently owns j/k navigation, the Escape stack, and
// extra single-key actions pages register (e.g. `e` on a document). Handlers
// live in refs so registering a list never re-renders the app; only overlay
// visibility is React state.
import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useRef,
  useState,
  type ReactNode,
  type RefObject,
} from "react";
import type { DocType } from "../api/types.ts";

export interface ListNavHandlers {
  move: (delta: number) => void;
  open: () => void;
  toggle?: () => void;
}

export type OverlayName = "palette" | "capture" | "keymap";

interface UiContextValue {
  overlays: Record<OverlayName, boolean>;
  setOverlay: (name: OverlayName, open: boolean) => void;
  /** Non-null while the "New <type>" dialog is open (palette or browse pages). */
  newDocType: DocType | null;
  setNewDocType: (type: DocType | null) => void;
  /** The list currently receiving j/k/Enter/x — set by useListNav. */
  listNavRef: RefObject<ListNavHandlers | null>;
  /** Escape pops the most recently pushed handler (one level out). */
  escapeStackRef: RefObject<(() => void)[]>;
  pushEscape: (handler: () => void) => () => void;
  /** Page-registered single keys, e.g. "e" → enter edit mode. */
  keyActionsRef: RefObject<Map<string, () => void>>;
  registerKey: (key: string, action: () => void) => () => void;
}

const UiContext = createContext<UiContextValue | null>(null);

export function UiProvider({ children }: { children: ReactNode }) {
  const [overlays, setOverlays] = useState<Record<OverlayName, boolean>>({
    palette: false,
    capture: false,
    keymap: false,
  });
  const [newDocType, setNewDocType] = useState<DocType | null>(null);
  const listNavRef = useRef<ListNavHandlers | null>(null);
  const escapeStackRef = useRef<(() => void)[]>([]);
  const keyActionsRef = useRef(new Map<string, () => void>());

  const setOverlay = useCallback((name: OverlayName, open: boolean) => {
    setOverlays((current) =>
      current[name] === open ? current : { ...current, [name]: open },
    );
  }, []);

  const pushEscape = useCallback((handler: () => void) => {
    escapeStackRef.current.push(handler);
    return () => {
      const stack = escapeStackRef.current;
      const at = stack.indexOf(handler);
      if (at !== -1) stack.splice(at, 1);
    };
  }, []);

  const registerKey = useCallback((key: string, action: () => void) => {
    keyActionsRef.current.set(key, action);
    return () => {
      if (keyActionsRef.current.get(key) === action) {
        keyActionsRef.current.delete(key);
      }
    };
  }, []);

  const value = useMemo(
    () => ({
      overlays,
      setOverlay,
      newDocType,
      setNewDocType,
      listNavRef,
      escapeStackRef,
      pushEscape,
      keyActionsRef,
      registerKey,
    }),
    [overlays, setOverlay, newDocType, pushEscape, registerKey],
  );

  return <UiContext.Provider value={value}>{children}</UiContext.Provider>;
}

export function useUi(): UiContextValue {
  const context = useContext(UiContext);
  if (!context) throw new Error("useUi must be used inside <UiProvider>");
  return context;
}
