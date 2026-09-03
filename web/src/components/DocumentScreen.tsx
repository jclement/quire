// The document page body, shared by /doc/* and /daily/<date>: read mode
// (rendered markdown, frontmatter properties strip, backlinks), edit mode
// (CodeMirror with autosave + conflict banner), and a desktop split view
// (editor left, live preview right). Cmd+E cycles the modes; Share/Rename
// actions live in the header. Callers key this component by path so all
// editing state resets on navigation.
import { Link as RouterLink } from "@tanstack/react-router";
import {
  AlertTriangle,
  FileQuestion,
  FolderPen,
  HelpCircle,
  MoreHorizontal,
  Pencil,
  Printer,
  Share2,
  Trash2,
  PenTool,
} from "lucide-react";
import {
  lazy,
  Suspense,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
  useCallback,
} from "react";
import { flushSync } from "react-dom";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { api, ApiError, errorMessage } from "../api/client.ts";
import {
  queryKeys,
  useDocument,
  useToggleTask,
  useRelated,
  useSemanticEnabled,
  useShares,
  useDefaultArea,
} from "../api/queries.ts";
import type { Document } from "../api/types.ts";
import { formatRelativeTime } from "../lib/dates.ts";
import { docHref, DOC_TYPE_INFO } from "../lib/docs.ts";
import { useDebouncedValue } from "../lib/useDebouncedValue.ts";
import { useUi } from "../keys/UiContext.tsx";
import type { MarkdownEditorHandle } from "../editor/MarkdownEditor.tsx";
import type { EditorContext } from "../editor/commands.ts";
import type { StripSync } from "../api/queries.ts";
import { frontmatterLines, splitFrontmatter } from "../lib/frontmatter.ts";
import { EditorToolbar } from "./EditorToolbar.tsx";
import { extractHeadings } from "../lib/headings.ts";
import { requestTableEdit } from "../lib/tableEditor.ts";
import { insertDrawingInto } from "../lib/drawings.ts";
import { preferredEditMode, storeEditMode } from "../lib/viewMode.ts";
import { findTables } from "../lib/tables.ts";
import { printPage, registerPrintHook } from "../lib/printing.ts";
import { DocumentRail } from "./DocumentRail.tsx";
import { EmptyState, ErrorState } from "./EmptyState.tsx";
import { FrontmatterStrip } from "./FrontmatterStrip.tsx";
import { Markdown } from "./Markdown.tsx";
import { SkeletonRows } from "./Skeleton.tsx";
import { useDocumentSave, type SaveStatus } from "./useDocumentSave.ts";

/** Split preview re-renders this long after the last keystroke. */
const PREVIEW_DEBOUNCE_MS = 300;

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

type ViewMode = "read" | "edit" | "split";

