// Mermaid diagram renderer, loaded lazily (the package is enormous) only when
// a ```mermaid fence is on screen — see the lazy() import in Markdown.tsx.
// Theme is picked at render time from the .dark class and re-picked when the
// theme toggles, or when the page is about to be printed (the print stylesheet
// forces the light palette, and a dark diagram on white paper reads as a black
// slab). A parse error falls back to the raw code with a note; it can never
// crash the page.
import { useEffect, useId, useState } from "react";
import mermaid from "mermaid";
import { registerPrintHook } from "../lib/printing.ts";

/** One render at an explicit theme. mermaid.initialize is global state, so the
 *  theme has to be set immediately before each render rather than once. */
async function renderDiagram(
  code: string,
  id: string,
  dark: boolean,
): Promise<string> {
  mermaid.initialize({
    startOnLoad: false,
    theme: dark ? "dark" : "neutral",
    fontFamily: "Inter Variable, sans-serif",
  });
  // mermaid.render wants a DOM-id-safe unique id.
  const { svg } = await mermaid.render(id, code);
  return svg;
}

export default function MermaidDiagram({ code }: { code: string }) {
  const [svg, setSvg] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);
  const [darkTheme, setDarkTheme] = useState(() =>
    document.documentElement.classList.contains("dark"),
  );
  const id = useId().replace(/[^a-zA-Z0-9]/g, "");

  // Re-render the diagram when the app theme flips (class on <html>).
  useEffect(() => {
    const observer = new MutationObserver(() => {
      setDarkTheme(document.documentElement.classList.contains("dark"));
    });
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ["class"],
    });
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    let cancelled = false;
    setFailed(false);
    renderDiagram(code, `mermaid-${id}-${darkTheme ? "d" : "l"}`, darkTheme)
      .then((rendered) => {
        if (!cancelled) setSvg(rendered);
      })
      .catch(() => {
        if (!cancelled) setFailed(true);
      });
    return () => {
      cancelled = true;
    };
  }, [code, id, darkTheme]);

  // Print swaps to the light theme and back. This is a hook printPage() can
  // await rather than something keyed off a print media query, because a
  // render is asynchronous and the print dialog snapshots the page the moment
  // its listeners return — a CSS-only answer would print the stale diagram.
  useEffect(() => {
    if (failed) return;
    return registerPrintHook(async (phase) => {
      const dark = document.documentElement.classList.contains("dark");
      // A light-theme reader needs neither the swap nor the restore.
      if (!dark) return;
      setSvg(
        await renderDiagram(code, `mermaid-${id}-${phase}`, phase === "screen"),
      );
    });
  }, [code, id, failed]);

  if (failed) {
    return (
      <div className="my-3">
        <pre>
          <code>{code}</code>
        </pre>
        <p className="mt-1 text-[10px] text-muted">diagram failed to render</p>
      </div>
    );
  }
  if (!svg) {
    return (
      <div className="my-3 h-24 animate-pulse rounded border border-border bg-hover" />
    );
  }
  return (
    <div
      className="my-3 flex justify-center overflow-x-auto rounded border border-border bg-raised p-3 print:overflow-visible print:break-inside-avoid"
      // Mermaid's own SVG output for this document's code — not remote HTML.
      dangerouslySetInnerHTML={{ __html: svg }}
    />
  );
}
