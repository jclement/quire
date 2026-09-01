// Quick capture (`c` key / mobile FAB): one autofocused text input that POSTs a
// task into today's daily note. Enter saves and closes; Shift+Enter saves and
// stays for the next thought. Optional due chips (Today / Tomorrow / Weekend)
// are the only other interaction — zero required fields beyond the text. The
// content mounts fresh per open, so no reset bookkeeping.
import { Check, Zap } from "lucide-react";
import { useRef, useState } from "react";
import { useCreateTask } from "../api/queries.ts";
import { addDaysISO, nextSaturdayISO, todayISO } from "../lib/dates.ts";
import { useUi } from "../keys/UiContext.tsx";
import { Modal } from "./Modal.tsx";

type DueChip = "today" | "tomorrow" | "weekend";

function chipDate(chip: DueChip): string {
  const today = todayISO();
  if (chip === "today") return today;
  if (chip === "tomorrow") return addDaysISO(today, 1);
  return nextSaturdayISO(today);
}

export function QuickCapture() {
  const { overlays, setOverlay } = useUi();
  const open = overlays.capture;
  const close = () => setOverlay("capture", false);
  return (
    <Modal open={open} onClose={close} variant="sheet" label="Quick capture">
      <CaptureContent close={close} />
    </Modal>
  );
}

function CaptureContent({ close }: { close: () => void }) {
  const [text, setText] = useState("");
  const [chip, setChip] = useState<DueChip | null>(null);
  const [justSaved, setJustSaved] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const createTask = useCreateTask();

  const save = (keepOpen: boolean) => {
    const trimmed = text.trim();
    if (!trimmed || createTask.isPending) return;
    createTask.mutate(
      { text: trimmed, ...(chip ? { due: chipDate(chip) } : {}) },
      {
        onSuccess: () => {
          if (!keepOpen) {
            close();
            return;
          }
          setText("");
          setChip(null);
          setJustSaved(true);
          setTimeout(() => setJustSaved(false), 1_500);
          inputRef.current?.focus();
        },
      },
    );
  };

  return (
    <>
      <div className="flex items-center gap-2 border-b border-border px-3">
        <Zap className="size-4 shrink-0 text-accent" aria-hidden="true" />
        <input
          ref={inputRef}
          autoFocus
          value={text}
          onChange={(event) => setText(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              event.preventDefault();
              save(event.shiftKey);
            }
          }}
          placeholder="Capture a task…"
          aria-label="New task text"
          className="h-12 w-full bg-transparent text-sm text-heading outline-none placeholder:text-muted"
        />
        {justSaved ? (
          <Check className="size-4 shrink-0 text-ok" aria-hidden="true" />
        ) : null}
      </div>
      <div className="flex items-center gap-1.5 px-3 py-2.5">
        {(["today", "tomorrow", "weekend"] as const).map((option) => (
          <button
            key={option}
            type="button"
            onClick={() => setChip(chip === option ? null : option)}
            className={`h-7 rounded-full border px-2.5 text-xs capitalize ${
              chip === option
                ? "border-accent bg-selected text-heading"
                : "border-border text-muted hover:bg-hover hover:text-body"
            }`}
          >
            {option}
          </button>
        ))}
        <span className="ml-auto hidden font-mono text-[10px] text-muted md:block">
          ↵ save · ⇧↵ save + another
        </span>
      </div>
      {createTask.isError ? (
        <p className="border-t border-border px-3 py-2 text-xs text-danger">
          Couldn't save — {createTask.error.message}
        </p>
      ) : null}
    </>
  );
}
