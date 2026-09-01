// Rename/move dialog: edits a document's vault path, always rewriting inbound
// links (the backlink count warns how many files will change). A 409 means the
// target path already exists and is shown inline. On success: toast with the
// rewrite count and navigate to the new path.
import { useNavigate } from "@tanstack/react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { FolderPen } from "lucide-react";
import { useState } from "react";
import { api, isConflictError } from "../api/client.ts";
import { queryKeys, useDocument } from "../api/queries.ts";
import { docHref } from "../lib/docs.ts";
import { noAutofill } from "../lib/noAutofill.ts";
import { useUi } from "../keys/UiContext.tsx";
import { Modal } from "./Modal.tsx";

export function RenameDialog() {
  const { renameDocPath, setRenameDocPath } = useUi();
  const close = () => setRenameDocPath(null);
  return (
    <Modal
      open={renameDocPath !== null}
      onClose={close}
      variant="center"
      label="Rename document"
    >
      {renameDocPath !== null ? (
        <RenameForm path={renameDocPath} close={close} />
      ) : null}
    </Modal>
  );
}

function RenameForm({ path, close }: { path: string; close: () => void }) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { toast } = useUi();
  const [newPath, setNewPath] = useState(path);
  // Cache hit when opened from the doc page; a background fetch otherwise.
  const doc = useDocument(path);
  const backlinkCount = doc.data?.backlinks.length ?? 0;

  const rename = useMutation({
    mutationFn: () => api.rename(path, newPath.trim(), true),
    onSuccess: (result) => {
      // The old path's caches are now lies; drop them rather than refetch 404s.
      queryClient.removeQueries({ queryKey: queryKeys.document(path) });
      void queryClient.invalidateQueries({ queryKey: ["documents"] });
      void queryClient.invalidateQueries({ queryKey: ["tasks"] });
      void queryClient.invalidateQueries({ queryKey: queryKeys.today });
      queryClient.setQueryData(
        queryKeys.document(result.document.path),
        result.document,
      );
      toast(
        result.rewritten.length === 0
          ? "Renamed"
          : `Renamed — ${result.rewritten.length} ${
              result.rewritten.length === 1 ? "document" : "documents"
            } updated`,
      );
      close();
      void navigate({ to: docHref(result.document.path), replace: true });
    },
  });

  const unchanged = newPath.trim() === path || newPath.trim() === "";
  const submit = () => {
    if (unchanged || rename.isPending) return;
    rename.mutate();
  };

  return (
    <div className="flex flex-col">
      <div className="flex items-center gap-2 border-b border-border px-3">
        <FolderPen className="size-4 shrink-0 text-muted" aria-hidden="true" />
        <input
          autoFocus
          value={newPath}
          onChange={(event) => setNewPath(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              event.preventDefault();
              submit();
            }
          }}
          aria-label="New path"
          {...noAutofill("rename-path")}
          className="h-11 w-full bg-transparent font-mono text-xs text-heading outline-none placeholder:text-muted"
        />
      </div>
      <div className="flex items-center gap-2 px-3 py-2">
        {rename.isError ? (
          <p className="text-xs text-danger">
            {isConflictError(rename.error)
              ? "That path already exists — pick another."
              : `Couldn't rename — ${rename.error.message}`}
          </p>
        ) : (
          <p className="text-xs text-muted">
            {backlinkCount === 0
              ? "Nothing links here."
              : `${backlinkCount} ${
                  backlinkCount === 1 ? "document links" : "documents link"
                } here — links will be updated.`}
          </p>
        )}
        <button
          type="button"
          onClick={submit}
          disabled={unchanged || rename.isPending}
          className="ml-auto h-7 shrink-0 rounded border border-border bg-accent px-2.5 text-xs font-medium text-white hover:opacity-90 disabled:opacity-50"
        >
          {rename.isPending ? "Renaming…" : "Rename"}
        </button>
      </div>
    </div>
  );
}
