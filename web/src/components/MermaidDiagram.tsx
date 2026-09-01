// Mermaid diagram renderer, loaded lazily (the package is enormous) only when
// a ```mermaid fence is on screen — see the lazy() import in Markdown.tsx.
// Theme is picked at render time from the .dark class and re-picked when the
// theme toggles. A parse error falls back to the raw code with a note; it can
// never crash the page.
import { useEffect, useId, useState } from "react";
import mermaid from "mermaid";

export default function MermaidDiagram({ code }: { code: string }) {
  const [svg, setSvg] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);
  const [darkTheme, setDarkTheme] = useState(() =>
    document.documentElement.classList.contains("dark"),
  );
  // mermaid.render wants a DOM-id-safe unique id.
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
    try {
      mermaid.initialize({
        startOnLoad: false,
        theme: darkTheme ? "dark" : "neutral",
        fontFamily: "Inter Variable, sans-serif",
      });
      mermaid
        .render(`mermaid-${id}-${darkTheme ? "d" : "l"}`, code)
        .then(({ svg: rendered }) => {
          if (!cancelled) setSvg(rendered);
        })
        .catch(() => {
          if (!cancelled) setFailed(true);
        });
    } catch {
      setFailed(true);
    }
    return () => {
      cancelled = true;
    };
  }, [code, id, darkTheme]);

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
      className="my-3 flex justify-center overflow-x-auto rounded border border-border bg-raised p-3"
      // Mermaid's own SVG output for this document's code — not remote HTML.
      dangerouslySetInnerHTML={{ __html: svg }}
    />
  );
}
