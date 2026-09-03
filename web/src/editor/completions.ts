// Autocomplete sources for the editor: `[[` completes against the document
// index (with a create-if-missing entry that POSTs a new note before linking),
// and `#` completes tags from whatever the caller has seen recently.
import type {
  Completion,
  CompletionContext,
  CompletionResult,
} from "@codemirror/autocomplete";
import type { EditorView } from "@codemirror/view";
import { api } from "../api/client.ts";

const WIKILINK_LIMIT = 10;

/** Inserts "Title]]" over the typed query (the "[[" is already in the doc). */
function wikilinkCompletion(title: string, detail: string): Completion {
  return { label: title, detail, apply: `${title}]]` };
}

/** Creates the missing note first, then links to the title the server settled
 * on — so the link resolves immediately. Falls back to a plain link if the
 * backend is unreachable (it will dangle until the note exists). */
function createNoteCompletion(
  query: string,
  area: string | undefined,
): Completion {
  return {
    label: `Create "${query}"`,
    detail: "new note",
    apply: (view: EditorView, _completion, from: number, to: number) => {
      void api
        .createDocument("note", query, undefined, area)
        .then((doc) => doc.title)
        .catch(() => query)
        .then((title) => {
          view.dispatch({
            changes: { from, to, insert: `${title}]]` },
            selection: { anchor: from + title.length + 2 },
          });
        });
    },
  };
}

/**
 * `[[` completion bound to the area new notes should file under (the one
 * being looked at, when there is exactly one).
 */
export function makeWikilinkSource(getArea: () => string | undefined) {
  return (context: CompletionContext) => wikilinkSource(context, getArea());
}

async function wikilinkSource(
  context: CompletionContext,
  area: string | undefined,
): Promise<CompletionResult | null> {
  const match = context.matchBefore(/\[\[([^\][]*)$/);
  if (!match) return null;
  const query = match.text.slice(2);
  const from = match.from + 2;

  let options: Completion[] = [];
  try {
    const docs = await api.listDocuments({ q: query, limit: WIKILINK_LIMIT });
    options = docs.map((doc) => wikilinkCompletion(doc.title, doc.type));
  } catch {
    // Backend down: still offer the raw-text entry below.
  }
  const exactExists = options.some(
    (option) => option.label.toLowerCase() === query.toLowerCase(),
  );
  if (query.trim() && !exactExists)
    options.push(createNoteCompletion(query.trim(), area));
  if (options.length === 0) return null;
  // filter:false — the server already matched; keep its (and our) order.
  return { from, options, filter: false };
}

/** `#` tag completion over an app-provided tag list (recent docs). */
export function makeTagSource(getTags: () => string[]) {
  return (context: CompletionContext): CompletionResult | null => {
    const match = context.matchBefore(/(?:^|[\s(])#[\w/-]*$/);
    if (!match) return null;
    const hashAt = match.text.indexOf("#");
    const tags = [...new Set(getTags())];
    if (tags.length === 0) return null;
    return {
      from: match.from + hashAt + 1,
      options: tags.map((tag) => ({ label: tag, type: "keyword" })),
      validFor: /^[\w/-]*$/,
    };
  };
}
