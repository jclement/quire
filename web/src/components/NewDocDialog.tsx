// Palette-style create dialog: a title for a new document of a preset type
// (opened by palette "New X" commands or a browse page's New button — state
// lives in UiContext so it survives the palette closing). The server picks the
// path; on success we navigate straight into the new document. The form mounts
// fresh per open.
import { isRealArea } from "../lib/area.ts";
import { useNavigate } from "@tanstack/react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client.ts";
import type { DocType } from "../api/types.ts";
import { docHref, DOC_TYPE_INFO } from "../lib/docs.ts";
import { noAutofill } from "../lib/noAutofill.ts";
import { useUi } from "../keys/UiContext.tsx";
import { Modal } from "./Modal.tsx";

export function NewDocDialog() {
  const { newDocType, setNewDocType } = useUi();
  const close = () => setNewDocType(null);
  return (
    <Modal
      open={newDocType !== null}
      onClose={close}
      variant="center"
      label={`New ${newDocType ? DOC_TYPE_INFO[newDocType].label : "document"}`}
    >
      {newDocType !== null ? (
        <NewDocForm type={newDocType} close={close} />
      ) : null}
    </Modal>
  );
}

function NewDocForm({ type, close }: { type: DocType; close: () => void }) {
  const navigate = useNavigate();
  const { area } = useUi();
  const queryClient = useQueryClient();
  const [title, setTitle] = useState("");

  const create = useMutation({
    // A document made while looking at Work is a Work document.
    mutationFn: (input: { title: string }) =>
      api.createDocument(
        type,
        input.title,
        undefined,
        isRealArea(area) ? area : undefined,
      ),
    onSuccess: (doc) => {
      void queryClient.invalidateQueries({ queryKey: ["documents"] });
      close();
      void navigate({ to: docHref(doc.path) });
    },
  });

  const info = DOC_TYPE_INFO[type];
  return (
    <>
      <div className="flex items-center gap-2 border-b border-border px-3 focus-within:border-accent">
        <info.icon className="size-4 shrink-0 text-muted" aria-hidden="true" />
        <input
          autoFocus
          value={title}
          onChange={(event) => setTitle(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter" && title.trim() && !create.isPending) {
              event.preventDefault();
              create.mutate({ title: title.trim() });
            }
          }}
          placeholder={`${info.label} title…`}
          aria-label={`New ${info.label} title`}
          {...noAutofill("new-doc-title")}
          className="field-bare h-11 w-full bg-transparent text-sm text-heading outline-none placeholder:text-muted"
        />
      </div>
      <p className="px-3 py-2 text-xs text-muted">
        {create.isError
          ? `Couldn't create — ${create.error.message}`
          : "↵ to create and open"}
      </p>
    </>
  );
}
