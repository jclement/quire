import { AreaDot } from "./AreaDot.tsx";
import { areaColorVar } from "../lib/area.ts";
import { useQueryClient } from "@tanstack/react-query";
import { queryKeys, useAreas, useAreasEnabled } from "../api/queries.ts";
// The properties strip under a document's title: frontmatter as dense
// key:value chips, never rendered as markdown. The relationship keys for this
// document's type (a meeting's people/project/company, a person's company)
// are editable in place — each stored link is a chip that opens the document
// and removes with ×, and "+" opens a typeahead over documents of the
// matching type that can also create the one you were looking for.
//
// Writes go through POST /link, which is idempotent and owns the rules about
// which keys hold a scalar and which hold a list; the response is the
// rewritten document, so the page updates from the server's own answer.
//
// This stays a strip, not a form: the controls are chip-scale at every width.
// Editing frontmatter on a phone is possible but the editor is the better
// place for it, per DESIGN.md's "serious editing is a desktop activity".
import { Link as RouterLink } from "@tanstack/react-router";
import { useMutation } from "@tanstack/react-query";
import { Plus, X } from "lucide-react";
import { useState } from "react";
import { api, errorMessage } from "../api/client.ts";
import { useDocumentList, useLinkEntity } from "../api/queries.ts";
import type { Document } from "../api/types.ts";
import { docHref, DOC_TYPE_INFO } from "../lib/docs.ts";
import {
  linkKeysFor,
  linkTargets,
  resolvedTargets,
  type LinkKey,
} from "../lib/entityLinks.ts";
import { noAutofill } from "../lib/noAutofill.ts";
import { useDebouncedValue } from "../lib/useDebouncedValue.ts";
import { useUi } from "../keys/UiContext.tsx";

const TYPEAHEAD_LIMIT = 8;
const TYPEAHEAD_DEBOUNCE_MS = 150;

export function FrontmatterStrip({ doc }: { doc: Document }) {
  const { toast } = useUi();
  const linkEntity = useLinkEntity(doc.path);
  const [addingKey, setAddingKey] = useState<string | null>(null);

  const linkKeys = linkKeysFor(doc.type);
  const editable = new Set(linkKeys.map((linkKey) => linkKey.key));
  // Everything else stays read-only: title is the heading, and a key this
  // type doesn't declare (a tag list, a due date) isn't a relationship.
  const entries = Object.entries(doc.frontmatter).filter(
    ([key]) => key !== "title" && !editable.has(key),
  );
  const areasEnabled = useAreasEnabled();
  const showArea = areasEnabled && doc.type !== "daily";
  if (linkKeys.length === 0 && entries.length === 0 && !showArea) return null;

  const resolved = resolvedTargets(doc.links);
  const apply = (key: string, target: string, remove = false) => {
    setAddingKey(null);
    linkEntity.mutate(
      { key, target, remove },
      { onError: (error) => toast(errorMessage(error)) },
    );
  };

  return (
    <div className="mb-3 flex flex-wrap items-start gap-1.5 border-b border-border pb-3">
      {showArea ? <AreaChip doc={doc} /> : null}
      {linkKeys.map((linkKey) => (
        <LinkKeyChips
          key={linkKey.key}
          linkKey={linkKey}
          targets={linkTargets(doc.frontmatter[linkKey.key])}
          resolved={resolved}
          adding={addingKey === linkKey.key}
          onAdding={(open) => setAddingKey(open ? linkKey.key : null)}
          onLink={(target) => apply(linkKey.key, target)}
          onUnlink={(target) => apply(linkKey.key, target, true)}
        />
      ))}
      {entries.map(([key, value]) => (
        <span key={key} className={`${CHIP_CLASSES} max-w-full`}>
          <span className="text-muted">{key}:</span>
          <span className="truncate text-body">
            {formatFrontmatterValue(value)}
          </span>
        </span>
      ))}
    </div>
  );
}

/** The shell every property shares — one dense bordered pill. On paper the
 * pill dissolves: the values are worth printing, the chrome around them is
 * not. */
const CHIP_CLASSES =
  "inline-flex items-center gap-1 rounded border border-border bg-raised px-1.5 py-0.5 font-mono text-[10px] print:rounded-none print:border-0 print:bg-transparent print:px-0";

