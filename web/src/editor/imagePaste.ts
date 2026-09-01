// Paste-an-image support: inserts a placeholder at the cursor immediately (no
// dialog, per DESIGN.md), uploads via POST /attachments, then swaps the
// placeholder for the server's ready-to-insert markdown — wherever it has
// moved to in the meantime. Upload failure turns the placeholder into a
// visible error comment rather than silently vanishing content.
import { EditorView } from "@codemirror/view";
import { api } from "../api/client.ts";

let placeholderCounter = 0;

/** Finds the placeholder wherever it now lives and replaces it. */
function replacePlaceholder(
  view: EditorView,
  placeholder: string,
  replacement: string,
): void {
  const text = view.state.doc.toString();
  const at = text.indexOf(placeholder);
  if (at === -1) return; // User deleted it; drop the result quietly.
  view.dispatch({
    changes: { from: at, to: at + placeholder.length, insert: replacement },
  });
}

function uploadPastedImage(view: EditorView, file: File): void {
  placeholderCounter += 1;
  const placeholder = `![uploading-${placeholderCounter}...]()`;
  view.dispatch(view.state.replaceSelection(placeholder));
  api
    .uploadAttachment(file)
    .then((upload) => replacePlaceholder(view, placeholder, upload.markdown))
    .catch(() => {
      replacePlaceholder(view, placeholder, "<!-- image upload failed -->");
    });
}

export const imagePasteHandler = EditorView.domEventHandlers({
  paste: (event, view) => {
    const files = [...(event.clipboardData?.files ?? [])].filter((file) =>
      file.type.startsWith("image/"),
    );
    if (files.length === 0) return false;
    event.preventDefault();
    for (const file of files) uploadPastedImage(view, file);
    return true;
  },
});
