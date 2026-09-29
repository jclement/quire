// "Make it a note" (`n` on the inbox): a checkbox that was never an action —
// a paragraph of feedback, an idea — becomes a note document titled from its
// first words. The title is editable before anything is written; the server
// does the rest (note created in the source's area, task line replaced by a
// bullet linking it), so backlinks and the index follow.
import { Link as RouterLink } from "@tanstack/react-router";
import { FileText } from "lucide-react";
import { useState } from "react";
import { errorMessage } from "../api/client.ts";
import { useDefaultArea, useTaskToNote } from "../api/queries.ts";
import type { Task } from "../api/types.ts";
import { docHref } from "../lib/docs.ts";
import { noAutofill } from "../lib/noAutofill.ts";
import { noteTitleFrom } from "../lib/noteTitle.ts";
import { useUi } from "../keys/UiContext.tsx";
import { Modal } from "./Modal.tsx";

export function TaskToNoteDialog({
  task,
  onClose,
}: {
  task: Task;
  onClose: () => void;
}) {
  const { toast } = useUi();
  const [title, setTitle] = useState(() => noteTitleFrom(task.text));
  // A daily note has no area of its own; the note then files where the
  // owner is looking, as a new document from anywhere else would.
  const area = useDefaultArea();
  const toNote = useTaskToNote();

  const submit = () => {
    if (title.trim() === "" || toNote.isPending) return;
    toNote.mutate(
      { id: task.id, title: title.trim(), area },
      {
        onSuccess: (note) => {
          toast(`Filed as note “${note.title}”`);
          onClose();
        },
      },
    );
  };

  return (
    <Modal open onClose={onClose} variant="center" label="Make it a note">
      <div className="flex flex-col">
        <div className="flex items-center gap-2 border-b border-border px-3 focus-within:border-accent">
          <FileText className="size-4 shrink-0 text-muted" aria-hidden="true" />
          <input
            autoFocus
            value={title}
            onChange={(event) => setTitle(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                event.preventDefault();
                submit();
              }
            }}
            aria-label="Note title"
            {...noAutofill("task-note-title")}
            className="field-bare h-11 w-full bg-transparent text-sm text-heading outline-none placeholder:text-muted"
          />
        </div>
        <p className="line-clamp-3 border-b border-border px-3 py-2 text-xs text-body">
          {task.text}
        </p>
        <div className="flex items-center gap-2 px-3 py-2">
          {toNote.isError ? (
            <p className="text-xs text-danger">
              Couldn't file it — {errorMessage(toNote.error)}
            </p>
          ) : (
            <p className="text-xs text-muted">
              The task in{" "}
              <RouterLink
                to={docHref(task.doc_path)}
                className="hover:text-accent hover:underline"
              >
                {task.doc_title}
              </RouterLink>{" "}
              becomes a link to the note.
            </p>
          )}
          <button
            type="button"
            onClick={submit}
            disabled={title.trim() === "" || toNote.isPending}
            className="ml-auto h-7 shrink-0 rounded border border-border bg-accent px-2.5 text-xs font-medium text-white hover:opacity-90 disabled:opacity-50"
          >
            {toNote.isPending ? "Filing…" : "Make note"}
          </button>
        </div>
      </div>
    </Modal>
  );
}
