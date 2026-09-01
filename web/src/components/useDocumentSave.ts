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

export interface BufferSyncInput {
  /** Markdown the server most recently returned for this path. */
  serverMarkdown: string;
  /** What the buffer holds right now. */
  bufferText: string;
  /** False when the buffer holds edits that have not reached the server. */
  bufferIsClean: boolean;
  /** True while a 409 is awaiting the user's keep-mine / take-disk choice. */
  inConflict: boolean;
}

/**
 * Whether to replace the buffer with the server's newer markdown.
 *
 * The server changes a file under us routinely — toggling a rendered task
 * checkbox rewrites one line, and SSE/refetch brings it back. Read mode renders
 * this buffer, so without adopting those changes the checkbox never appears to
 * flip. Unsaved local edits always win: they are never clobbered, and a genuine
 * divergence surfaces as a 409 on the next save instead.
 */
export function shouldAdoptServerVersion(input: BufferSyncInput): boolean {
  if (input.inConflict) return false;
  if (!input.bufferIsClean) return false;
  return input.serverMarkdown !== input.bufferText;
}

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
   * Tracks the server when there is nothing unsaved (see
   * shouldAdoptServerVersion), so server-side edits like a task toggle appear
   * immediately.
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
  const [text, setText] = useState(doc.markdown);

  const textRef = useRef(doc.markdown);
  const savedTextRef = useRef(doc.markdown);
  const baseShaRef = useRef(doc.sha256);
  const inFlightRef = useRef(false);
  const queuedRef = useRef(false);
  const conflictRef = useRef(false);
  const idleTimerRef = useRef<ReturnType<typeof setTimeout> | undefined>(
    undefined,
  );

  // Adopt server-side changes during render (React's "adjust state when a prop
  // changes" pattern) so the very first frame after a refetch already shows
  // them — an effect would render one stale frame first.
  const [syncedSha, setSyncedSha] = useState(doc.sha256);
  if (doc.sha256 !== syncedSha) {
    setSyncedSha(doc.sha256);
    // "saved" is precisely the state where the buffer matches the last write
    // and nothing is in flight or conflicted — so it is the clean signal, and
    // reading it (rather than the refs) keeps this check pure state.
    const adopt = shouldAdoptServerVersion({
      serverMarkdown: doc.markdown,
      bufferText: text,
      bufferIsClean: status === "saved",
      inConflict: status === "conflict",
    });
    if (adopt) {
      setText(doc.markdown);
      // Mirror into the save machinery's refs so the next write carries the
      // right base sha. Idempotent: guarded by the sha comparison above.
      textRef.current = doc.markdown;
      savedTextRef.current = doc.markdown;
      baseShaRef.current = doc.sha256;
    }
  }

  useEffect(() => () => clearTimeout(idleTimerRef.current), []);

  const doSave = useCallback(
    async (overrideSha?: string): Promise<void> => {
      if (inFlightRef.current) {
        queuedRef.current = true;
        return;
      }
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
        // Our own write is already in the buffer; record its sha so the
        // render-phase sync doesn't treat it as an incoming server change.
        setSyncedSha(saved.sha256);
        queryClient.setQueryData(queryKeys.document(path), saved);
        setStatus(textRef.current === text ? "saved" : "dirty");
      } catch (error) {
        if (isConflictError(error)) {
          conflictRef.current = true;
          queuedRef.current = false;
          const server = await api.getDocument(path).catch(() => null);
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
    setText(textRef.current);
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
    setText(conflictDoc.markdown);
    setSyncedSha(conflictDoc.sha256);
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
