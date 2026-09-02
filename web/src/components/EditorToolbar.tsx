// The editor's toolbar: always shown above the editor, with buttons that
// enable and disable by what the cursor is on. Each action injects or
// rewrites markdown at the cursor through editor/commands.ts; the toolbar
// itself holds no editor state beyond the context it is handed.
import type { EditorView } from "@codemirror/view";
import {
  Check,
  CheckSquare,
  Heading as HeadingIcon,
  LayoutGrid,
  MessageSquareQuote,
  PenTool,
  Table2,
  WrapText,
} from "lucide-react";
import { useEffect, useState, type ReactNode } from "react";
import { CALLOUT_TYPES, type CalloutType } from "../lib/callouts.ts";
import type { TaskLine } from "../lib/taskLine.ts";
import {
  applyTaskLine,
  insertTable,
  makeTask,
  setCallout,
  setHeading,
  type EditorContext,
} from "../editor/commands.ts";
import { editTableAtCursor, formatTableAtCursor } from "../editor/tables.ts";

interface EditorToolbarProps {
  context: EditorContext | null;
  run: (command: (view: EditorView) => void) => void;
  onInsertDrawing: () => void;
}

const BUTTON =
  "flex size-7 items-center justify-center rounded border border-border text-body hover:bg-hover hover:text-heading disabled:cursor-default disabled:opacity-40 disabled:hover:bg-transparent disabled:hover:text-body";

/** Keeps the editor focused through a click, so the command lands on the
 * cursor the person can see. Popovers with inputs opt out. */
const keepFocus = (event: React.MouseEvent) => event.preventDefault();

export function EditorToolbar({
  context,
  run,
  onInsertDrawing,
}: EditorToolbarProps) {
  const [open, setOpen] = useState<"heading" | "callout" | "task" | null>(null);
  const task = context?.task ?? null;
  const level = context?.headingLevel ?? 0;
  const callout = context?.callout ?? null;

  return (
    <div
      role="toolbar"
      aria-label="Editor tools"
      className="mb-1.5 flex flex-wrap items-center gap-0.5 border-b border-border pb-1.5 print:hidden"
    >
      <Menu
        open={open === "heading"}
        onOpen={() => setOpen(open === "heading" ? null : "heading")}
        onClose={() => setOpen(null)}
        label="Heading"
        tip={level ? `Heading ${level} — change level` : "Heading"}
        active={level > 0}
        icon={
          level ? (
            <span className="font-mono text-[11px] font-semibold">
              H{level}
            </span>
          ) : (
            <HeadingIcon className="size-4" aria-hidden="true" />
          )
        }
        disabled={!context}
      >
        {[1, 2, 3].map((n) => (
          <MenuItem
            key={n}
            selected={level === n}
            onClick={() => run((view) => setHeading(view, n))}
          >
            Heading {n}
          </MenuItem>
        ))}
        <MenuItem
          selected={level === 0}
          onClick={() => run((view) => setHeading(view, 0))}
        >
          Plain text
        </MenuItem>
      </Menu>

      <div className="relative">
        {/* One button for tasks: prose becomes a task and the details open;
            a task just opens its details. */}
        <IconButton
          label="Task"
          tip={task ? "Edit task" : "Make this line a task"}
          onClick={() => {
            if (!task) run(makeTask);
            setOpen(open === "task" ? null : "task");
          }}
          disabled={!context}
          active={task !== null}
          expanded={open === "task"}
          keepFocus={false}
        >
          <CheckSquare className="size-4" aria-hidden="true" />
        </IconButton>
        {open === "task" && task && context ? (
          <TaskDetails
            task={task}
            onApply={(next) => {
              run((view) => applyTaskLine(view, context.line, next));
              setOpen(null);
            }}
            onClose={() => setOpen(null)}
          />
        ) : null}
      </div>

      <Menu
        open={open === "callout"}
        onOpen={() => setOpen(open === "callout" ? null : "callout")}
        onClose={() => setOpen(null)}
        label="Callout"
        tip={callout ? `Callout: ${callout} — change type` : "Callout"}
        active={callout !== null}
        icon={<MessageSquareQuote className="size-4" aria-hidden="true" />}
        disabled={!context}
      >
        {CALLOUT_TYPES.map((type) => (
          <MenuItem
            key={type}
            selected={callout === type}
            onClick={() => run((view) => setCallout(view, type as CalloutType))}
          >
            {capitalize(type)}
          </MenuItem>
        ))}
      </Menu>

      <Divider />

      <IconButton
        label="Table"
        tip="Insert a table"
        onClick={() => run(insertTable)}
        disabled={!context || context.inTable}
      >
        <Table2 className="size-4" aria-hidden="true" />
      </IconButton>
      <IconButton
        label="Reformat table"
        tip="Reformat table (⌘⌥T)"
        onClick={() => run((view) => void formatTableAtCursor(view))}
        disabled={!context?.inTable}
      >
        <WrapText className="size-4" aria-hidden="true" />
      </IconButton>
      <IconButton
        label="Edit as grid"
        tip="Edit table as a grid"
        onClick={() => run((view) => void editTableAtCursor(view))}
        disabled={!context?.inTable}
      >
        <LayoutGrid className="size-4" aria-hidden="true" />
      </IconButton>

      <Divider />

      <IconButton
        label="Drawing"
        tip="Insert a drawing"
        onClick={onInsertDrawing}
        disabled={!context}
      >
        <PenTool className="size-4" aria-hidden="true" />
      </IconButton>
    </div>
  );
}

function Divider() {
  return (
    <span className="mx-1 h-4 border-l border-border" aria-hidden="true" />
  );
}

/**
 * A square icon button with a tooltip (the accessible name doubles as the
 * fallback tip). Keeps the editor focused through the click unless it
 * opens something with inputs of its own.
 */
