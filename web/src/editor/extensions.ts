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

const markdownHighlight = HighlightStyle.define([
  { tag: tags.heading, color: "var(--heading)", fontWeight: "600" },
  { tag: tags.strong, color: "var(--heading)", fontWeight: "600" },
  { tag: tags.emphasis, fontStyle: "italic" },
  {
    tag: tags.strikethrough,
    textDecoration: "line-through",
    color: "var(--muted)",
  },
  { tag: tags.link, color: "var(--accent)" },
  { tag: tags.url, color: "var(--accent)" },
  { tag: tags.monospace, color: "var(--heading)" },
  { tag: tags.quote, color: "var(--muted)", fontStyle: "italic" },
  { tag: tags.meta, color: "var(--muted)" },
  { tag: tags.processingInstruction, color: "var(--muted)" },
  { tag: tags.labelName, color: "var(--accent)" },
  { tag: tags.comment, color: "var(--muted)" },
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
