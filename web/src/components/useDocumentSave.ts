// Autosave state machine for the editor, and the buffer both editor and read
// mode render. One PUT per burst: every change arms a 2s idle timer; Cmd+S /
// Cmd+Enter / blur flush immediately; concurrent requests are single-flighted
// with a queued re-save. Every write carries base_sha256 — a 409 freezes
// autosave and surfaces the conflict for an explicit keep-mine / take-disk
// decision (no auto-merge, per DESIGN.md).
import { useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useRef, useState } from "react";
import { api, isConflictError } from "../api/client.ts";
import { queryKeys } from "../api/queries.ts";
import type { Document } from "../api/types.ts";

const IDLE_SAVE_MS = 2_000;

export type SaveStatus = "saved" | "dirty" | "saving" | "conflict" | "error";

export interface DocumentSave {
  status: SaveStatus;
  /** The server's version of the file when a 409 hit; null otherwise. */
  conflictDoc: Document | null;
  errorMessage: string | null;
  /** Wire to the editor's onChange. */
  onEditorChange: (text: string) => void;
  /** Flush now (Cmd+S, blur, exiting edit mode). */
  save: () => Promise<void>;
  /** Conflict: overwrite the disk version with the editor's text. */
  keepMine: () => Promise<void>;
  /** Conflict: adopt the disk version; returns its text for the editor. */
  takeDisk: () => string | null;
  /**
   * The current buffer: what read mode renders and what the next save sends.
   * While nothing is unsaved this IS the server's version, so a file changed
   * underneath — a task toggle, or vim in another window — shows up without
   * any syncing step.
   */
  text: string;
  /** Latest buffer text, readable from callbacks without a stale closure. */
  currentText: () => string;
}

export function useDocumentSave(path: string, doc: Document): DocumentSave {
  const queryClient = useQueryClient();
  const [status, setStatus] = useState<SaveStatus>("saved");
  const [conflictDoc, setConflictDoc] = useState<Document | null>(null);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  // publishedText only matters while the buffer is dirty; see `text` below.
  const [publishedText, setPublishedText] = useState(doc.markdown);

  const textRef = useRef(doc.markdown);
  const savedTextRef = useRef(doc.markdown);
  const baseShaRef = useRef(doc.sha256);
  const inFlightRef = useRef(false);
  const queuedRef = useRef(false);
  const conflictRef = useRef(false);
  const idleTimerRef = useRef<ReturnType<typeof setTimeout> | undefined>(
    undefined,
  );

  // "saved" is precisely the state where the buffer matches the last write
  // with nothing in flight or conflicted.
  const clean = status === "saved";

  // While the buffer is clean it *is* the server's version — derived, not a
  // copy kept in sync. The previous design stored a copy and refreshed it
  // with a render-phase setState; that update was silently lost on a later
  // render, so an edit made outside the app (or a task toggle) refetched
  // correctly and still showed the old text on screen. Deriving makes the
  // invariant structural: there is no second copy to go stale.
  const text = clean ? doc.markdown : publishedText;

  // The save machinery works from refs so typing does not re-render. Point
  // them at the server's version whenever the buffer is clean, so the next
  // write carries the right base sha.
  useEffect(() => {
    if (!clean) return;
    textRef.current = doc.markdown;
    savedTextRef.current = doc.markdown;
    baseShaRef.current = doc.sha256;
    setPublishedText(doc.markdown);
  }, [clean, doc.markdown, doc.sha256]);

  useEffect(() => () => clearTimeout(idleTimerRef.current), []);

  const doSave = useCallback(
    async (overrideSha?: string): Promise<void> => {
      if (inFlightRef.current) {
        queuedRef.current = true;
        return;
      }
      // A 409 freezes saving until the user chooses. onEditorChange already
      // respects that, but blur and Cmd+S reach here directly — and clicking
      // "Keep mine" blurs the editor, so without this the resolution was
      // immediately preceded by an ordinary save carrying the same stale
      // base sha, which conflicted again and swallowed the choice. Only an
      // explicit override (keepMine/takeDisk) may write during a conflict.
      if (conflictRef.current && !overrideSha) return;
      const text = textRef.current;
      if (text === savedTextRef.current && !overrideSha) {
        setStatus("saved");
        return;
      }
      inFlightRef.current = true;
      setStatus("saving");
      try {
        const saved = await api.putDocument(
          path,
          text,
          overrideSha ?? baseShaRef.current,
        );
        baseShaRef.current = saved.sha256;
        savedTextRef.current = text;
        conflictRef.current = false;
        setConflictDoc(null);
        queryClient.setQueryData(queryKeys.document(path), saved);
        setStatus(textRef.current === text ? "saved" : "dirty");
      } catch (error) {
        // Before believing a failure, ask the server what it holds. A
        // response lost in transit (a tunnel hiccup) leaves the write
        // applied with the client none the wiser; its retry then carries
        // the old base sha and "conflicts" with its own save. Left there,
        // read mode keeps showing this buffer and ignores every refetch —
        // task toggles hit the disk and never appear. If the server has
        // exactly this text, the save landed and nothing is wrong.
        const server = await api.getDocument(path).catch(() => null);
        if (server && server.markdown === text) {
          baseShaRef.current = server.sha256;
          savedTextRef.current = text;
          conflictRef.current = false;
          setConflictDoc(null);
          setErrorMessage(null);
          queryClient.setQueryData(queryKeys.document(path), server);
          setStatus(textRef.current === text ? "saved" : "dirty");
        } else if (isConflictError(error)) {
          conflictRef.current = true;
          queuedRef.current = false;
          setConflictDoc(server);
          setStatus("conflict");
        } else {
          setErrorMessage(
            error instanceof Error ? error.message : "Save failed",
          );
          setStatus("error");
        }
      } finally {
        inFlightRef.current = false;
        if (queuedRef.current && !conflictRef.current) {
          queuedRef.current = false;
          void doSave();
        }
      }
    },
    [path, queryClient],
  );

  const onEditorChange = useCallback(
    (text: string) => {
      textRef.current = text;
      if (conflictRef.current) return; // Frozen until the user resolves.
      setStatus((current) => (current === "saving" ? current : "dirty"));
      clearTimeout(idleTimerRef.current);
      idleTimerRef.current = setTimeout(() => void doSave(), IDLE_SAVE_MS);
    },
    [doSave],
  );

  const save = useCallback(() => {
    clearTimeout(idleTimerRef.current);
    // Publish the buffer for read mode now rather than after the round-trip;
    // per-keystroke setState would re-render the whole page for nothing.
    setPublishedText(textRef.current);
    return doSave();
  }, [doSave]);

  const keepMine = useCallback(async () => {
    const server =
      conflictDoc ?? (await api.getDocument(path).catch(() => null));
    if (!server) return;
    conflictRef.current = false;
    await doSave(server.sha256);
  }, [conflictDoc, doSave, path]);

  const takeDisk = useCallback((): string | null => {
    if (!conflictDoc) return null;
    textRef.current = conflictDoc.markdown;
    savedTextRef.current = conflictDoc.markdown;
    baseShaRef.current = conflictDoc.sha256;
    conflictRef.current = false;
    queryClient.setQueryData(queryKeys.document(path), conflictDoc);
    setConflictDoc(null);
    setPublishedText(conflictDoc.markdown);
    setStatus("saved");
    return conflictDoc.markdown;
  }, [conflictDoc, path, queryClient]);

  const currentText = useCallback(() => textRef.current, []);

  return {
    status,
    conflictDoc,
    errorMessage,
    onEditorChange,
    save,
    keepMine,
    takeDisk,
    text,
    currentText,
  };
}
