// Shared UI state for the keyboard framework and global overlays: which overlay
// is open, which list currently owns j/k navigation, the Escape stack, and
// extra single-key actions pages register (e.g. `e` on a document). Handlers
// live in refs so registering a list never re-renders the app; only overlay
// visibility is React state.
import { loadArea, storeArea } from "../lib/area.ts";
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
  /** `s`: open the snooze popover for the selected task. */
  snooze?: () => void;
}

export type OverlayName = "palette" | "capture" | "keymap" | "markdownHelp";

export interface Toast {
  id: number;
  message: string;
}

interface UiContextValue {
  overlays: Record<OverlayName, boolean>;
  setOverlay: (name: OverlayName, open: boolean) => void;
  /** Non-null while the "New <type>" dialog is open (palette or browse pages). */
  newDocType: DocType | null;
  setNewDocType: (type: DocType | null) => void;
  /** Non-null while the share dialog is open, holding the doc's vault path. */
  shareDocPath: string | null;
  setShareDocPath: (path: string | null) => void;
  /** Non-null while the rename dialog is open, holding the doc's vault path. */
  renameDocPath: string | null;
  setRenameDocPath: (path: string | null) => void;
  /** Non-null while the delete confirmation is open. */
  deleteDocPath: string | null;
  setDeleteDocPath: (path: string | null) => void;
  /** Transient confirmations ("Link copied"); auto-dismissed. */
  toasts: Toast[];
  toast: (message: string) => void;

  /** The area switcher's value: "" (all), "none" (unclassified), or a name. */
  area: string;
  setArea: (area: string) => void;
  /** The list currently receiving j/k/Enter/x — set by useListNav. */
  listNavRef: RefObject<ListNavHandlers | null>;
  /** Escape pops the most recently pushed handler (one level out). */
  escapeStackRef: RefObject<(() => void)[]>;
  pushEscape: (handler: () => void) => () => void;
  /** Page-registered single keys, e.g. "e" → enter edit mode. */
  keyActionsRef: RefObject<Map<string, () => void>>;
  registerKey: (key: string, action: () => void) => () => void;
}

/** How long a toast stays up. */
const TOAST_MS = 3_000;

const UiContext = createContext<UiContextValue | null>(null);

export function UiProvider({ children }: { children: ReactNode }) {
  const [overlays, setOverlays] = useState<Record<OverlayName, boolean>>({
    palette: false,
    capture: false,
    keymap: false,
    markdownHelp: false,
  });
  const [newDocType, setNewDocType] = useState<DocType | null>(null);
  const [shareDocPath, setShareDocPath] = useState<string | null>(null);
  const [renameDocPath, setRenameDocPath] = useState<string | null>(null);
  const [deleteDocPath, setDeleteDocPath] = useState<string | null>(null);
  const [toasts, setToasts] = useState<Toast[]>([]);
  const [area, setAreaState] = useState<string>(loadArea);
  const setArea = useCallback((next: string) => {
    storeArea(next);
    setAreaState(next);
  }, []);
  const nextToastId = useRef(0);
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

  const toast = useCallback((message: string) => {
    nextToastId.current += 1;
    const id = nextToastId.current;
    setToasts((current) => [...current, { id, message }]);
    setTimeout(() => {
      setToasts((current) => current.filter((entry) => entry.id !== id));
    }, TOAST_MS);
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
      shareDocPath,
      setShareDocPath,
      renameDocPath,
      setRenameDocPath,
      deleteDocPath,
      setDeleteDocPath,
      toasts,
      toast,
      area,
      setArea,
      listNavRef,
      escapeStackRef,
      pushEscape,
      keyActionsRef,
      registerKey,
    }),
    [
      overlays,
      setOverlay,
      newDocType,
      shareDocPath,
      renameDocPath,
      deleteDocPath,
      toasts,
      toast,
      area,
      setArea,
      pushEscape,
      registerKey,
    ],
  );

  return <UiContext.Provider value={value}>{children}</UiContext.Provider>;
}

export function useUi(): UiContextValue {
  const context = useContext(UiContext);
  if (!context) throw new Error("useUi must be used inside <UiProvider>");
  return context;
}
