// Inbox triage: one key per decision, so processing the inbox is a run of
// j, t, j, s, j, n rather than a form per task. The table here is the one
// source for the row buttons, the keys and the cheat sheet, so the three
// cannot disagree about what `w` does.
import type { Task, TaskEdit } from "../api/types.ts";
import { addDaysISO, nextMondayISO } from "./dates.ts";

export type TriageKey = "t" | "m" | "w" | "d" | "f" | "p" | "s" | "n";

export interface TriageAction {
  key: TriageKey;
  /** Short verb for the cheat sheet and the button's accessible name. */
  label: string;
}

/** In the order the row shows them: scheduling, then the ways out. */
export const TRIAGE_ACTIONS: TriageAction[] = [
  { key: "t", label: "Due today" },
  { key: "m", label: "Due tomorrow" },
  { key: "w", label: "Due next Monday" },
  { key: "d", label: "Pick a due date" },
  { key: "f", label: "Defer until…" },
  { key: "p", label: "Waiting on someone" },
  { key: "s", label: "Someday" },
  { key: "n", label: "Make it a note" },
];

/** The tag that parks a task (index.SomedayTag on the server). */
export const SOMEDAY_TAG = "someday";

/** What a triage key does to a task: an edit to send straight away, a date
 * to pick first, or the note dialog. */
export type TriageStep =
  | { kind: "edit"; edit: TaskEdit; toast: string }
  | { kind: "pick"; field: "due" | "defer" }
  | { kind: "note" };

export function triageStep(
  key: TriageKey,
  task: Task,
  today: string,
): TriageStep {
  switch (key) {
    case "t":
      return { kind: "edit", edit: { due: today }, toast: "Due today" };
    case "m":
      return {
        kind: "edit",
        edit: { due: addDaysISO(today, 1) },
        toast: "Due tomorrow",
      };
    case "w":
      return {
        kind: "edit",
        edit: { due: nextMondayISO(today) },
        toast: "Due next Monday",
      };
    case "d":
      return { kind: "pick", field: "due" };
    case "f":
      return { kind: "pick", field: "defer" };
    case "p":
      return { kind: "edit", edit: { waiting: true }, toast: "Waiting" };
    case "s":
      return {
        kind: "edit",
        edit: { text: withTag(task.text, SOMEDAY_TAG) },
        toast: "Parked for someday",
      };
    case "n":
      return { kind: "note" };
  }
}

/** Appends #tag to task text unless it already carries it. */
export function withTag(text: string, tag: string): string {
  const has = new RegExp(`(^|\\s)#${tag}(?![\\p{L}\\p{N}_/-])`, "iu");
  return has.test(text) ? text : `${text} #${tag}`;
}
