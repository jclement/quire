// Messages for the Unwritten page's Add alias action. The server's generic
// 409 text is written for the editor ("reload and reapply your edit"), which
// means nothing on a row that has no editor; this says what happened here.
import { errorMessage, isConflictError } from "../api/client.ts";

/** Why adding `alias` to the page titled `title` failed, in the row's terms. */
export function addAliasErrorMessage(
  error: unknown,
  alias: string,
  title: string,
): string {
  if (isConflictError(error)) {
    return `${title} changed while “${alias}” was being added — nothing was written. Try again.`;
  }
  return errorMessage(error);
}
