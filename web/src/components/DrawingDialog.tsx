// The Excalidraw editor, full-screen, over whatever page asked for it. The
// package is a large chunk, so it is imported only when a drawing is first
// opened; its fonts are self-hosted under /excalidraw/ (vite.config.ts).
// Saving exports an SVG render and the scene JSON and PUTs both; the note
// keeps embedding the render, which the <img> then re-fetches.
import type { ExcalidrawElement } from "@excalidraw/excalidraw/element/types";
import type { ExcalidrawImperativeAPI } from "@excalidraw/excalidraw/types";
import { Loader2 } from "lucide-react";
import { useEffect, useRef, useState, type ComponentType } from "react";
import { api } from "../api/client.ts";
import { bumpDrawing, onDrawingEditRequest } from "../lib/drawings.ts";
import { useUi } from "../keys/UiContext.tsx";

type ExcalidrawModule = typeof import("@excalidraw/excalidraw");

let modulePromise: Promise<ExcalidrawModule> | null = null;
function loadExcalidraw(): Promise<ExcalidrawModule> {
  if (!modulePromise) {
    window.EXCALIDRAW_ASSET_PATH = "/excalidraw/";
    modulePromise = Promise.all([
      import("@excalidraw/excalidraw"),
      import("@excalidraw/excalidraw/index.css"),
    ]).then(([m]) => m);
  }
  return modulePromise;
}

interface Scene {
  elements?: unknown[];
  appState?: { viewBackgroundColor?: string };
  files?: Record<string, unknown>;
}

/** What a drawing with nothing in it renders as — the same picture the server
 * writes for a fresh one, so an emptied drawing doesn't become a blank box. */
const EMPTY_RENDER =
  `<svg xmlns="http://www.w3.org/2000/svg" width="320" height="120" viewBox="0 0 320 120">` +
  `<rect width="320" height="120" rx="6" fill="#f8fafc" stroke="#cbd5e1" stroke-dasharray="6 4"/>` +
  `<text x="160" y="66" text-anchor="middle" font-family="sans-serif" font-size="14" fill="#64748b">Empty drawing</text></svg>`;

