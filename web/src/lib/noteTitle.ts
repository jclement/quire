// The default title when a task becomes a note — the same rule as the
// server's service.NoteTitleFrom, so the dialog proposes exactly what an
// agent calling task_to_note without a title would get.

/** How many words of the task become the proposed title. */
const TITLE_WORDS = 8;

/** First eight words, links read as their display text, tags and inline
 * markup dropped, trailing punctuation trimmed. */
export function noteTitleFrom(text: string): string {
  const plain = text
    .replace(
      /\[\[([^[\]|]+)(?:\|([^[\]]+))?\]\]/g,
      (_link, target: string, alias?: string) => alias ?? target,
    )
    .replace(/(^|\s)#\p{L}[\p{L}\p{N}_/-]*/gu, "")
    .replaceAll("**", "")
    .replaceAll("`", "")
    .replaceAll("==", "");
  const words = plain.split(/\s+/).filter(Boolean).slice(0, TITLE_WORDS);
  return words.join(" ").replace(/[.,;:!?—-]+$/, "");
}
