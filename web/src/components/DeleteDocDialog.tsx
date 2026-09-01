// Delete confirmation for a document. Destructive and therefore always a
// two-step: the dialog names the file, says plainly that the markdown leaves
// the vault, and only then calls DELETE. On success: toast, drop the caches
// that referenced it, and leave the page (back if we came from somewhere in
// the app, else Today).
import { useNavigate } from "@tanstack/react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Trash2 } from "lucide-react";
import { api, errorMessage } from "../api/client.ts";
import { queryKeys, useDocument } from "../api/queries.ts";
import { useUi } from "../keys/UiContext.tsx";
import { Modal } from "./Modal.tsx";

export function DeleteDocDialog() {
  const { deleteDocPath, setDeleteDocPath } = useUi();
  const close = () => setDeleteDocPath(null);
  return (
    <Modal
      open={deleteDocPath !== null}
      onClose={close}
      variant="center"
      label="Delete document"
    >
      {deleteDocPath !== null ? (
        <DeleteConfirm path={deleteDocPath} close={close} />
      ) : null}
    </Modal>
  );
}

function DeleteConfirm({ path, close }: { path: string; close: () => void }) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { toast } = useUi();
  // Cached when opened from the document page; the path is the fallback title.
  const doc = useDocument(path);
  const title = doc.data?.title ?? path;

  const remove = useMutation({
    mutationFn: () => api.deleteDocument(path),
    onSuccess: () => {
      queryClient.removeQueries({ queryKey: queryKeys.document(path) });
      void queryClient.invalidateQueries({ queryKey: ["documents"] });
      void queryClient.invalidateQueries({ queryKey: ["search"] });
      void queryClient.invalidateQueries({ queryKey: ["tasks"] });
      void queryClient.invalidateQueries({ queryKey: queryKeys.today });
      toast(`Deleted ${title}`);
      close();
      // history.length > 1 means we arrived from somewhere inside the app.
      if (window.history.length > 1) window.history.back();
      else void navigate({ to: "/today" });
    },
  });

  return (
    <div className="flex flex-col">
      <div className="flex items-start gap-2.5 border-b border-border px-3 py-3">
        <AlertTriangle
          className="mt-0.5 size-4 shrink-0 text-danger"
          aria-hidden="true"
        />
        <div className="min-w-0">
          <h2 className="text-sm font-semibold text-heading">
            Delete {title}?
          </h2>
          <p className="mt-1 text-xs text-muted">
            This removes <span className="font-mono text-body">{path}</span>{" "}
            from the vault. Tasks and backlinks in other notes will point at
            nothing. If your vault is a git repository the file is recoverable
            from history — otherwise this is permanent.
          </p>
        </div>
      </div>
      <div className="flex items-center gap-2 px-3 py-2">
        {remove.isError ? (
          <p className="text-xs text-danger">{errorMessage(remove.error)}</p>
        ) : null}
        <button
          type="button"
          onClick={close}
          className="ml-auto h-8 shrink-0 rounded border border-border px-2.5 text-xs text-body hover:bg-hover hover:text-heading"
        >
          Cancel
        </button>
        <button
          type="button"
          onClick={() => remove.mutate()}
          disabled={remove.isPending}
          className="flex h-8 shrink-0 items-center gap-1.5 rounded border border-danger bg-danger px-2.5 text-xs font-medium text-white hover:opacity-90 disabled:opacity-50"
        >
          <Trash2 className="size-3.5" aria-hidden="true" />
          {remove.isPending ? "Deleting…" : "Delete"}
        </button>
      </div>
    </div>
  );
}
