// The buffer is what read mode renders, so whether it follows the server is
// a user-visible correctness question, not an implementation detail. Two
// bugs have already lived here: a toggled checkbox that updated the file but
// not the view, and an edit made outside the app never reaching an open page.
//
// The rule under test: adopt the server's version when there is nothing
// unsaved, never when there is.
import { describe, expect, test } from "bun:test";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook } from "@testing-library/react";
import { useDocumentSave } from "./useDocumentSave.ts";
import type { Document } from "../api/types.ts";

function docWith(markdown: string, sha: string): Document {
  return {
    path: "notes/x.md",
    type: "note",
    title: "X",
    mtime: "2026-09-01T10:00:00Z",
    sha256: sha,
    tags: [],
    markdown,
    frontmatter: {},
    links: [],
    backlinks: [],
    tasks: [],
  } as unknown as Document;
}

function wrapper({ children }: { children: React.ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

describe("useDocumentSave buffer sync", () => {
  test("a clean buffer follows the server", () => {
    // The external-edit case: vim rewrites the file, SSE refetches it, and
    // the open page must show the new text.
    const { result, rerender } = renderHook(
      ({ doc }) => useDocumentSave("notes/x.md", doc),
      { wrapper, initialProps: { doc: docWith("original\n", "sha-1") } },
    );
    expect(result.current.text).toBe("original\n");

    rerender({ doc: docWith("rewritten by vim\n", "sha-2") });

    expect(result.current.text).toBe("rewritten by vim\n");
  });

  test("unsaved edits are never clobbered", () => {
    const { result, rerender } = renderHook(
      ({ doc }) => useDocumentSave("notes/x.md", doc),
      { wrapper, initialProps: { doc: docWith("original\n", "sha-1") } },
    );

    act(() => result.current.onEditorChange("my unsaved work\n"));
    // A server change arrives while the buffer is dirty.
    rerender({ doc: docWith("someone else's version\n", "sha-2") });

    expect(result.current.text).not.toBe("someone else's version\n");
    expect(result.current.currentText()).toBe("my unsaved work\n");
  });

  test("an unchanged sha is not treated as a server change", () => {
    const { result, rerender } = renderHook(
      ({ doc }) => useDocumentSave("notes/x.md", doc),
      { wrapper, initialProps: { doc: docWith("original\n", "sha-1") } },
    );
    rerender({ doc: docWith("original\n", "sha-1") });
    expect(result.current.text).toBe("original\n");
    expect(result.current.status).toBe("saved");
  });
});