function DocumentView({
  path,
  doc,
  initialEdit,
}: {
  path: string;
  doc: Document;
  initialEdit: boolean;
}) {
  // A document opened for editing lands in whichever of Edit / Split the
  // person used last; every switch into one of them updates that memory.
  const [mode, setMode] = useState<ViewMode>(() =>
    initialEdit ? preferredEditMode() : "read",
  );
  useEffect(() => {
    if (mode !== "read") storeEditMode(mode);
  }, [mode]);
  const {
    registerKey,
    pushEscape,
    setOverlay,
    setShareDocPath,
    setRenameDocPath,
    setDeleteDocPath,
    toast,
  } = useUi();
  const save = useDocumentSave(path, doc);
  // The share button lights up while a live link exists for this document.
  const defaultArea = useDefaultArea();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const shares = useShares();
  const isShared = (shares.data ?? []).some(
    (share) =>
      share.doc_path === path &&
      !share.revoked_at &&
      (!share.expires_at || new Date(share.expires_at) > new Date()),
  );
  // Related-by-meaning needs the embeddings pipeline; the hook stays
  // disabled (no request) when it is off.
  const related = useRelated(path, useSemanticEnabled() && mode === "read");
  const editorRef = useRef<MarkdownEditorHandle>(null);
  const toggleTask = useToggleTask();
  // Editor buffer mirrored into state for the split preview and the outline
  // (debounced so neither re-renders per keystroke).
  const [liveText, setLiveText] = useState(
    () => splitFrontmatter(save.currentText()).body,
  );
  const previewText = useDebouncedValue(liveText, PREVIEW_DEBOUNCE_MS);
  const [editorTopLine, setEditorTopLine] = useState(1);
  const [editorContext, setEditorContext] = useState<EditorContext | null>(
    null,
  );
  // The properties strip while editing: the server rewrites the file the
  // buffer has just been flushed to, and the editor adopts the result.
  const stripSync: StripSync = {
    before: () => save.save(),
    after: (updated) => {
      save.adopt(updated);
      const { body } = splitFrontmatter(updated.markdown);
      editorRef.current?.adopt(body);
      setLiveText(body);
    },
  };

  // Read mode outlines the saved buffer; edit/split outlines what's being typed.
  const outlineSource = mode === "read" ? save.text : previewText;
  // Indexed task lines count from the top of the file; the editor buffer
  // starts after the frontmatter, so shift them for the split preview.
  const bodyTasks = useMemo(() => {
    const offset = frontmatterLines(save.text);
    return offset === 0
      ? doc.tasks
      : doc.tasks.map((task) => ({ ...task, line: task.line - offset }));
  }, [doc.tasks, save.text]);
  const headings = useMemo(
    () => extractHeadings(outlineSource),
    [outlineSource],
  );

  const exitEdit = () => {
    void save.save();
    setMode("read");
  };

  // A dangling [[link]] clicked in read mode becomes a note, filed where
  // this document is. The Unwritten page offers the other types.
  const createMissing = useCallback(
    (name: string) => {
      api
        .createDocument("note", name, undefined, doc.area || defaultArea)
        .then((created) => {
          void queryClient.invalidateQueries({ queryKey: ["documents"] });
          void queryClient.invalidateQueries({ queryKey: ["unwritten"] });
          void queryClient.invalidateQueries({
            queryKey: queryKeys.document(path),
          });
          toast(`Created ${created.title}`);
          void navigate({ to: docHref(created.path), search: { edit: true } });
        })
        .catch((error: unknown) => toast(errorMessage(error)));
    },
    [doc.area, defaultArea, path, navigate, queryClient, toast],
  );

  // Read mode's "Edit table": open the grid on the nth table and, on save,
  // splice the result into the buffer and flush it through the same
  // conflict-checked path an editor save takes.
  const editTable = useCallback(
    (index: number) => {
      const text = save.currentText();
      const range = findTables(text)[index];
      if (!range) return;
      requestTableEdit({
        block: text.slice(range.from, range.to),
        apply: (next) => {
          const now = save.currentText();
          const again = findTables(now)[index];
          if (!again) return;
          save.onEditorChange(
            now.slice(0, again.from) + next + now.slice(again.to),
          );
          void save.save();
        },
      });
    },
    [save],
  );

  // `e` enters edit mode; Escape (outside the editor's focus) backs out of it.
  useEffect(() => {
    if (mode === "read") return registerKey("e", () => setMode("edit"));
    return pushEscape(exitEdit);
    // exitEdit identity is per-render but only mode changes matter here.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mode, registerKey, pushEscape]);

  // Cmd/Ctrl+E cycles read → edit → split (split is desktop-only; small
  // screens bounce back to read). Registered once; uses functional setState.
  useEffect(
    () =>
      registerKey("mod+e", () => {
        const desktop = window.matchMedia("(min-width: 768px)").matches;
        setMode((current) => {
          if (current === "read") return "edit";
          if (current === "edit" && desktop) return "split";
          return "read";
        });
      }),
    [registerKey],
  );

  // Printing the editor is useless, so every print — the menu, the palette,
  // ⌘P, and the browser's own File ▸ Print — drops to read mode first.
  // flushSync, not a queued setState: `beforeprint` snapshots the page as soon
  // as its listeners return, so the switch has to be in the DOM by then.
  useEffect(
    () =>
      registerPrintHook((phase) => {
        if (phase === "print") flushSync(() => setMode("read"));
      }),
    [],
  );

  // Any route back to read mode flushes pending edits (Cmd+E cycling included;
  // a no-op when nothing changed).
  useEffect(() => {
    if (mode === "read") void save.save();
    // save.save is stable per mount.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mode]);

  // Tag pool for `#` autocomplete: everything this doc and its neighbors carry.
  const getTags = () => [
    ...doc.tags,
    ...doc.backlinks.flatMap((backlink) => backlink.tags),
    ...doc.tasks.flatMap((task) => task.tags),
  ];

  const typeInfo = DOC_TYPE_INFO[doc.type];
  return (
    <article className="flex min-h-full flex-col">
      {/* On desktop the header (title, mode, share, menu) stays put while the
          note scrolls; on a phone only the editor toolbar does, since the
          header would eat the screen. */}
      <div className="md:sticky md:-top-4 md:z-10 md:-mt-4 md:bg-surface md:pt-4">
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
          {/* Everything from here right is screen furniture: on paper the header
            is just the type icon, the title and the rule under it. */}
          <span className="ml-auto hidden text-xs text-muted sm:inline print:hidden">
            {formatRelativeTime(doc.mtime)}
          </span>
          {mode !== "read" ? (
            <span
              className={`font-mono text-[10px] print:hidden ${STATUS_LABEL[save.status].classes}`}
            >
              {STATUS_LABEL[save.status].text}
            </span>
          ) : null}
          <ModeSwitch mode={mode} onChange={setMode} onLeaveEdit={exitEdit} />
          <button
            type="button"
            onClick={() => setOverlay("markdownHelp", true)}
            aria-label="Markdown help"
            title="Markdown reference"
            className="flex size-7 items-center justify-center rounded border border-border text-muted hover:bg-hover hover:text-heading print:hidden"
          >
            <HelpCircle className="size-3.5" aria-hidden="true" />
          </button>
          <button
            type="button"
            onClick={() => setShareDocPath(path)}
            aria-label="Share this document"
            aria-pressed={isShared}
            title={isShared ? "Shared — manage its links" : "Share"}
            className={`flex size-7 items-center justify-center rounded border print:hidden ${
              isShared
                ? "border-accent bg-accent/10 text-accent hover:opacity-90"
                : "border-border text-muted hover:bg-hover hover:text-heading"
            }`}
          >
            <Share2 className="size-3.5" aria-hidden="true" />
          </button>
          <DocMenu
            onRename={() => setRenameDocPath(path)}
            onDelete={() => setDeleteDocPath(path)}
            onInsertDrawing={() =>
              insertDrawingInto(path).catch(() =>
                toast("Couldn't create a drawing — reload and try again"),
              )
            }
          />
          <button
            type="button"
            onClick={mode === "read" ? () => setMode("edit") : exitEdit}
            className="flex h-7 items-center gap-1.5 rounded border border-border px-2 text-xs text-body hover:bg-hover hover:text-heading md:hidden print:hidden"
          >
            <Pencil className="size-3" aria-hidden="true" />
            {mode === "read" ? "Edit" : "Done"}
          </button>
        </header>
        {/* The properties strip lives with the header in both modes; while
          editing it syncs through the buffer, and the toolbar joins it. */}
        {mode === "read" ? (
          <FrontmatterStrip doc={doc} />
        ) : (
          <>
            <FrontmatterStrip doc={doc} sync={stripSync} />
            <EditorToolbar
              context={editorContext}
              run={(command) => editorRef.current?.run(command)}
              onInsertDrawing={() =>
                insertDrawingInto(path).catch(() =>
                  toast("Couldn't create a drawing — reload and try again"),
                )
              }
            />
          </>
        )}
      </div>

      {save.status === "conflict" ? (
        <ConflictBanner
          onKeepMine={() => void save.keepMine()}
          onTakeDisk={() => {
            const disk = save.takeDisk();
            if (disk !== null) {
              editorRef.current?.setValue(splitFrontmatter(disk).body);
            }
          }}
        />
      ) : null}
      {save.status === "error" && save.errorMessage ? (
        <p className="mb-3 border border-danger/40 px-3 py-2 text-xs text-danger">
          {save.errorMessage}
        </p>
      ) : null}

      {/* The rail (outline + backlinks) is a sibling column, never an overlay:
          it shortens the content rather than floating over it, so nothing can
          overlap and the page can never scroll sideways. */}
      <div className="flex min-h-0 flex-1 gap-6 print:block">
        <div className="flex min-w-0 flex-1 flex-col">
          {mode === "read" ? (
            <>
              <Markdown
                markdown={save.text}
                links={doc.links}
                tasks={doc.tasks}
                onToggleTask={(task) => toggleTask.mutate(task)}
                onCreateMissing={createMissing}
                onEditTable={editTable}
              />
              <Backlinks doc={doc} />
            </>
          ) : (
            <>
              <div
                className={
                  mode === "split"
                    ? "grid flex-1 md:grid-cols-2"
                    : "flex flex-1 flex-col"
                }
              >
                <Suspense fallback={<SkeletonRows count={6} />}>
                  <MarkdownEditor
                    ref={editorRef}
                    initialValue={splitFrontmatter(save.currentText()).body}
                    onChange={(body) => {
                      // The buffer is the body; the frontmatter is the app's and is
                      // stitched back on from the last known text.
                      const head = splitFrontmatter(save.currentText()).head;
                      save.onEditorChange(head + body);
                      setLiveText(body);
                    }}
                    onSave={() => void save.save()}
                    onSaveAndExit={exitEdit}
                    onBlur={() => void save.save()}
                    getTags={getTags}
                    getArea={() => doc.area || defaultArea}
                    onTopLineChange={setEditorTopLine}
                    onContextChange={setEditorContext}
                  />
                </Suspense>
                {mode === "split" ? (
                  <div className="hidden min-w-0 border-l border-border pt-3 pl-4 md:block">
                    {/* Preview follows the buffer, debounced. A checkbox here
                      flips its line in the editor, so the toggle lands in
                      the same save as everything else being typed. */}
                    <Markdown
                      markdown={previewText}
                      links={doc.links}
                      tasks={bodyTasks}
                      onToggleLine={(line) =>
                        editorRef.current?.toggleTaskOnLine(line)
                      }
                    />
                  </div>
                ) : null}
              </div>
            </>
          )}
        </div>
        <DocumentRail
          headings={headings}
          mode={mode === "read" ? "rendered" : "source"}
          activeLine={editorTopLine}
          onScrollToLine={(line) => editorRef.current?.scrollToLine(line)}
          backlinks={doc.backlinks}
          related={related.data ?? []}
          openTasks={doc.open_tasks ?? []}
        />
      </div>
    </article>
  );
}

/** Read / Edit / Split segmented control — Cmd+E does the same cycling, and
 * Split needs the width, so it only appears on md+. */
function ModeSwitch({
  mode,
  onChange,
  onLeaveEdit,
}: {
  mode: ViewMode;
  onChange: (mode: ViewMode) => void;
  onLeaveEdit: () => void;
}) {
  const options: { value: ViewMode; label: string }[] = [
    { value: "read", label: "Read" },
    { value: "edit", label: "Edit" },
    { value: "split", label: "Split" },
  ];
  return (
    <div
      role="group"
      aria-label="View mode"
      title="Switch view — ⌘E cycles read / edit / split"
      className="hidden h-7 items-center rounded border border-border md:flex print:hidden"
    >
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          aria-pressed={mode === option.value}
          onClick={() => {
            // Leaving edit/split flushes pending edits, same as the Done button.
            if (option.value === "read") onLeaveEdit();
            else onChange(option.value);
          }}
          className={`h-full px-2 text-xs first:rounded-l last:rounded-r ${
            mode === option.value
              ? "bg-selected text-heading"
              : "text-muted hover:bg-hover hover:text-body"
          }`}
        >
          {option.label}
        </button>
      ))}
    </div>
  );
}

/** The "…" menu on the doc header — non-primary actions (Print, Rename,
 * Delete). */
function DocMenu({
  onRename,
  onDelete,
  onInsertDrawing,
}: {
  onRename: () => void;
  onDelete: () => void;
  onInsertDrawing: () => void;
}) {
  const [open, setOpen] = useState(false);
  return (
    <div className="relative print:hidden">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        aria-label="Document actions"
        aria-expanded={open}
        className="flex size-7 items-center justify-center rounded border border-border text-muted hover:bg-hover hover:text-heading"
      >
        <MoreHorizontal className="size-3.5" aria-hidden="true" />
      </button>
      {open ? (
        <>
          <div
            className="fixed inset-0 z-10"
            aria-hidden="true"
            onClick={() => setOpen(false)}
          />
          <div className="absolute top-full right-0 z-20 mt-1 w-44 rounded border border-border bg-raised py-1 shadow-lg">
            <button
              type="button"
              onClick={() => {
                setOpen(false);
                // The print hooks put the page in read mode and re-render any
                // diagrams for paper before the dialog opens.
                void printPage();
              }}
              className="flex h-8 w-full items-center gap-2 px-3 text-left text-xs text-body hover:bg-hover hover:text-heading"
            >
              <Printer className="size-3.5 text-muted" aria-hidden="true" />
              Print / Save as PDF
            </button>
            <button
              type="button"
              onClick={() => {
                setOpen(false);
                onInsertDrawing();
              }}
              className="flex h-8 w-full items-center gap-2 px-3 text-left text-xs text-body hover:bg-hover hover:text-heading"
            >
              <PenTool className="size-3.5 text-muted" aria-hidden="true" />
              Insert drawing
            </button>
            <button
              type="button"
              onClick={() => {
                setOpen(false);
                onRename();
              }}
              className="flex h-8 w-full items-center gap-2 px-3 text-left text-xs text-body hover:bg-hover hover:text-heading"
            >
              <FolderPen className="size-3.5 text-muted" aria-hidden="true" />
              Rename…
            </button>
            <button
              type="button"
              onClick={() => {
                setOpen(false);
                onDelete();
              }}
              className="flex h-8 w-full items-center gap-2 px-3 text-left text-xs text-body hover:bg-hover hover:text-danger"
            >
              <Trash2 className="size-3.5 text-muted" aria-hidden="true" />
              Delete…
            </button>
          </div>
        </>
      ) : null}
    </div>
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

/**
 * Backlinks below the document, for screens too narrow for the rail — above
 * lg the rail shows the same list beside the content, so this hides rather
 * than repeating it.
 */
function Backlinks({ doc }: { doc: Document }) {
  if (doc.backlinks.length === 0) return null;
  return (
    <section className="mt-8 border-t border-border pt-3 lg:hidden print:hidden">
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
