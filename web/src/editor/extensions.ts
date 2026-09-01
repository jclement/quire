// CodeMirror extensions shared by the markdown editor: theme + syntax colors
// (referencing the app's CSS custom properties so light/dark just works), list
// indentation keys, and the Cmd/Ctrl+L checkbox toggle. List *continuation* on
// Enter comes from @codemirror/lang-markdown's own keymap, enabled in
// MarkdownEditor.tsx via the GFM base language.
import { indentLess, indentMore } from "@codemirror/commands";
import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { EditorSelection } from "@codemirror/state";
import { EditorView, keymap, type KeyBinding } from "@codemirror/view";
import { tags } from "@lezer/highlight";

export const editorTheme = EditorView.theme({
  "&": {
    fontSize: "13px",
    color: "var(--body)",
  },
  ".cm-content": {
    fontFamily: "var(--font-mono)",
    caretColor: "var(--accent)",
    padding: "12px 0",
    lineHeight: "1.65",
  },
  ".cm-line": { padding: "0 12px" },
  ".cm-cursor": { borderLeftColor: "var(--accent)" },
  "&.cm-focused .cm-selectionBackground, .cm-selectionBackground": {
    background: "var(--selected)",
  },
  ".cm-activeLine": { background: "transparent" },
  ".cm-tooltip": {
    background: "var(--raised)",
    border: "1px solid var(--border)",
    color: "var(--body)",
  },
  ".cm-tooltip.cm-tooltip-autocomplete > ul > li": {
    fontFamily: "var(--font-sans)",
    fontSize: "12px",
    padding: "3px 8px",
  },
  ".cm-tooltip.cm-tooltip-autocomplete > ul > li[aria-selected]": {
    background: "var(--selected)",
    color: "var(--heading)",
  },
  ".cm-completionDetail": {
    fontFamily: "var(--font-mono)",
    fontSize: "10px",
    color: "var(--muted)",
    fontStyle: "normal",
  },
});

// Markdown source colors. Everything used to resolve to heading/muted, which
// rendered as near-monochrome text; these use dedicated --syn-* tokens so the
// document's structure is scannable while prose stays comfortable to read.
// Headings step down in size so an outline is visible at a glance.
const markdownHighlight = HighlightStyle.define([
  {
    tag: tags.heading1,
    color: "var(--syn-heading)",
    fontWeight: "700",
    fontSize: "1.35em",
  },
  {
    tag: tags.heading2,
    color: "var(--syn-heading)",
    fontWeight: "700",
    fontSize: "1.2em",
  },
  {
    tag: tags.heading3,
    color: "var(--syn-heading)",
    fontWeight: "600",
    fontSize: "1.08em",
  },
  { tag: tags.heading, color: "var(--syn-heading)", fontWeight: "600" },
  { tag: tags.strong, color: "var(--heading)", fontWeight: "700" },
  { tag: tags.emphasis, color: "var(--heading)", fontStyle: "italic" },
  {
    tag: tags.strikethrough,
    textDecoration: "line-through",
    color: "var(--muted)",
  },
  { tag: tags.link, color: "var(--syn-link)", textDecoration: "underline" },
  { tag: tags.url, color: "var(--syn-link)" },
  // Inline code and fenced content.
  { tag: tags.monospace, color: "var(--syn-code)" },
  { tag: tags.quote, color: "var(--syn-quote)", fontStyle: "italic" },
  // Frontmatter fences and the like.
  { tag: tags.meta, color: "var(--syn-marker)" },
  // The syntax characters themselves: #, *, -, >, ``` — deliberately faint so
  // the words carry the page and the markers recede.
  { tag: tags.processingInstruction, color: "var(--syn-marker)" },
  { tag: tags.labelName, color: "var(--syn-link)" },
  { tag: tags.comment, color: "var(--muted)", fontStyle: "italic" },
  // Tokens from languages embedded in fenced code blocks.
  { tag: tags.keyword, color: "var(--syn-keyword)" },
  { tag: tags.string, color: "var(--ok)" },
  { tag: tags.number, color: "var(--warn)" },
  { tag: tags.bool, color: "var(--warn)" },
  { tag: tags.typeName, color: "var(--syn-heading)" },
  { tag: tags.propertyName, color: "var(--syn-code)" },
  { tag: tags.variableName, color: "var(--body)" },
  { tag: tags.function(tags.variableName), color: "var(--syn-link)" },
  { tag: tags.operator, color: "var(--syn-marker)" },
  { tag: tags.punctuation, color: "var(--syn-marker)" },
  { tag: tags.list, color: "var(--body)" },
]);

export const editorHighlighting = syntaxHighlighting(markdownHighlight);

/** `- [ ]` ↔ `- [x]` on every selected line; bare list items gain a checkbox. */
export function toggleCheckboxOnLine(view: EditorView): boolean {
  const { state } = view;
  const changes = state.changeByRange((range) => {
    const line = state.doc.lineAt(range.head);
    const replaced = toggleLineCheckbox(line.text);
    if (replaced === null) return { range };
    const delta = replaced.length - line.text.length;
    return {
      changes: { from: line.from, to: line.to, insert: replaced },
      range: EditorSelection.cursor(
        Math.min(range.head + delta, line.from + replaced.length),
      ),
    };
  });
  view.dispatch(changes, { userEvent: "input" });
  return true;
}

const UNCHECKED = /^(\s*(?:[-*+]|\d+[.)])\s+)\[ \]\s?/;
const CHECKED = /^(\s*(?:[-*+]|\d+[.)])\s+)\[[xX]\]\s?/;
const BARE_ITEM = /^(\s*(?:[-*+]|\d+[.)])\s+)(?!\[[ xX]\])/;

function toggleLineCheckbox(text: string): string | null {
  if (UNCHECKED.test(text)) return text.replace(UNCHECKED, "$1[x] ");
  if (CHECKED.test(text)) return text.replace(CHECKED, "$1[ ] ");
  if (BARE_ITEM.test(text)) return text.replace(BARE_ITEM, "$1[ ] ");
  return null;
}

/** Tab/Shift-Tab indent (list nesting); Enter continuation lives in the
 * language keymap. */
export const indentKeymap: KeyBinding[] = [
  { key: "Tab", run: indentMore, shift: indentLess },
];

export function editorKeymap(bindings: KeyBinding[]) {
  return keymap.of([...bindings, ...indentKeymap]);
}
