// The snooze popover on a task row (`s` key or the hover calendar button):
// quick chips (Today / Tomorrow / This weekend / Next week / Clear) plus a raw
// YYYY-MM-DD input, applied via PATCH /tasks/<id> {due}. Not optimistic — the
// content-derived task id can change on edit, so we let invalidation reconcile.
import { useRef } from "react";
import { useEditTask } from "../api/queries.ts";
import type { Task } from "../api/types.ts";
import { noAutofill } from "../lib/noAutofill.ts";
import {
  addDaysISO,
  nextMondayISO,
  nextSaturdayISO,
  parseISODate,
  todayISO,
} from "../lib/dates.ts";

interface SnoozePopoverProps {
  task: Task;
  onClose: () => void;
}

function chipOptions(): { label: string; due: string }[] {
  const today = todayISO();
  return [
    { label: "Today", due: today },
    { label: "Tomorrow", due: addDaysISO(today, 1) },
    { label: "This weekend", due: nextSaturdayISO(today) },
    { label: "Next week", due: nextMondayISO(today) },
    // Empty string is the API's "clear this field".
    { label: "Clear date", due: "" },
  ];
}

export function SnoozePopover({ task, onClose }: SnoozePopoverProps) {
  const editTask = useEditTask();
  const inputRef = useRef<HTMLInputElement>(null);

  const apply = (due: string) => {
    editTask.mutate({ id: task.id, edit: { due } });
    onClose();
  };

  const applyTyped = () => {
    const typed = inputRef.current?.value.trim() ?? "";
    if (!parseISODate(typed)) return; // Ignore anything that isn't a real date.
    apply(typed);
  };

  return (
    <>
      {/* Click-away layer under the popover. */}
      <div
        className="fixed inset-0 z-10"
        aria-hidden="true"
        onClick={(event) => {
          event.stopPropagation();
          onClose();
        }}
      />
      <div
        role="dialog"
        aria-label={`Snooze: ${task.text}`}
        onClick={(event) => event.stopPropagation()}
        onKeyDown={(event) => {
          if (event.key === "Escape") {
            event.stopPropagation();
            onClose();
          }
        }}
        className="absolute top-full right-2 z-20 mt-1 w-48 rounded border border-border bg-raised py-1 shadow-lg"
      >
        {chipOptions().map((option) => (
          <button
            key={option.label}
            type="button"
            onClick={() => apply(option.due)}
            className="flex h-8 w-full items-center px-3 text-left text-xs text-body hover:bg-hover hover:text-heading"
          >
            {option.label}
            {option.due ? (
              <span className="ml-auto font-mono text-[10px] text-muted">
                {option.due.slice(5)}
              </span>
            ) : null}
          </button>
        ))}
        <div className="mt-1 border-t border-border px-3 pt-1.5 pb-1">
          <input
            ref={inputRef}
            autoFocus
            placeholder="YYYY-MM-DD"
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                event.preventDefault();
                applyTyped();
              }
            }}
            aria-label="Snooze to date"
            {...noAutofill("snooze-date")}
            className="field-bare h-7 w-full rounded border border-border bg-transparent px-1.5 font-mono text-[11px] text-heading outline-none placeholder:text-muted focus:border-accent"
          />
        </div>
      </div>
    </>
  );
}