export function DrawingDialog() {
  const { toast } = useUi();
  const [path, setPath] = useState<string | null>(null);
  const [scene, setScene] = useState<Scene | null>(null);
  const [mod, setMod] = useState<ExcalidrawModule | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [confirmDiscard, setConfirmDiscard] = useState(false);
  const apiRef = useRef<ExcalidrawImperativeAPI | null>(null);
  // Excalidraw's onChange fires for viewport and selection changes too, so
  // "dirty" is judged by scene version (a sum of element versions) against
  // the version seen on the first change after opening.
  const baseVersionRef = useRef<number | null>(null);

  useEffect(
    () =>
      onDrawingEditRequest((scenePath) => {
        setPath(scenePath);
        setScene(null);
        setError(null);
        setDirty(false);
        setConfirmDiscard(false);
        baseVersionRef.current = null;
        void Promise.all([
          loadExcalidraw(),
          fetch(`/api/v1/files/${scenePath}`, { cache: "no-store" }).then(
            async (res) => {
              if (!res.ok) throw new Error(`drawing not found (${res.status})`);
              return (await res.json()) as Scene;
            },
          ),
        ])
          .then(([loaded, data]) => {
            setMod(loaded);
            setScene(data);
          })
          .catch((err: unknown) => {
            setError(
              err instanceof Error ? err.message : "Couldn't open the drawing",
            );
          });
      }),
    [],
  );

  // Lock page scroll while open, like Modal does.
  useEffect(() => {
    if (!path) return;
    const previous = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = previous;
    };
  }, [path]);

  const close = () => {
    setPath(null);
    setScene(null);
    apiRef.current = null;
  };

  const cancel = () => {
    if (dirty && !confirmDiscard) {
      setConfirmDiscard(true);
      return;
    }
    close();
  };

  const save = async () => {
    const excalidraw = apiRef.current;
    if (!path || !mod || !excalidraw) return;
    setSaving(true);
    try {
      const elements = excalidraw.getSceneElements();
      const appState = excalidraw.getAppState();
      const files = excalidraw.getFiles();
      let svg = EMPTY_RENDER;
      if (elements.length > 0) {
        const node = await mod.exportToSvg({
          elements,
          appState: {
            ...appState,
            exportBackground: true,
            exportWithDarkMode: false,
            exportEmbedScene: false,
          },
          files,
          exportPadding: 16,
        });
        svg = new XMLSerializer().serializeToString(node);
      }
      const sceneJSON = JSON.parse(
        mod.serializeAsJSON(elements, appState, files, "local"),
      ) as Record<string, unknown>;
      await api.saveDrawing(path, sceneJSON, svg);
      bumpDrawing(path);
      toast("Drawing saved");
      close();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Save failed");
    } finally {
      setSaving(false);
    }
  };

  if (!path) return null;
  const dark = document.documentElement.classList.contains("dark");
  const loaded = mod;
  const Excalidraw = mod?.Excalidraw as
    ComponentType<Record<string, unknown>> | undefined;

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label="Drawing"
      className="fixed inset-0 z-50 flex flex-col bg-surface print:hidden"
      // Excalidraw's own shortcuts (Escape deselects, `r` is rectangle…) must
      // not reach the app's global keys or the page's Escape stack.
      onKeyDown={(event) => event.stopPropagation()}
    >
      <div className="flex h-10 shrink-0 items-center gap-2 border-b border-border px-3">
        <span className="text-sm font-semibold text-heading">Drawing</span>
        <span className="hidden truncate font-mono text-xs text-muted sm:inline">
          {path}
        </span>
        {error ? (
          <span className="truncate text-xs text-danger">{error}</span>
        ) : null}
        <div className="ml-auto flex items-center gap-2">
          {confirmDiscard ? (
            <span className="text-xs text-muted">Discard changes?</span>
          ) : null}
          <button
            type="button"
            onClick={cancel}
            className={`flex h-8 items-center rounded border border-border px-2.5 text-xs hover:bg-hover ${
              confirmDiscard ? "text-danger" : "text-body"
            }`}
          >
            {confirmDiscard ? "Discard" : "Cancel"}
          </button>
          <button
            type="button"
            onClick={() => void save()}
            disabled={saving || !scene}
            className="flex h-8 items-center gap-1 rounded border border-border bg-accent px-2.5 text-xs font-medium text-white hover:opacity-90 disabled:opacity-50"
          >
            {saving ? (
              <Loader2 className="size-3.5 animate-spin" aria-hidden="true" />
            ) : null}
            Save drawing
          </button>
        </div>
      </div>
      <div className="min-h-0 flex-1">
        {Excalidraw && scene && loaded ? (
          <Excalidraw
            excalidrawAPI={(instance: ExcalidrawImperativeAPI) => {
              apiRef.current = instance;
            }}
            initialData={{
              elements: scene.elements ?? [],
              appState: {
                viewBackgroundColor:
                  scene.appState?.viewBackgroundColor ?? "#ffffff",
              },
              files: scene.files ?? {},
              scrollToContent: true,
            }}
            theme={dark ? "dark" : "light"}
            onChange={(elements: readonly ExcalidrawElement[]) => {
              const version = loaded.getSceneVersion(elements);
              if (baseVersionRef.current === null)
                baseVersionRef.current = version;
              setDirty(version !== baseVersionRef.current);
            }}
            UIOptions={{
              canvasActions: {
                loadScene: false,
                saveToActiveFile: false,
                export: false,
                toggleTheme: false,
              },
            }}
          />
        ) : error ? null : (
          <div className="flex h-full items-center justify-center gap-2 text-sm text-muted">
            <Loader2 className="size-4 animate-spin" aria-hidden="true" />
            Loading Excalidraw…
          </div>
        )}
      </div>
    </div>
  );
}
