// The document page body, shared by /doc/* and /daily/<date>: read mode
// (rendered markdown, frontmatter properties strip, backlinks) and edit mode
// (CodeMirror with autosave + conflict banner). Callers key this component by
// path so all editing state resets on navigation.
import { Link as RouterLink } from "@tanstack/react-router";
import { AlertTriangle, FileQuestion, Pencil } from "lucide-react";
import {
  lazy,
  Suspense,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { ApiError } from "../api/client.ts";
import { useDocument, useToggleTask } from "../api/queries.ts";
import type { Document } from "../api/types.ts";
import { formatRelativeTime } from "../lib/dates.ts";
import { docHref, DOC_TYPE_INFO } from "../lib/docs.ts";
import { useUi } from "../keys/UiContext.tsx";
import type { MarkdownEditorHandle } from "../editor/MarkdownEditor.tsx";
import { EmptyState, ErrorState } from "./EmptyState.tsx";
import { Markdown } from "./Markdown.tsx";
import { SkeletonRows } from "./Skeleton.tsx";
import { useDocumentSave, type SaveStatus } from "./useDocumentSave.ts";

// CodeMirror is ~⅓ of the bundle and only needed once someone edits; keep it
// off the critical path (DESIGN.md's bundle budget).
const MarkdownEditor = lazy(() =>
  import("../editor/MarkdownEditor.tsx").then((module) => ({
    default: module.MarkdownEditor,
  })),
);

export function DocumentScreen({
  path,
  initialEdit = false,
  notFoundFallback,
}: {
  path: string;
  initialEdit?: boolean;
  /** Rendered on 404 instead of the default empty state (daily's create UI). */
  notFoundFallback?: ReactNode;
}) {
  const docQuery = useDocument(path);
  if (docQuery.isPending) return <SkeletonRows count={8} />;
  if (docQuery.isError) {
    if (docQuery.error instanceof ApiError && docQuery.error.status === 404) {
      if (notFoundFallback) return <>{notFoundFallback}</>;
      return (
        <EmptyState
          icon={FileQuestion}
          title="No such document"
          hint={`Nothing lives at ${path} (yet).`}
        />
      );
    }
    return <ErrorState error={docQuery.error} />;
  }
  return (
    <DocumentView path={path} doc={docQuery.data} initialEdit={initialEdit} />
  );
}

const STATUS_LABEL: Record<SaveStatus, { text: string; classes: string }> = {
  saved: { text: "saved", classes: "text-muted" },
  dirty: { text: "edited", classes: "text-muted" },
  saving: { text: "saving…", classes: "text-muted" },
  conflict: { text: "conflict", classes: "text-danger" },
  error: { text: "save failed", classes: "text-danger" },
};

function DocumentView({
  path,
  doc,
  initialEdit,
}: {
  path: string;
  doc: Document;
  initialEdit: boolean;
}) {
  const [mode, setMode] = useState<"read" | "edit">(
    initialEdit ? "edit" : "read",
  );
  const { registerKey, pushEscape } = useUi();
  const save = useDocumentSave(path, doc);
  const editorRef = useRef<MarkdownEditorHandle>(null);
  const toggleTask = useToggleTask();

  const exitEdit = () => {
    void save.save();
    setMode("read");
  };

  // `e` enters edit mode; Escape (outside the editor's focus) backs out of it.
  useEffect(() => {
    if (mode === "read") return registerKey("e", () => setMode("edit"));
    return pushEscape(exitEdit);
    // exitEdit identity is per-render but only mode changes matter here.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mode, registerKey, pushEscape]);

  // Tag pool for `#` autocomplete: everything this doc and its neighbors carry.
  const getTags = () => [
    ...doc.tags,
    ...doc.backlinks.flatMap((backlink) => backlink.tags),
    ...doc.tasks.flatMap((task) => task.tags),
  ];

  const typeInfo = DOC_TYPE_INFO[doc.type];
  return (
    <article className="flex min-h-full flex-col">
      <header className="mb-3 flex items-center gap-2 border-b border-border pb-3">
        <typeInfo.icon
          className="size-4 shrink-0 text-muted"
          aria-hidden="true"
        />
        <h1 className="min-w-0 truncate text-lg font-semibold text-heading">
          {doc.title}
        </h1>
        <span className="hidden font-mono text-[10px] uppercase text-muted sm:inline">
          {doc.type}
        </span>
        <span className="ml-auto hidden text-xs text-muted sm:inline">
          {formatRelativeTime(doc.mtime)}
        </span>
        {mode === "edit" ? (
          <span
            className={`font-mono text-[10px] ${STATUS_LABEL[save.status].classes}`}
          >
            {STATUS_LABEL[save.status].text}
          </span>
        ) : null}
        <button
          type="button"
          onClick={mode === "read" ? () => setMode("edit") : exitEdit}
          className="flex h-7 items-center gap-1.5 rounded border border-border px-2 text-xs text-body hover:bg-hover hover:text-heading"
        >
          <Pencil className="size-3" aria-hidden="true" />
          {mode === "read" ? "Edit" : "Done"}
        </button>
      </header>

      {save.status === "conflict" ? (
        <ConflictBanner
          onKeepMine={() => void save.keepMine()}
          onTakeDisk={() => {
            const disk = save.takeDisk();
            if (disk !== null) editorRef.current?.setValue(disk);
          }}
        />
      ) : null}
      {save.status === "error" && save.errorMessage ? (
        <p className="mb-3 border border-danger/40 px-3 py-2 text-xs text-danger">
          {save.errorMessage}
        </p>
      ) : null}

      {mode === "read" ? (
        <>
          <FrontmatterStrip frontmatter={doc.frontmatter} />
          <Markdown
            markdown={save.currentText()}
            links={doc.links}
            tasks={doc.tasks}
            onToggleTask={(task) => toggleTask.mutate(task)}
          />
          <Backlinks doc={doc} />
        </>
      ) : (
        <Suspense fallback={<SkeletonRows count={6} />}>
          <MarkdownEditor
            ref={editorRef}
            initialValue={save.currentText()}
            onChange={save.onEditorChange}
            onSave={() => void save.save()}
            onSaveAndExit={exitEdit}
            onBlur={() => void save.save()}
            getTags={getTags}
          />
        </Suspense>
      )}
    </article>
  );
}

function ConflictBanner({
  onKeepMine,
  onTakeDisk,
}: {
  onKeepMine: () => void;
  onTakeDisk: () => void;
}) {
  return (
    <div className="mb-3 flex flex-wrap items-center gap-2 border border-warn/50 bg-warn/5 px-3 py-2">
      <AlertTriangle className="size-4 shrink-0 text-warn" aria-hidden="true" />
      <p className="text-xs text-body">
        This file changed on disk while you were editing.
      </p>
      <div className="ml-auto flex gap-1.5">
        <button
          type="button"
          onClick={onKeepMine}
          className="h-6 rounded border border-border px-2 text-xs text-body hover:bg-hover"
        >
          Keep mine
        </button>
        <button
          type="button"
          onClick={onTakeDisk}
          className="h-6 rounded border border-border px-2 text-xs text-body hover:bg-hover"
        >
          Take disk
        </button>
      </div>
    </div>
  );
}

/** Frontmatter as a compact key:value chip strip (never rendered as markdown). */
function FrontmatterStrip({
  frontmatter,
}: {
  frontmatter: Record<string, unknown>;
}) {
  const entries = Object.entries(frontmatter).filter(
    ([key]) => key !== "title",
  );
  if (entries.length === 0) return null;
  return (
    <div className="mb-3 flex flex-wrap gap-1.5 border-b border-border pb-3">
      {entries.map(([key, value]) => (
        <span
          key={key}
          className="inline-flex max-w-full items-center gap-1 rounded border border-border bg-raised px-1.5 py-0.5 font-mono text-[10px]"
        >
          <span className="text-muted">{key}:</span>
          <span className="truncate text-body">
            {formatFrontmatterValue(value)}
          </span>
        </span>
      ))}
    </div>
  );
}

function formatFrontmatterValue(value: unknown): string {
  if (Array.isArray(value)) return value.map(String).join(", ");
  if (value === null || value === undefined) return "—";
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

function Backlinks({ doc }: { doc: Document }) {
  if (doc.backlinks.length === 0) return null;
  return (
    <section className="mt-8 border-t border-border pt-3">
      <h2 className="mb-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted">
        Linked from
      </h2>
      <ul className="divide-y divide-border">
        {doc.backlinks.map((backlink) => (
          <li key={backlink.path}>
            <RouterLink
              to={docHref(backlink.path)}
              className="flex h-8 items-center gap-2 text-sm text-body hover:bg-hover hover:text-heading"
            >
              <span className="truncate">{backlink.title}</span>
              <span className="ml-auto font-mono text-[10px] uppercase text-muted">
                {backlink.type}
              </span>
            </RouterLink>
          </li>
        ))}
      </ul>
    </section>
  );
}
