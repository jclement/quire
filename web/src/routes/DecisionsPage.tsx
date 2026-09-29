// The decision log: every bullet under a "Decisions" heading and every
// document tagged decision, newest first — "what did we settle, when, and
// who was in the room". Narrowed by the area switcher like every list, and
// by a text filter here; each row links back to where it was decided.
import { Link as RouterLink } from "@tanstack/react-router";
import { Gavel, Search as SearchIcon } from "lucide-react";
import { useState } from "react";
import { errorMessage } from "../api/client.ts";
import { useDecisions } from "../api/queries.ts";
import type { Decision } from "../api/types.ts";
import { EmptyState } from "../components/EmptyState.tsx";
import { SkeletonRows } from "../components/Skeleton.tsx";
import { InlineTaskText } from "../components/TaskList.tsx";
import { DOC_TYPE_INFO, docHref, isDocType } from "../lib/docs.ts";
import { noAutofill } from "../lib/noAutofill.ts";
import { useDebouncedValue } from "../lib/useDebouncedValue.ts";

/** Long enough to skip a request per keystroke, short enough to feel live. */
const FILTER_DEBOUNCE_MS = 200;

export function DecisionsPage() {
  const [filter, setFilter] = useState("");
  const q = useDebouncedValue(filter.trim(), FILTER_DEBOUNCE_MS);
  const decisions = useDecisions({ q });
  return (
    <div className="flex max-w-4xl flex-col gap-3">
      <header className="flex items-center gap-2 border-b border-border pb-2">
        <Gavel className="size-4 text-muted" aria-hidden="true" />
        <h1 className="text-lg font-semibold text-heading">Decisions</h1>
        {decisions.data ? (
          <span className="text-xs text-muted">{decisions.data.length}</span>
        ) : null}
      </header>
      <div className="flex items-center gap-2 border-b border-border pb-1 focus-within:border-accent">
        <SearchIcon
          className="size-3.5 shrink-0 text-muted"
          aria-hidden="true"
        />
        <input
          value={filter}
          onChange={(event) => setFilter(event.target.value)}
          placeholder="Filter decisions…"
          aria-label="Filter decisions"
          {...noAutofill("decision-filter")}
          className="field-bare h-8 w-full bg-transparent text-sm text-heading outline-none placeholder:text-muted"
        />
      </div>
      {decisions.isPending ? (
        <SkeletonRows count={5} />
      ) : decisions.isError ? (
        <p className="text-xs text-danger">{errorMessage(decisions.error)}</p>
      ) : decisions.data.length === 0 ? (
        <EmptyState
          icon={Gavel}
          title={q ? "No decisions match" : "No decisions yet"}
          hint={
            q
              ? "Try fewer words."
              : "Write bullets under a “## Decisions” heading in a meeting note, or create a note from the decision template."
          }
        />
      ) : (
        <ul className="divide-y divide-border border-y border-border">
          {decisions.data.map((decision) => (
            <DecisionRow
              key={`${decision.path}:${decision.line}`}
              decision={decision}
            />
          ))}
        </ul>
      )}
    </div>
  );
}

function DecisionRow({ decision }: { decision: Decision }) {
  const SourceIcon = isDocType(decision.type)
    ? DOC_TYPE_INFO[decision.type].icon
    : undefined;
  return (
    <li className="flex flex-wrap items-baseline gap-x-3 gap-y-1 px-2 py-1.5">
      <span className="w-20 shrink-0 font-mono text-[11px] text-muted">
        {decision.date}
      </span>
      <span className="min-w-0 flex-1 text-sm text-body">
        {decision.kind === "record" ? (
          <RouterLink
            to={docHref(decision.path)}
            className="font-medium text-heading hover:text-accent hover:underline"
          >
            {decision.text}
          </RouterLink>
        ) : (
          <InlineTaskText text={decision.text} />
        )}
      </span>
      <span className="flex flex-wrap items-center gap-1">
        {decision.entities.map((entity) => (
          <EntityChip
            key={entity.path}
            path={entity.path}
            title={entity.title}
            type={entity.type}
          />
        ))}
        {decision.kind === "inline" ? (
          <RouterLink
            to={docHref(decision.path)}
            title={`Decided in ${decision.title}`}
            className="flex max-w-48 items-center gap-1 truncate text-xs text-muted hover:text-accent hover:underline"
          >
            {SourceIcon ? (
              <SourceIcon className="size-3 shrink-0" aria-hidden="true" />
            ) : null}
            <span className="truncate">{decision.title}</span>
          </RouterLink>
        ) : (
          <span className="rounded border border-border px-1 font-mono text-[10px] uppercase text-muted">
            record
          </span>
        )}
      </span>
    </li>
  );
}

function EntityChip({
  path,
  title,
  type,
}: {
  path: string;
  title: string;
  type: string;
}) {
  const Icon = isDocType(type) ? DOC_TYPE_INFO[type].icon : undefined;
  return (
    <RouterLink
      to={docHref(path)}
      className="flex h-5 items-center gap-1 rounded-full border border-border px-1.5 text-[11px] text-muted hover:border-accent hover:text-accent"
    >
      {Icon ? <Icon className="size-3 shrink-0" aria-hidden="true" /> : null}
      {title}
    </RouterLink>
  );
}
