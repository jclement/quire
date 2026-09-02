// Table editing in CodeMirror: reformat-at-cursor and edit-as-grid (the
// toolbar's table buttons and their keybindings), and Tab / Shift-Tab
// moving between cells so a table can be filled in without counting pipes.
//
// The formatting itself lives in lib/tables.ts (pure, tested); this file is
// only the editor plumbing around it.
import { EditorView, type KeyBinding } from "@codemirror/view";
import {
  findTableAt,
  formatAllTables,
  formatTable,
  splitCells,
} from "../lib/tables.ts";
import { requestTableEdit } from "../lib/tableEditor.ts";

/** Opens the grid editor on the table under the cursor. False when not in one. */
export function editTableAtCursor(view: EditorView): boolean {
  const { from } = view.state.selection.main;
  const text = view.state.doc.toString();
  const range = findTableAt(text, from);
  if (!range) return false;
  requestTableEdit({
    block: text.slice(range.from, range.to),
    apply: (next) => {
      // The document may have moved on while the dialog was open (a save
      // rewrote it, say), so relocate the table rather than trusting the
      // range we started with.
      const now = view.state.doc.toString();
      const again = findTableAt(now, Math.min(from, now.length));
      if (!again) return;
      view.dispatch({
        changes: { from: again.from, to: again.to, insert: next },
        selection: { anchor: again.from },
      });
      view.focus();
    },
  });
  return true;
}

/** Reformats the table under the cursor. False when not in one. */
export function formatTableAtCursor(view: EditorView): boolean {
  const { from } = view.state.selection.main;
  const text = view.state.doc.toString();
  const range = findTableAt(text, from);
  if (!range) return false;
  const before = text.slice(range.from, range.to);
  const after = formatTable(before);
  if (after === before) return true;
  // Keep the cursor on the same row: compute its line offset within the
  // table and reapply it after the rewrite.
  const cursorLine = view.state.doc.lineAt(from).number;
  view.dispatch({
    changes: { from: range.from, to: range.to, insert: after },
  });
  const line = view.state.doc.line(Math.min(cursorLine, view.state.doc.lines));
  view.dispatch({ selection: { anchor: line.to } });
  return true;
}

/** Reformats every table in the document. */
export function formatAllTablesInDoc(view: EditorView): boolean {
  const text = view.state.doc.toString();
  const after = formatAllTables(text);
  if (after === text) return true;
  const { from } = view.state.selection.main;
  view.dispatch({
    changes: { from: 0, to: text.length, insert: after },
    selection: { anchor: Math.min(from, after.length) },
  });
  return true;
}

/**
 * Tab inside a table jumps to the next cell (Shift-Tab: previous), creating
 * a new row when Tab is pressed in the last cell of the last line. Outside
 * a table it returns false so the ordinary Tab binding applies.
 */
function moveCell(view: EditorView, direction: 1 | -1): boolean {
  const state = view.state;
  const pos = state.selection.main.head;
  const text = state.doc.toString();
  const range = findTableAt(text, pos);
  if (!range) return false;

  const line = state.doc.lineAt(pos);
  const cells = cellSpans(line.text);
  if (cells.length === 0) return false;

  const column = pos - line.from;
  let index = cells.findIndex((c) => column >= c.from && column <= c.to);
  if (index < 0) index = column < cells[0]!.from ? 0 : cells.length - 1;

  let target = index + direction;
  let targetLine = line;
  if (target >= cells.length) {
    // Past the last cell: next row, or a fresh row at the end.
    if (line.to >= range.to) {
      const blank = "| " + cells.map(() => "").join(" | ") + " |";
      view.dispatch({
        changes: { from: line.to, insert: "\n" + blank },
        selection: { anchor: line.to + 3 },
      });
      return true;
    }
    targetLine = state.doc.line(line.number + 1);
    // Skip the delimiter row — nobody wants to land on `---`.
    if (targetLine.number === state.doc.lineAt(range.from).number + 1) {
      if (targetLine.to >= range.to) return true;
      targetLine = state.doc.line(targetLine.number + 1);
    }
    target = 0;
  } else if (target < 0) {
    if (line.from <= range.from) return true;
    targetLine = state.doc.line(line.number - 1);
    if (targetLine.number === state.doc.lineAt(range.from).number + 1) {
      targetLine = state.doc.line(targetLine.number - 1);
    }
    const prev = cellSpans(targetLine.text);
    target = prev.length - 1;
  }
  const spans = cellSpans(targetLine.text);
  const cell = spans[Math.min(target, spans.length - 1)];
  if (!cell) return true;
  view.dispatch({
    selection: {
      anchor: targetLine.from + cell.from,
      head: targetLine.from + cell.to,
    },
    scrollIntoView: true,
  });
  return true;
}

/** Character spans of each cell's trimmed content within a row line. */
function cellSpans(line: string): Array<{ from: number; to: number }> {
  const cells = splitCells(line);
  const spans: Array<{ from: number; to: number }> = [];
  let cursor = line.startsWith("|") ? 1 : 0;
  for (const cell of cells) {
    const at = cell === "" ? -1 : line.indexOf(cell, cursor);
    if (at >= 0) {
      spans.push({ from: at, to: at + cell.length });
      cursor = at + cell.length;
    } else {
      // Empty cell: land just after the separator.
      const next = line.indexOf("|", cursor);
      const inner = next < 0 ? line.length : next;
      spans.push({
        from: Math.min(cursor + 1, inner),
        to: Math.min(cursor + 1, inner),
      });
      cursor = next < 0 ? line.length : next + 1;
    }
  }
  return spans;
}

export const tableKeymap: KeyBinding[] = [
  { key: "Mod-Alt-t", run: formatTableAtCursor },
  { key: "Mod-Alt-Shift-t", run: formatAllTablesInDoc },
  { key: "Tab", run: (view) => moveCell(view, 1) },
  { key: "Shift-Tab", run: (view) => moveCell(view, -1) },
];