function formatFrontmatterValue(value: unknown): string {
  if (Array.isArray(value)) return value.map(String).join(", ");
  if (value === null || value === undefined) return "—";
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

function LinkKeyChips({
  linkKey,
  targets,
  resolved,
  adding,
  onAdding,
  onLink,
  onUnlink,
}: {
  linkKey: LinkKey;
  targets: string[];
  resolved: Map<string, string>;
  adding: boolean;
  onAdding: (open: boolean) => void;
  onLink: (target: string) => void;
  onUnlink: (target: string) => void;
}) {
  // A scalar key holds one link, so it only offers "+" while it is empty —
  // changing it is remove-then-add rather than a silent overwrite.
  const canAdd = !linkKey.singular || targets.length === 0;
  // A div, not a span: the popover below it is block content.
  return (
    <div
      // A key with nothing linked is an invitation to link something, which is
      // an edit affordance — on paper it would print as a bare "people:".
      className={`${CHIP_CLASSES} relative max-w-full ${
        targets.length === 0 ? "print:hidden" : ""
      }`}
    >
      <span className="text-muted">{linkKey.key}:</span>
      {targets.map((target) => (
        <LinkChip
          key={target}
          target={target}
          path={resolved.get(target.toLowerCase())}
          onRemove={() => onUnlink(target)}
        />
      ))}
      {canAdd ? (
        <button
          type="button"
          onClick={() => onAdding(!adding)}
          aria-label={`Add ${linkKey.key} to this document`}
          aria-expanded={adding}
          className="-my-0.5 flex items-center rounded p-1 text-muted hover:bg-hover hover:text-heading print:hidden"
        >
          <Plus className="size-3" aria-hidden="true" />
        </button>
      ) : null}
      {adding ? (
        <AddLinkPopover
          linkKey={linkKey}
          onClose={() => onAdding(false)}
          onPick={onLink}
        />
      ) : null}
    </div>
  );
}

function LinkChip({
  target,
  path,
  onRemove,
}: {
  target: string;
  path: string | undefined;
  onRemove: () => void;
}) {
  return (
    <span className="inline-flex min-w-0 items-center gap-0.5 rounded-sm bg-hover px-1 print:bg-transparent print:px-0">
      {path ? (
        <RouterLink
          to={docHref(path)}
          className="truncate text-body hover:text-accent"
        >
          {target}
        </RouterLink>
      ) : (
        <span
          className="truncate text-muted"
          title="No document links here yet"
        >
          {target}
        </span>
      )}
      <button
        type="button"
        onClick={onRemove}
        aria-label={`Unlink ${target}`}
        className="-my-0.5 flex shrink-0 items-center rounded p-1 text-muted hover:text-danger print:hidden"
      >
        <X className="size-2.5" aria-hidden="true" />
      </button>
    </span>
  );
}

/**
 * Typeahead over documents of one type, with a create-if-missing row so a
 * person who doesn't have a page yet isn't a dead end. Picking hands the
 * title back — the server resolves it to a wikilink.
 */
function AddLinkPopover({
  linkKey,
  onClose,
  onPick,
}: {
  linkKey: LinkKey;
  onClose: () => void;
  onPick: (target: string) => void;
}) {
  const [text, setText] = useState("");
  const typed = text.trim();
  const query = useDebouncedValue(typed, TYPEAHEAD_DEBOUNCE_MS);
  const matches = useDocumentList({
    type: linkKey.type,
    q: query,
    limit: TYPEAHEAD_LIMIT,
  });
  const info = DOC_TYPE_INFO[linkKey.type];

  const create = useMutation({
    mutationFn: (title: string) => api.createDocument(linkKey.type, title),
    onSuccess: (created) => onPick(created.title),
  });

  const found = matches.data ?? [];
  const exact = found.some(
    (match) => match.title.toLowerCase() === typed.toLowerCase(),
  );
  const canCreate = typed.length > 0 && !exact && !create.isPending;

  return (
    <>
      {/* Click-away layer under the popover. */}
      <div
        className="fixed inset-0 z-10"
        aria-hidden="true"
        onClick={onClose}
      />
      <div
        role="dialog"
        aria-label={`Add ${linkKey.key}`}
        onKeyDown={(event) => {
          if (event.key === "Escape") {
            event.stopPropagation();
            onClose();
          }
          if (event.key === "Enter") {
            event.preventDefault();
            const first = found[0];
            if (first) onPick(first.title);
            else if (canCreate) create.mutate(typed);
          }
        }}
        className="absolute top-full left-0 z-20 mt-1 w-56 rounded border border-border bg-raised py-1 font-sans shadow-lg"
      >
        <div className="border-b border-border px-2 pb-1 focus-within:border-accent">
          <input
            autoFocus
            value={text}
            onChange={(event) => setText(event.target.value)}
            placeholder={`${info.label} name…`}
            aria-label={`Search ${info.plural.toLowerCase()}`}
            {...noAutofill(`link-${linkKey.key}`)}
            className="field-bare h-7 w-full bg-transparent text-xs text-heading outline-none placeholder:text-muted"
          />
        </div>
        <ul className="max-h-48 overflow-y-auto">
          {found.map((match) => (
            <li key={match.path}>
              <button
                type="button"
                onClick={() => onPick(match.title)}
                className="flex h-8 w-full items-center gap-2 px-2 text-left text-xs text-body hover:bg-hover hover:text-heading"
              >
                <info.icon
                  className="size-3.5 shrink-0 text-muted"
                  aria-hidden="true"
                />
                <span className="truncate">{match.title}</span>
              </button>
            </li>
          ))}
        </ul>
        {canCreate ? (
          <button
            type="button"
            onClick={() => create.mutate(typed)}
            className="flex h-8 w-full items-center gap-2 border-t border-border px-2 text-left text-xs text-body hover:bg-hover hover:text-heading"
          >
            <Plus className="size-3.5 shrink-0 text-muted" aria-hidden="true" />
            <span className="truncate">
              Create {info.label.toLowerCase()} “{typed}”
            </span>
          </button>
        ) : null}
        {found.length === 0 && !canCreate ? (
          <p className="px-2 py-2 text-xs text-muted">
            {create.isPending
              ? "Creating…"
              : create.isError
                ? errorMessage(create.error)
                : `No ${info.plural.toLowerCase()} yet — type a name to create one.`}
          </p>
        ) : null}
      </div>
    </>
  );
}

/**
 * Which area the document files under, changeable in place. Rendered for
 * every non-daily document — including ones with no frontmatter at all —
 * because filing is the thing you most want to do to an unfiled note.
 */
function AreaChip({ doc }: { doc: Document }) {
  const queryClient = useQueryClient();
  const { toast } = useUi();
  const areas = useAreas();
  const setArea = useMutation({
    mutationFn: (area: string) =>
      api.setFrontmatter(doc.path, { area: area || null }),
    onSuccess: (updated) => {
      queryClient.setQueryData(queryKeys.document(doc.path), updated);
      void queryClient.invalidateQueries({ queryKey: ["documents"] });
      void queryClient.invalidateQueries({ queryKey: ["areas"] });
    },
    onError: (error) => toast(errorMessage(error)),
  });
  const choices = Array.from(
    new Set(
      [...(areas.data ?? []).map((a) => a.area), doc.area].filter(Boolean),
    ),
  );
  const current = (areas.data ?? []).find((a) => a.area === doc.area);
  return (
    <label
      className="flex h-7 items-center gap-1 rounded border bg-raised px-1.5 text-xs text-muted"
      style={{
        borderColor: doc.area ? areaColorVar(current?.color) : undefined,
      }}
    >
      {doc.area ? <AreaDot color={current?.color} /> : <span>area</span>}
      <select
        aria-label="Document area"
        value={doc.area}
        disabled={setArea.isPending}
        onChange={(event) => setArea.mutate(event.target.value)}
        className="field-bare bg-transparent text-xs text-heading outline-none"
      >
        <option value="">unclassified</option>
        {choices.map((choice) => (
          <option key={choice} value={choice}>
            {choice}
          </option>
        ))}
      </select>
    </label>
  );
}
