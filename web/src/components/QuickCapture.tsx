// Quick capture (`c` key / mobile FAB): one autofocused text input that POSTs a
// task into today's daily note, plus an optional photo (camera on mobile) —
// with a file attached it goes through POST /capture and the text becomes
// optional. Enter saves and closes; Shift+Enter saves and stays. Due chips
// (Today / Tomorrow / Weekend) are the only other interaction. The content
// mounts fresh per open, so no reset bookkeeping.
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Check, ImagePlus, X, Zap } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { api, errorMessage } from "../api/client.ts";
import type { Document, Task } from "../api/types.ts";
import { invalidateTaskCaches } from "../api/queries.ts";
import { addDaysISO, nextSaturdayISO, todayISO } from "../lib/dates.ts";
import { noAutofill } from "../lib/noAutofill.ts";
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

interface CaptureInput {
  text: string;
  kind: "task" | "note";
  due?: string;
  file: File | null;
}

function CaptureContent({ close }: { close: () => void }) {
  const [text, setText] = useState("");
  const [kind, setKind] = useState<"task" | "note">("task");
  const [chip, setChip] = useState<DueChip | null>(null);
  const [exact, setExact] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [justSaved, setJustSaved] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const fileRef = useRef<HTMLInputElement>(null);
  const queryClient = useQueryClient();

  const save = useMutation<Document | Task, Error, CaptureInput>({
    // A photo goes through /capture (which files it + mints the task); plain
    // text stays on the cheaper /tasks path.
    mutationFn: (input: CaptureInput) => {
      // A thought is not an action: prose goes into the day as prose, and
      // never turns up in the task inbox pretending to be something to do.
      if (input.kind === "note" && !input.file) {
        return api.captureNote(input.text);
      }
      return input.file
        ? api.capture({
            file: input.file,
            ...(input.text ? { text: input.text } : {}),
            ...(input.due ? { due: input.due } : {}),
          })
        : api.createTask(input.text, input.due);
    },
    onSuccess: () => invalidateTaskCaches(queryClient),
  });

  const submit = (keepOpen: boolean) => {
    const trimmed = text.trim();
    // With a photo attached the text is optional; without one it's the task.
    if ((!trimmed && !file) || save.isPending) return;
    const due = exact || (chip ? chipDate(chip) : "");
    save.mutate(
      { text: trimmed, kind, ...(due ? { due } : {}), file },
      {
        onSuccess: () => {
          if (!keepOpen) {
            close();
            return;
          }
          setText("");
          setChip(null);
          setExact("");
          setFile(null);
          setJustSaved(true);
          setTimeout(() => setJustSaved(false), 1_500);
          inputRef.current?.focus();
        },
      },
    );
  };

  return (
    <>
      <div className="flex items-center gap-2 border-b border-border px-3 focus-within:border-accent">
        <Zap className="size-4 shrink-0 text-accent" aria-hidden="true" />
        <input
          ref={inputRef}
          autoFocus
          value={text}
          onChange={(event) => setText(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              event.preventDefault();
              submit(event.shiftKey);
            }
          }}
          placeholder={
            file
              ? "Add a note (optional)…"
              : kind === "note"
                ? "Capture a thought…"
                : "Capture a task…"
          }
          aria-label="New task text"
          {...noAutofill("capture")}
          className="field-bare h-12 w-full bg-transparent text-sm text-heading outline-none placeholder:text-muted"
        />
        {justSaved ? (
          <Check className="size-4 shrink-0 text-ok" aria-hidden="true" />
        ) : null}
        <button
          type="button"
          onClick={() => fileRef.current?.click()}
          aria-label="Attach a photo"
          className="flex size-8 shrink-0 items-center justify-center rounded text-muted hover:bg-hover hover:text-heading"
        >
          <ImagePlus className="size-4" aria-hidden="true" />
        </button>
        <input
          ref={fileRef}
          type="file"
          accept="image/*"
          capture="environment"
          className="hidden"
          aria-hidden="true"
          tabIndex={-1}
          onChange={(event) => {
            setFile(event.target.files?.[0] ?? null);
            // Same file re-pickable after remove.
            event.target.value = "";
            inputRef.current?.focus();
          }}
        />
      </div>
      {file ? <PhotoChip file={file} onRemove={() => setFile(null)} /> : null}
      <div className="flex flex-wrap items-center gap-1.5 px-3 py-2.5">
        <div
          role="group"
          aria-label="Capture as"
          className="flex h-7 items-center rounded border border-border"
        >
          {(["task", "note"] as const).map((option) => (
            <button
              key={option}
              type="button"
              aria-pressed={kind === option}
              onClick={() => setKind(option)}
              className={`h-full px-2 text-xs capitalize first:rounded-l last:rounded-r ${
                kind === option
                  ? "bg-selected text-heading"
                  : "text-muted hover:bg-hover hover:text-body"
              }`}
            >
              {option}
            </button>
          ))}
        </div>
        {kind === "task" ? (
          <>
            {(["today", "tomorrow", "weekend"] as const).map((option) => (
              <button
                key={option}
                type="button"
                onClick={() => {
                  setChip(chip === option ? null : option);
                  setExact("");
                }}
                className={`h-7 rounded-full border px-2.5 text-xs capitalize ${
                  chip === option && !exact
                    ? "border-accent bg-selected text-heading"
                    : "border-border text-muted hover:bg-hover hover:text-body"
                }`}
              >
                {option}
              </button>
            ))}
            {/* Anything else — the form due on Friday — without a second visit. */}
            <input
              type="date"
              value={exact}
              onChange={(event) => {
                setExact(event.target.value);
                setChip(null);
              }}
              aria-label="Due date"
              className={`field-bare h-7 rounded border bg-transparent px-1.5 text-xs outline-none ${
                exact
                  ? "border-accent text-heading"
                  : "border-border text-muted"
              }`}
            />
          </>
        ) : null}
        <span className="ml-auto hidden font-mono text-[10px] text-muted md:block">
          ↵ save · ⇧↵ save + another
        </span>
      </div>
      {save.isError ? (
        <p className="border-t border-border px-3 py-2 text-xs text-danger">
          Couldn't save — {errorMessage(save.error)}
        </p>
      ) : null}
    </>
  );
}

/** Tiny thumbnail chip for the attached photo. */
function PhotoChip({ file, onRemove }: { file: File; onRemove: () => void }) {
  const url = useMemo(() => URL.createObjectURL(file), [file]);
  useEffect(() => () => URL.revokeObjectURL(url), [url]);
  return (
    <div className="flex items-center gap-2 border-b border-border px-3 py-1.5">
      <img
        src={url}
        alt=""
        className="size-8 shrink-0 rounded border border-border object-cover"
      />
      <span className="truncate text-xs text-muted">{file.name}</span>
      <button
        type="button"
        onClick={onRemove}
        aria-label="Remove photo"
        className="ml-auto flex size-6 shrink-0 items-center justify-center rounded text-muted hover:bg-hover hover:text-heading"
      >
        <X className="size-3.5" aria-hidden="true" />
      </button>
    </div>
  );
}
