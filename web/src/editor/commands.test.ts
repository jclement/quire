import { describe, expect, test } from "bun:test";
import { EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import {
  applyTaskLine,
  editorContext,
  insertTable,
  makeTask,
  setCallout,
  setHeading,
} from "./commands.ts";
import { parseTaskLine } from "../lib/taskLine.ts";

function view(doc: string, cursor: number) {
  return new EditorView({
    state: EditorState.create({ doc, selection: { anchor: cursor } }),
  });
}

describe("editor commands", () => {
  test("context reads heading, task, list and table state", () => {
    const v = view(
      "## Title\n- [ ] do 📅 2026-09-10\n| a | b |\n|---|---|\n| 1 | 2 |\n",
      0,
    );
    expect(editorContext(v.state)).toMatchObject({
      line: 1,
      headingLevel: 2,
      task: null,
      inTable: false,
    });
    v.dispatch({ selection: { anchor: v.state.doc.line(2).from + 3 } });
    const ctx = editorContext(v.state);
    expect(ctx.task?.due).toBe("2026-09-10");
    expect(ctx.isListItem).toBe(true);
    v.dispatch({ selection: { anchor: v.state.doc.line(4).from + 2 } });
    expect(editorContext(v.state).inTable).toBe(true);
  });

  test("heading levels set and clear", () => {
    const v = view("hello", 2);
    setHeading(v, 2);
    expect(v.state.doc.toString()).toBe("## hello");
    setHeading(v, 1);
    expect(v.state.doc.toString()).toBe("# hello");
    setHeading(v, 0);
    expect(v.state.doc.toString()).toBe("hello");
  });

  test("make task from prose and from a bullet; metadata applies in order", () => {
    const v = view("call sarah\n- buy milk", 3);
    makeTask(v);
    expect(v.state.doc.line(1).text).toBe("- [ ] call sarah");
    v.dispatch({ selection: { anchor: v.state.doc.line(2).from + 2 } });
    makeTask(v);
    expect(v.state.doc.line(2).text).toBe("- [ ] buy milk");
    const task = parseTaskLine(v.state.doc.line(2).text)!;
    applyTaskLine(v, 2, {
      ...task,
      due: "2026-09-10",
      priority: 2,
      waiting: true,
    });
    expect(v.state.doc.line(2).text).toBe("- [ ] buy milk 🔼 📅 2026-09-10 ⏳");
  });

  test("callouts wrap a selection and retype in place", () => {
    const v = view("one\ntwo\n\nthree", 0);
    v.dispatch({ selection: { anchor: 0, head: 7 } });
    setCallout(v, "warning");
    expect(v.state.doc.toString()).toBe("> [!warning]\n> one\n> two\n\nthree");
    v.dispatch({ selection: { anchor: 16 } });
    expect(editorContext(v.state).callout).toBe("warning");
    setCallout(v, "tip");
    expect(v.state.doc.line(1).text).toBe("> [!tip]");
  });

  test("a table is inserted on its own lines with the cursor in the first cell", () => {
    const v = view("intro", 5);
    insertTable(v);
    expect(v.state.doc.toString()).toBe(
      "intro\n\n| Column | Column |\n| --- | --- |\n|  |  |",
    );
    expect(v.state.doc.lineAt(v.state.selection.main.head).number).toBe(3);
  });
});
