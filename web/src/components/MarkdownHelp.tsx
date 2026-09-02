// The markdown cheatsheet overlay: quire's dialect in one screen — GFM plus
// wikilinks, tags, callouts, mermaid, and the task metadata emoji that the
// indexer reads. Reachable from the document header's "?" button and the
// palette's "Markdown help" command. Two columns on desktop, a scrollable
// sheet on mobile; Escape closes (Modal handles it).
import { useUi } from "../keys/UiContext.tsx";
import { Modal } from "./Modal.tsx";

interface HelpRow {
  syntax: string;
  meaning: string;
}

interface HelpSection {
  title: string;
  rows: HelpRow[];
}

const SECTIONS: HelpSection[] = [
  {
    title: "Text",
    rows: [
      { syntax: "# H1  ·  ## H2  ·  ### H3", meaning: "Headings (levels 1–6)" },
      { syntax: "**bold**  ·  *italic*", meaning: "Emphasis" },
      { syntax: "~~struck~~", meaning: "Strikethrough" },
      { syntax: "`inline code`", meaning: "Code, verbatim" },
      { syntax: "> quoted", meaning: "Blockquote" },
      { syntax: "---", meaning: "Horizontal rule" },
    ],
  },
  {
    title: "Lists",
    rows: [
      { syntax: "- item  ·  * item", meaning: "Bullet list" },
      { syntax: "1. item", meaning: "Numbered list" },
      { syntax: "  - nested", meaning: "Indent two spaces (or Tab) to nest" },
    ],
  },
  {
    title: "Tasks",
    rows: [
      { syntax: "- [ ] open task", meaning: "An unfinished task" },
      { syntax: "- [x] done", meaning: "Completed" },
      { syntax: "📅 2026-09-04", meaning: "Due date" },
      {
        syntax: "🛫 2026-09-01",
        meaning: "Defer until — hidden from Today before this",
      },
      { syntax: "⏫  ·  🔼  ·  🔽", meaning: "Priority: high · medium · low" },
      { syntax: "⏳", meaning: "Waiting on someone else" },
      { syntax: "🔁 every year", meaning: "Repeats on a schedule" },
      {
        syntax: "🔁 every 3 months when done",
        meaning: "Repeats after completion",
      },
      { syntax: "✅ 2026-09-01", meaning: "Completion date (added on toggle)" },
    ],
  },
  {
    title: "Links & references",
    rows: [
      { syntax: "[text](https://example.com)", meaning: "External link" },
      { syntax: "[[Sarah Chen]]", meaning: "Wikilink to another note" },
      { syntax: "[[people/sarah-chen|Sarah]]", meaning: "Wikilink with alias" },
      { syntax: "#project/apollo", meaning: "Tag (nests with slashes)" },
      {
        syntax: "![](attachments/x.png)",
        meaning: "Image — or just paste one",
      },
    ],
  },
  {
    title: "Blocks",
    rows: [
      {
        syntax: "| a | b |\n|---|---|\n| 1 | 2 |",
        meaning:
          "Table — ⌘⌥T reformats the one under the cursor, Tab moves between cells",
      },
      {
        syntax: "```go\ncode\n```",
        meaning: "Fenced code, highlighted by language",
      },
      {
        syntax: "```mermaid\ngraph LR\n  A --> B\n```",
        meaning: "Mermaid diagram",
      },
    ],
  },
  {
    title: "Callouts",
    rows: [
      { syntax: "> [!NOTE] Optional title\n> body", meaning: "Callout block" },
      {
        syntax: "note · info · tip · warning",
        meaning: "…and danger · question · success · example",
      },
    ],
  },
];

export function MarkdownHelp() {
  const { overlays, setOverlay } = useUi();
  return (
    <Modal
      open={overlays.markdownHelp}
      onClose={() => setOverlay("markdownHelp", false)}
      variant="help"
      label="Markdown reference"
    >
      <div className="max-h-[85vh] overflow-y-auto p-4 md:max-h-[75vh]">
        <h2 className="mb-1 text-sm font-semibold text-heading">
          Markdown in quire
        </h2>
        <p className="mb-4 text-xs text-muted">
          Standard markdown, plus wikilinks, tags, callouts, and the task
          metadata the indexer reads.
        </p>
        {/* A grid, not CSS columns: multicol fragments a section across the
            column break, which reads badly in a scrolling panel. */}
        <div className="grid items-start gap-x-8 md:grid-cols-2">
          {SECTIONS.map((section) => (
            <section key={section.title} className="mb-4">
              <h3 className="mb-1 text-[10px] font-semibold tracking-wider text-muted uppercase">
                {section.title}
              </h3>
              <dl className="border-t border-border">
                {section.rows.map((row) => (
                  <div
                    key={row.syntax}
                    className="flex flex-col gap-0.5 border-b border-border py-1.5 sm:flex-row sm:items-baseline sm:gap-3"
                  >
                    <dt className="shrink-0 font-mono text-[11px] whitespace-pre-line text-heading sm:w-1/2">
                      {row.syntax}
                    </dt>
                    <dd className="text-xs text-muted">{row.meaning}</dd>
                  </div>
                ))}
              </dl>
            </section>
          ))}
        </div>
      </div>
    </Modal>
  );
}