function IconButton({
  label,
  tip,
  onClick,
  disabled,
  active = false,
  expanded,
  keepFocus: keep = true,
  children,
}: {
  label: string;
  tip: string;
  onClick: () => void;
  disabled: boolean;
  active?: boolean;
  expanded?: boolean;
  keepFocus?: boolean;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      aria-label={label}
      aria-pressed={expanded === undefined && active ? true : undefined}
      aria-expanded={expanded}
      data-tip={tip}
      onMouseDown={keep ? keepFocus : undefined}
      onClick={onClick}
      disabled={disabled}
      className={`${BUTTON} tip ${active ? "text-accent" : ""}`}
    >
      {children}
    </button>
  );
}

function capitalize(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1);
}

function Menu({
  open,
  onOpen,
  onClose,
  label,
  tip,
  icon,
  active,
  disabled,
  children,
}: {
  open: boolean;
  onOpen: () => void;
  onClose: () => void;
  label: string;
  tip: string;
  icon: ReactNode;
  active: boolean;
  disabled: boolean;
  children: ReactNode;
}) {
  useEscape(open, onClose);
  return (
    <div className="relative">
      <button
        type="button"
        onMouseDown={keepFocus}
        onClick={onOpen}
        disabled={disabled}
        aria-label={label}
        aria-haspopup="menu"
        aria-expanded={open}
        data-tip={tip}
        className={`${BUTTON} tip ${active ? "text-accent" : ""}`}
      >
        {icon}
      </button>
      {open ? (
        <>
          <div
            className="fixed inset-0 z-10"
            aria-hidden="true"
            onMouseDown={onClose}
          />
          <div
            role="menu"
            aria-label={label}
            className="absolute top-full left-0 z-20 mt-1 w-40 rounded border border-border bg-raised py-1 shadow-lg"
            onClick={onClose}
          >
            {children}
          </div>
        </>
      ) : null}
    </div>
  );
}

function MenuItem({
  selected,
  onClick,
  children,
}: {
  selected: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      role="menuitem"
      onMouseDown={keepFocus}
      onClick={onClick}
      className="flex h-8 w-full items-center gap-2 px-3 text-left text-xs text-body hover:bg-hover hover:text-heading"
    >
      <span className="flex-1">{children}</span>
      {selected ? (
        <Check className="size-3.5 text-accent" aria-hidden="true" />
      ) : null}
    </button>
  );
}

/** Escape closes a popup from anywhere, since focus stays in the editor. */
function useEscape(open: boolean, onClose: () => void) {
  useEffect(() => {
    if (!open) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      event.stopPropagation();
      onClose();
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [open, onClose]);
}

/** The task's metadata as a small form; Apply rewrites the line. */
function TaskDetails({
  task,
  onApply,
  onClose,
}: {
  task: TaskLine;
  onApply: (next: TaskLine) => void;
  onClose: () => void;
}) {
  const [draft, setDraft] = useState<TaskLine>(task);
  useEscape(true, onClose);
  const field =
    "field-bare h-7 w-full rounded border border-border bg-surface px-1.5 text-xs text-heading outline-none focus:border-accent";
  return (
    <>
      <div
        className="fixed inset-0 z-10"
        aria-hidden="true"
        onMouseDown={onClose}
      />
      <form
        role="dialog"
        aria-label="Task details"
        onSubmit={(event) => {
          event.preventDefault();
          onApply(draft);
        }}
        className="absolute top-full left-0 z-20 mt-1 grid w-64 grid-cols-[auto_1fr] items-center gap-x-3 gap-y-2 rounded border border-border bg-raised p-3 font-sans text-xs shadow-lg"
      >
        <label htmlFor="task-due" className="text-muted">
          Due
        </label>
        <input
          id="task-due"
          type="date"
          value={draft.due}
          onChange={(event) => setDraft({ ...draft, due: event.target.value })}
          className={field}
        />
        <label htmlFor="task-defer" className="text-muted">
          Defer until
        </label>
        <input
          id="task-defer"
          type="date"
          value={draft.defer}
          onChange={(event) =>
            setDraft({ ...draft, defer: event.target.value })
          }
          className={field}
        />
        <label htmlFor="task-priority" className="text-muted">
          Priority
        </label>
        <select
          id="task-priority"
          value={draft.priority}
          onChange={(event) =>
            setDraft({
              ...draft,
              priority: Number(event.target.value) as TaskLine["priority"],
            })
          }
          className={field}
        >
          <option value={0}>None</option>
          <option value={1}>High ⏫</option>
          <option value={2}>Medium 🔼</option>
          <option value={3}>Low 🔽</option>
        </select>
        <label htmlFor="task-repeat" className="text-muted">
          Repeat
        </label>
        <input
          id="task-repeat"
          value={draft.recur}
          onChange={(event) =>
            setDraft({ ...draft, recur: event.target.value.trim() })
          }
          placeholder="every week"
          className={field}
        />
        <label htmlFor="task-waiting" className="text-muted">
          Waiting
        </label>
        <input
          id="task-waiting"
          type="checkbox"
          checked={draft.waiting}
          onChange={(event) =>
            setDraft({ ...draft, waiting: event.target.checked })
          }
          className="size-3.5 accent-(--accent)"
        />
        <div className="col-span-2 mt-1 flex justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            className="flex h-7 items-center rounded border border-border px-2 text-xs text-body hover:bg-hover"
          >
            Cancel
          </button>
          <button
            type="submit"
            className="flex h-7 items-center rounded border border-border bg-accent px-2 text-xs font-medium text-white hover:opacity-90"
          >
            Apply
          </button>
        </div>
      </form>
    </>
  );
}
