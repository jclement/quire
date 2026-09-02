// The task line grammar, mirrored from internal/markdown/scan.go so the
// toolbar can edit a task's metadata in place: `- [ ] text ⏫ 📅 2026-09-10
// 🛫 2026-09-08 ⏳ 🔁 every week ✅ 2026-09-02`. Parse pulls the markers out
// of the text; serialize writes them back in one canonical order, which is
// also how the server writes them (recurrence spawns, completion stamps).
export interface TaskLine {
  /** Everything before the checkbox: indentation and the list marker. */
  prefix: string;
  done: boolean;
  /** The task's words, markers removed, whitespace collapsed. */
  text: string;
  due: string;
  defer: string;
  completedOn: string;
  waiting: boolean;
  /** 0 none, 1 high, 2 medium, 3 low — the scanner's numbering. */
  priority: 0 | 1 | 2 | 3;
  /** "every week", "every 3 months when done", or "". */
  recur: string;
}

const CHECKBOX = /^(\s*(?:[-*+]|\d+[.)])\s+)\[([ xX])\]\s?(.*)$/;
const RECUR =
  /🔁\s*(every(?:\s+\d+)?\s+(?:day|week|month|year)s?(?:\s+when\s+done)?)/;
const DATE = /^\s*(\d{4}-\d{2}-\d{2})/;

export const PRIORITY_MARK = { 1: "⏫", 2: "🔼", 3: "🔽" } as const;

/** Parses a task line; null when the line is not a task. */
export function parseTaskLine(line: string): TaskLine | null {
  const m = CHECKBOX.exec(line);
  if (!m) return null;
  let text = m[3]!;
  const out: TaskLine = {
    prefix: m[1]!,
    done: m[2] !== " ",
    text: "",
    due: "",
    defer: "",
    completedOn: "",
    waiting: false,
    priority: 0,
    recur: "",
  };
  const recur = RECUR.exec(text);
  if (recur) {
    out.recur = recur[1]!.split(/\s+/).join(" ");
    text = text.replace(recur[0], "");
  }
  const dated = (marker: string): string => {
    const at = text.indexOf(marker);
    if (at < 0) return "";
    const after = text.slice(at + marker.length);
    const d = DATE.exec(after);
    if (d) {
      const cut = after.indexOf(d[1]!) + d[1]!.length;
      text = text.slice(0, at) + after.slice(cut);
      return d[1]!;
    }
    text = text.slice(0, at) + after;
    return "";
  };
  out.due = dated("📅");
  out.defer = dated("🛫");
  out.completedOn = dated("✅");
  if (text.includes("⏳")) {
    out.waiting = true;
    text = text.replace("⏳", "");
  }
  for (const level of [1, 2, 3] as const) {
    const mark = PRIORITY_MARK[level];
    if (text.includes(mark)) {
      out.priority = level;
      text = text.replace(mark, "");
      break;
    }
  }
  out.text = text.split(/\s+/).filter(Boolean).join(" ");
  return out;
}

/** Writes a task line back in canonical marker order. */
export function serializeTaskLine(task: TaskLine): string {
  const parts = [task.text];
  if (task.priority) parts.push(PRIORITY_MARK[task.priority]);
  if (task.due) parts.push(`📅 ${task.due}`);
  if (task.defer) parts.push(`🛫 ${task.defer}`);
  if (task.waiting) parts.push("⏳");
  if (task.recur) parts.push(`🔁 ${task.recur}`);
  if (task.completedOn) parts.push(`✅ ${task.completedOn}`);
  return `${task.prefix}[${task.done ? "x" : " "}] ${parts.filter(Boolean).join(" ")}`;
}
