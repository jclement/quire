// Autosave state machine for the editor. One PUT per burst: every change arms
// a 2s idle timer; Cmd+S / Cmd+Enter / blur flush immediately; concurrent
// requests are single-flighted with a queued re-save. Every write carries
// base_sha256 — a 409 freezes autosave and surfaces the conflict for an
// explicit keep-mine / take-disk decision (no auto-merge, per DESIGN.md).
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
  /** Latest editor text (for re-entering edit mode without losing edits). */
  currentText: () => string;
}

export function useDocumentSave(path: string, doc: Document): DocumentSave {
  const queryClient = useQueryClient();
  const [status, setStatus] = useState<SaveStatus>("saved");
  const [conflictDoc, setConflictDoc] = useState<Document | null>(null);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  const textRef = useRef(doc.markdown);
  const savedTextRef = useRef(doc.markdown);
  const baseShaRef = useRef(doc.sha256);
  const inFlightRef = useRef(false);
  const queuedRef = useRef(false);
  const conflictRef = useRef(false);
  const idleTimerRef = useRef<ReturnType<typeof setTimeout> | undefined>(
    undefined,
  );

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
    currentText,
  };
}
