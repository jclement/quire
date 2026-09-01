// Cmd/Ctrl+K palette: fuzzy document search (server-side match, client-side
// re-rank so exact titles win) merged with static commands. A leading ">"
// restricts to commands. Full-screen on mobile, top-anchored panel on desktop.
// The inner content mounts fresh each open (blank query, instant focus), and
// the doc query keeps previous results while typing so the list never flashes.
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useNavigate, useRouterState } from "@tanstack/react-router";
import {
  CalendarRange,
  ChevronsRight,
  FolderPen,
  HelpCircle,
  Plus,
  Printer,
  Search,
  Share2,
  Sunrise,
  Trash2,
  type LucideIcon,
} from "lucide-react";
import {
  useMemo,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
} from "react";
import { api } from "../api/client.ts";
import { queryKeys } from "../api/queries.ts";
import type { DocMeta, DocType } from "../api/types.ts";
import { todayISO } from "../lib/dates.ts";
import { DOC_TYPE_INFO, docHref, vaultPathFromRoute } from "../lib/docs.ts";
import { fuzzyRank } from "../lib/fuzzy.ts";
import { printPage } from "../lib/printing.ts";
import { useDebouncedValue } from "../lib/useDebouncedValue.ts";
import { noAutofill } from "../lib/noAutofill.ts";
import { useUi } from "../keys/UiContext.tsx";
import { Modal } from "./Modal.tsx";

const RESULT_LIMIT = 12;
const QUERY_DEBOUNCE_MS = 150;

interface Command {
  id: string;
  label: string;
  icon: LucideIcon;
  run: (ui: ReturnType<typeof useUi>, go: (to: string) => void) => void;
}

const NEW_DOC_TYPES: DocType[] = [
  "note",
  "person",
  "company",
  "project",
  "meeting",
];

const COMMANDS: Command[] = [
  ...NEW_DOC_TYPES.map<Command>((type) => ({
    id: `new-${type}`,
    label: `New ${DOC_TYPE_INFO[type].label}`,
    icon: Plus,
    run: (ui) => ui.setNewDocType(type),
  })),
  {
    id: "open-today",
    label: "Open Today",
    icon: Sunrise,
    run: (_ui, go) => go("/today"),
  },
  {
    id: "open-daily",
    label: "Open Daily Note",
    icon: DOC_TYPE_INFO.daily.icon,
    run: (_ui, go) => go(`/daily/${todayISO()}`),
  },
  {
    id: "open-calendar",
    label: "Open Calendar",
    icon: CalendarRange,
    run: (_ui, go) => go("/calendar"),
  },
  {
    id: "search",
    label: "Search",
    icon: Search,
    run: (_ui, go) => go("/search"),
  },
  {
    id: "markdown-help",
    label: "Markdown help",
    icon: HelpCircle,
    run: (ui) => ui.setOverlay("markdownHelp", true),
  },
  {
    // Not a document-context command: any page is printable, and on a document
    // the registered print hooks drop it to read mode first (lib/printing.ts).
    id: "print",
    label: "Print / Save as PDF",
    icon: Printer,
    run: () => void printPage(),
  },
];

type PaletteItem =
  { kind: "command"; command: Command } | { kind: "doc"; doc: DocMeta };

function itemText(item: PaletteItem): string {
  return item.kind === "command" ? item.command.label : item.doc.title;
}

/** Commands (static + context) + fetched docs, fuzzy-ranked; ">" limits to
 * commands. */
function usePaletteItems(
  query: string,
  extraCommands: Command[],
): PaletteItem[] {
  const commandsOnly = query.startsWith(">");
  const text = (commandsOnly ? query.slice(1) : query).trim();
  const debounced = useDebouncedValue(text, QUERY_DEBOUNCE_MS);

  const docsQuery = useQuery({
    queryKey: queryKeys.documents({ q: debounced, limit: RESULT_LIMIT }),
    queryFn: () => api.listDocuments({ q: debounced, limit: RESULT_LIMIT }),
    enabled: !commandsOnly && debounced.length > 0,
    placeholderData: keepPreviousData,
    retry: false,
  });

  return useMemo(() => {
    const commands = [...COMMANDS, ...extraCommands].map<PaletteItem>(
      (command) => ({ kind: "command", command }),
    );
    if (commandsOnly) return fuzzyRank(text, commands, itemText);
    const docs = (docsQuery.data ?? []).map<PaletteItem>((doc) => ({
      kind: "doc",
      doc,
    }));
    return fuzzyRank(text, [...docs, ...commands], itemText).slice(
      0,
      RESULT_LIMIT,
    );
  }, [commandsOnly, text, docsQuery.data, extraCommands]);
}

