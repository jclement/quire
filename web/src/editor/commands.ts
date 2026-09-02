// Toolbar commands: what the buttons above the editor do to the buffer, and
// the context (is the cursor in a table? on a task? under a heading?) that
// decides which of them are enabled. Every command works on the line under
// the cursor, or the selected lines where that makes sense (callouts).
import type { EditorState } from "@codemirror/state";
import { EditorSelection } from "@codemirror/state";
import type { EditorView } from "@codemirror/view";
import type { CalloutType } from "../lib/callouts.ts";
import { findTableAt } from "../lib/tables.ts";
import {
  parseTaskLine,
  serializeTaskLine,
  type TaskLine,
} from "../lib/taskLine.ts";
import { toggleLineCheckbox } from "./extensions.ts";

export interface EditorContext {
  /** 1-based line under the cursor. */
  line: number;
  inTable: boolean;
  headingLevel: number;
  /** The task on the cursor line, parsed; null when it is not one. */
  task: TaskLine | null;
  isListItem: boolean;
  /** The callout type when the cursor line is inside a `> [!type]` block. */
  callout: CalloutType | string | null;
}

const HEADING = /^(#{1,6})\s+/;
const LIST_ITEM = /^\s*(?:[-*+]|\d+[.)])\s+/;
const CALLOUT_HEAD = /^\s*>\s*\[!([a-z]+)\]/i;

/** What the cursor is on. Cheap enough to run on every selection change. */
export function editorContext(state: EditorState): EditorContext {
  const head = state.selection.main.head;
  const line = state.doc.lineAt(head);
  const text = line.text;
  return {
    line: line.number,
    inTable: findTableAt(state.doc.toString(), head) !== null,
    headingLevel: HEADING.exec(text)?.[1]?.length ?? 0,
    task: parseTaskLine(text),
    isListItem: LIST_ITEM.test(text),
    callout: calloutAt(state, line.number),
  };
}

/** The type of the callout the line sits in, walking up through `>` lines. */
function calloutAt(state: EditorState, lineNumber: number): string | null {
  for (let n = lineNumber; n >= 1; n--) {
    const text = state.doc.line(n).text;
    if (!/^\s*>/.test(text)) return null;
    const head = CALLOUT_HEAD.exec(text);
    if (head) return head[1]!.toLowerCase();
  }
  return null;
}

function replaceLine(view: EditorView, lineNumber: number, next: string): void {
  const line = view.state.doc.line(lineNumber);
  if (line.text === next) return;
  view.dispatch(
    {
      changes: { from: line.from, to: line.to, insert: next },
      selection: EditorSelection.cursor(line.from + next.length),
    },
    { userEvent: "input" },
  );
}

/** Sets the cursor line's heading level; 0 makes it plain text. */
export function setHeading(view: EditorView, level: number): void {
  const line = view.state.doc.lineAt(view.state.selection.main.head);
  const body = line.text.replace(HEADING, "");
  replaceLine(
    view,
    line.number,
    level > 0 ? `${"#".repeat(level)} ${body}` : body,
  );
}

/** Turns the cursor line into a task (a bare line gets `- [ ] `; a list
 * item gets a checkbox); a task is left as it is. */
export function makeTask(view: EditorView): void {
  const line = view.state.doc.lineAt(view.state.selection.main.head);
  if (parseTaskLine(line.text)) return;
  const toggled = toggleLineCheckbox(line.text);
  replaceLine(view, line.number, toggled ?? `- [ ] ${line.text.trimStart()}`);
}

/** Rewrites the task on a line with new metadata (the details popover). */
export function applyTaskLine(
  view: EditorView,
  lineNumber: number,
  task: TaskLine,
): void {
  replaceLine(view, lineNumber, serializeTaskLine(task));
}

/**
 * Wraps the selected lines (or the cursor line) in a callout of the given
 * type; when already inside one, retypes it instead.
 */
export function setCallout(view: EditorView, type: CalloutType): void {
  const { state } = view;
  const { from, to } = state.selection.main;
  const first = state.doc.lineAt(from);
  const last = state.doc.lineAt(to);
  // Retype an existing callout: find its head line.
  for (let n = first.number; n >= 1; n--) {
    const text = state.doc.line(n).text;
    if (!/^\s*>/.test(text)) break;
    if (CALLOUT_HEAD.test(text)) {
      replaceLine(
        view,
        n,
        text.replace(CALLOUT_HEAD, (m, t: string) =>
          m.replace(`[!${t}]`, `[!${type}]`),
        ),
      );
      return;
    }
  }
  const lines: string[] = [];
  for (let n = first.number; n <= last.number; n++)
    lines.push(state.doc.line(n).text);
  const body = lines.map((l) => (l.trim() === "" ? ">" : `> ${l}`)).join("\n");
  const insert = `> [!${type}]\n${body}`;
  view.dispatch(
    {
      changes: { from: first.from, to: last.to, insert },
      selection: EditorSelection.cursor(first.from + insert.length),
    },
    { userEvent: "input" },
  );
}

/** Inserts a block of markdown on its own line(s) at the cursor. */
export function insertBlock(view: EditorView, block: string): void {
  const { state } = view;
  const line = state.doc.lineAt(state.selection.main.head);
  const before = line.text.trim() === "" ? "" : "\n\n";
  const at = line.text.trim() === "" ? line.from : line.to;
  const insert = `${before}${block}`;
  view.dispatch(
    {
      changes: { from: at, to: line.text.trim() === "" ? line.to : at, insert },
      selection: EditorSelection.cursor(at + insert.length),
      scrollIntoView: true,
    },
    { userEvent: "input" },
  );
}

/** A starter table, cursor left in its first cell. */
export function insertTable(view: EditorView): void {
  const block = "| Column | Column |\n| --- | --- |\n|  |  |";
  const { state } = view;
  const line = state.doc.lineAt(state.selection.main.head);
  const empty = line.text.trim() === "";
  const before = empty ? "" : "\n\n";
  const at = empty ? line.from : line.to;
  view.dispatch(
    {
      changes: {
        from: at,
        to: empty ? line.to : at,
        insert: `${before}${block}`,
      },
      selection: EditorSelection.cursor(at + before.length + 2),
      scrollIntoView: true,
    },
    { userEvent: "input" },
  );
}
