// Palette-style create dialog: a title for a new document of a preset type
// (opened by palette "New X" commands or a browse page's New button — state
// lives in UiContext so it survives the palette closing). The server picks the
// path; on success we navigate straight into the new document.
import { useNavigate } from "@tanstack/react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { api } from "../api/client.ts";
import { docHref, DOC_TYPE_INFO } from "../lib/docs.ts";
import { useUi } from "../keys/UiContext.tsx";
import { Modal } from "./Modal.tsx";

export function NewDocDialog() {
  const { newDocType, setNewDocType } = useUi();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [title, setTitle] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  const create = useMutation({
    mutationFn: (input: { title: string }) =>
      api.createDocument(newDocType ?? "note", input.title),
    onSuccess: (doc) => {
      void queryClient.invalidateQueries({ queryKey: ["documents"] });
      setNewDocType(null);
      void navigate({ to: docHref(doc.path) });
    },
  });

  useEffect(() => {
    if (newDocType !== null) {
      setTitle("");
      create.reset();
      requestAnimationFrame(() => inputRef.current?.focus());
    }
    // reset() is stable; re-running on open only is intentional.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [newDocType]);

  const info = newDocType ? DOC_TYPE_INFO[newDocType] : null;

  return (
    <Modal
      open={newDocType !== null}
      onClose={() => setNewDocType(null)}
      variant="center"
      label={`New ${info?.label ?? "document"}`}
    >
      <div className="flex items-center gap-2 border-b border-border px-3">
        {info ? <info.icon className="size-4 shrink-0 text-muted" aria-hidden="true" /> : null}
        <input
          ref={inputRef}
          value={title}
          onChange={(event) => setTitle(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter" && title.trim() && !create.isPending) {
              event.preventDefault();
              create.mutate({ title: title.trim() });
            }
          }}
          placeholder={`${info?.label ?? "Document"} title…`}
          aria-label={`New ${info?.label ?? "document"} title`}
          className="h-11 w-full bg-transparent text-sm text-heading outline-none placeholder:text-muted"
        />
      </div>
      <p className="px-3 py-2 text-xs text-muted">
        {create.isError
          ? `Couldn't create — ${create.error.message}`
          : "↵ to create and open"}
      </p>
    </Modal>
  );
}