export function CommandPalette() {
  const { overlays, setOverlay } = useUi();
  const open = overlays.palette;
  const close = () => setOverlay("palette", false);
  return (
    <Modal
      open={open}
      onClose={close}
      variant="palette"
      label="Command palette"
    >
      <PaletteContent close={close} />
    </Modal>
  );
}

function PaletteContent({ close }: { close: () => void }) {
  const ui = useUi();
  const navigate = useNavigate();
  const [query, setQuery] = useState("");
  const [index, setIndex] = useState(0);

  // Share/Rename only exist while a document page is open — they act on it.
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  });
  const docPath = vaultPathFromRoute(pathname);
  const contextCommands = useMemo<Command[]>(
    () =>
      docPath
        ? [
            {
              id: "share-doc",
              label: "Share this document",
              icon: Share2,
              run: (paletteUi) => paletteUi.setShareDocPath(docPath),
            },
            {
              id: "rename-doc",
              label: "Rename this document",
              icon: FolderPen,
              run: (paletteUi) => paletteUi.setRenameDocPath(docPath),
            },
            {
              id: "delete-doc",
              label: "Delete this document",
              icon: Trash2,
              run: (paletteUi) => paletteUi.setDeleteDocPath(docPath),
            },
          ]
        : [],
    [docPath],
  );
  const items = usePaletteItems(query, contextCommands);

  // Selection resets when the query changes — adjusted during render rather
  // than in an effect (the React-endorsed pattern; avoids a double render pass).
  const [lastQuery, setLastQuery] = useState(query);
  if (lastQuery !== query) {
    setLastQuery(query);
    setIndex(0);
  }

  const go = (to: string) => void navigate({ to });
  const pick = (item: PaletteItem) => {
    close();
    if (item.kind === "doc") go(docHref(item.doc.path));
    else item.command.run(ui, go);
  };

  const onKeyDown = (event: ReactKeyboardEvent) => {
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      const delta = event.key === "ArrowDown" ? 1 : -1;
      setIndex((at) => Math.min(items.length - 1, Math.max(0, at + delta)));
    } else if (event.key === "Enter") {
      event.preventDefault();
      const item = items[index];
      if (item) pick(item);
    }
  };

  return (
    <>
      <div className="flex items-center gap-2 border-b border-border px-3 focus-within:border-accent">
        <Search className="size-4 shrink-0 text-muted" aria-hidden="true" />
        <input
          autoFocus
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          onKeyDown={onKeyDown}
          placeholder="Search documents, > for commands…"
          aria-label="Command palette input"
          {...noAutofill("palette")}
          className="field-bare h-11 w-full bg-transparent text-sm text-heading outline-none placeholder:text-muted"
        />
      </div>
      <ul className="flex-1 overflow-y-auto py-1 md:max-h-80" role="listbox">
        {items.map((item, at) => (
          <PaletteRow
            key={item.kind === "doc" ? item.doc.path : item.command.id}
            item={item}
            selected={at === index}
            onPick={() => pick(item)}
            onHover={() => setIndex(at)}
          />
        ))}
        {items.length === 0 ? (
          <li className="px-3 py-6 text-center text-xs text-muted">
            {query ? "No matches." : "Type to search, or > for commands."}
          </li>
        ) : null}
      </ul>
    </>
  );
}

function PaletteRow({
  item,
  selected,
  onPick,
  onHover,
}: {
  item: PaletteItem;
  selected: boolean;
  onPick: () => void;
  onHover: () => void;
}) {
  const Icon =
    item.kind === "command"
      ? item.command.icon
      : DOC_TYPE_INFO[item.doc.type].icon;
  return (
    <li role="option" aria-selected={selected}>
      <button
        type="button"
        onClick={onPick}
        onMouseMove={onHover}
        className={`flex h-9 w-full items-center gap-2.5 px-3 text-left text-sm ${
          selected ? "bg-selected text-heading" : "text-body hover:bg-hover"
        }`}
      >
        <Icon className="size-4 shrink-0 text-muted" aria-hidden="true" />
        <span className="truncate">{itemText(item)}</span>
        <span className="ml-auto flex items-center gap-1 font-mono text-[10px] uppercase text-muted">
          {item.kind === "doc" ? (
            item.doc.type
          ) : (
            <>
              <ChevronsRight className="size-3" aria-hidden="true" />
              command
            </>
          )}
        </span>
      </button>
    </li>
  );
}
