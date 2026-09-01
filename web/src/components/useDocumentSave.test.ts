// Regression tests for the buffer-vs-server decision in useDocumentSave.
//
// The bug this guards: read mode renders the save buffer, which used to be
// seeded once from the document and never updated. Toggling a task checkbox in
// rendered markdown wrote `- [x] … ✅ …` to disk and refetched, but the buffer
// still held the old text, so the checkbox never appeared to flip. The risky
// half of the fix is the other direction — adopting must never eat unsaved
// edits, and must stay out of the way during a conflict.
import { describe, expect, test } from "bun:test";
import { shouldAdoptServerVersion } from "./useDocumentSave.ts";

const UNCHECKED = "- [ ] Send Sarah the diagram\n";
const CHECKED = "- [x] Send Sarah the diagram ✅ 2026-09-01\n";

describe("shouldAdoptServerVersion", () => {
  test("adopts a server-side task toggle when the buffer is clean", () => {
    expect(
      shouldAdoptServerVersion({
        serverMarkdown: CHECKED,
        bufferText: UNCHECKED,
        bufferIsClean: true,
        inConflict: false,
      }),
    ).toBe(true);
  });

  test("never clobbers unsaved local edits", () => {
    expect(
      shouldAdoptServerVersion({
        serverMarkdown: CHECKED,
        bufferText: `${UNCHECKED}My in-progress sentence`,
        bufferIsClean: false,
        inConflict: false,
      }),
    ).toBe(false);
  });

  test("stays out of the way while a conflict is unresolved", () => {
    // Even with a clean buffer: the user is mid-decision on keep-mine /
    // take-disk, and takeDisk owns the adoption in that path.
    expect(
      shouldAdoptServerVersion({
        serverMarkdown: CHECKED,
        bufferText: UNCHECKED,
        bufferIsClean: true,
        inConflict: true,
      }),
    ).toBe(false);
  });

  test("no-ops when the server text already matches the buffer", () => {
    // Our own save echoes back identical markdown — adopting would be a
    // pointless re-render.
    expect(
      shouldAdoptServerVersion({
        serverMarkdown: CHECKED,
        bufferText: CHECKED,
        bufferIsClean: true,
        inConflict: false,
      }),
    ).toBe(false);
  });

  test("adopts when the buffer was saved and the server then moved on", () => {
    // Saved our edit, then an external change (git pull, another tab) arrived.
    expect(
      shouldAdoptServerVersion({
        serverMarkdown: `${CHECKED}Added elsewhere\n`,
        bufferText: CHECKED,
        bufferIsClean: true,
        inConflict: false,
      }),
    ).toBe(true);
  });
});
