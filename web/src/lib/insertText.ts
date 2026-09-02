// "Put this text into the document" from somewhere that can't see the
// editor — the command palette inserting a drawing embed, say. If an editor
// is mounted it takes the text at its cursor and reports so; otherwise the
// caller falls back to appending through the API.
export interface InsertTextRequest {
  text: string;
  handled: boolean;
}

const EVENT = "quire:insert-text";

/** True when a mounted editor took the text. */
export function requestInsertText(text: string): boolean {
  const detail: InsertTextRequest = { text, handled: false };
  window.dispatchEvent(new CustomEvent<InsertTextRequest>(EVENT, { detail }));
  return detail.handled;
}

export function onInsertTextRequest(
  handler: (request: InsertTextRequest) => void,
): () => void {
  const listener = (event: Event) =>
    handler((event as CustomEvent<InsertTextRequest>).detail);
  window.addEventListener(EVENT, listener);
  return () => window.removeEventListener(EVENT, listener);
}
