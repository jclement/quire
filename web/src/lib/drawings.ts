// Excalidraw drawings on the client side. A drawing is a `.excalidraw` scene
// next to a `.excalidraw.svg` render (see internal/service/drawings.go); the
// note embeds the render as a plain image and this module is how the app
// recognises one, keeps its <img> fresh after a save, and asks the single
// mounted DrawingDialog to open on it.
import { useSyncExternalStore } from "react";

export const DRAWING_RENDER_SUFFIX = ".excalidraw.svg";

/** Whether an image src is the render half of a drawing (vault-relative). */
export function isDrawingRender(src: string): boolean {
  return (
    src.toLowerCase().endsWith(DRAWING_RENDER_SUFFIX) &&
    !/^(https?:|data:|blob:|\/)/.test(src)
  );
}

/** The scene path for a render path. */
export function drawingSourceFor(render: string): string {
  return render.slice(0, -".svg".length);
}

// ---- render versions: bump after a save so the <img> re-fetches ----

const versions = new Map<string, number>();
const listeners = new Set<() => void>();

export function bumpDrawing(scenePath: string): void {
  versions.set(scenePath, (versions.get(scenePath) ?? 0) + 1);
  for (const l of listeners) l();
}

export function useDrawingVersion(scenePath: string): number {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    () => versions.get(scenePath) ?? 0,
  );
}

// ---- open-the-editor bridge ----

const EVENT = "quire:edit-drawing";

/** Opens the drawing editor on a scene path. */
export function requestDrawingEdit(scenePath: string): void {
  window.dispatchEvent(new CustomEvent<string>(EVENT, { detail: scenePath }));
}

export function onDrawingEditRequest(
  handler: (scenePath: string) => void,
): () => void {
  const listener = (event: Event) =>
    handler((event as CustomEvent<string>).detail);
  window.addEventListener(EVENT, listener);
  return () => window.removeEventListener(EVENT, listener);
}

declare global {
  interface Window {
    EXCALIDRAW_ASSET_PATH?: string;
  }
}
