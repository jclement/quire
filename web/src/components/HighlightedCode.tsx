// Syntax highlighting for fenced code in the read view. highlight.js core with
// a hand-picked common-language set, loaded lazily (see Markdown.tsx) so it
// stays out of the critical bundle. Token colors are CSS-variable driven
// (.hljs-* rules in index.css), so dark mode just works. hljs escapes its
// output, so the innerHTML below contains only its own span markup.
import { useMemo } from "react";
import hljs from "highlight.js/lib/core";
import bash from "highlight.js/lib/languages/bash";
import css from "highlight.js/lib/languages/css";
import diff from "highlight.js/lib/languages/diff";
import go from "highlight.js/lib/languages/go";
import ini from "highlight.js/lib/languages/ini";
import javascript from "highlight.js/lib/languages/javascript";
import json from "highlight.js/lib/languages/json";
import markdown from "highlight.js/lib/languages/markdown";
import python from "highlight.js/lib/languages/python";
import rust from "highlight.js/lib/languages/rust";
import sql from "highlight.js/lib/languages/sql";
import typescript from "highlight.js/lib/languages/typescript";
import xml from "highlight.js/lib/languages/xml";
import yaml from "highlight.js/lib/languages/yaml";

hljs.registerLanguage("bash", bash);
hljs.registerAliases(["sh", "shell", "zsh"], { languageName: "bash" });
hljs.registerLanguage("css", css);
hljs.registerLanguage("diff", diff);
hljs.registerLanguage("go", go);
hljs.registerLanguage("ini", ini);
hljs.registerAliases(["toml"], { languageName: "ini" });
hljs.registerLanguage("javascript", javascript);
hljs.registerAliases(["js", "jsx"], { languageName: "javascript" });
hljs.registerLanguage("json", json);
hljs.registerLanguage("markdown", markdown);
hljs.registerAliases(["md"], { languageName: "markdown" });
hljs.registerLanguage("python", python);
hljs.registerAliases(["py"], { languageName: "python" });
hljs.registerLanguage("rust", rust);
hljs.registerLanguage("sql", sql);
hljs.registerLanguage("typescript", typescript);
hljs.registerAliases(["ts", "tsx"], { languageName: "typescript" });
hljs.registerLanguage("xml", xml);
hljs.registerAliases(["html", "svg"], { languageName: "xml" });
hljs.registerLanguage("yaml", yaml);
hljs.registerAliases(["yml"], { languageName: "yaml" });

export default function HighlightedCode({
  code,
  language,
}: {
  code: string;
  language: string;
}) {
  const html = useMemo(() => {
    if (!hljs.getLanguage(language)) return null;
    try {
      return hljs.highlight(code, { language }).value;
    } catch {
      return null;
    }
  }, [code, language]);

  if (html === null) return <code>{code}</code>;
  return <code dangerouslySetInnerHTML={{ __html: html }} />;
}
