// Rendered markdown (read mode + Today's daily note). react-markdown with GFM,
// frontmatter (parsed away, shown by DocumentScreen as a properties strip), and
// the quire flavor from remarkQuire: wikilinks resolved through the document's
// links array, Obsidian callouts, clickable task checkboxes matched to indexed
// tasks by source line, and a copy button on fenced code.
import { Link as RouterLink } from "@tanstack/react-router";
import {
  AlertTriangle,
  Check,
  CheckCircle2,
  Copy,
  FileCode2,
  Flame,
  HelpCircle,
  Info,
  Lightbulb,
  StickyNote,
  type LucideIcon,
} from "lucide-react";
import {
  createContext,
  useContext,
  useMemo,
  useRef,
  useState,
  type ComponentProps,
  type ReactNode,
} from "react";
import ReactMarkdown, {
  type Components,
  type ExtraProps,
} from "react-markdown";
import remarkFrontmatter from "remark-frontmatter";
import remarkGfm from "remark-gfm";
import type { Link, Task } from "../api/types.ts";
import type { CalloutType } from "../lib/callouts.ts";
import { docHref } from "../lib/docs.ts";
import { remarkQuire, WIKILINK_HREF_PREFIX } from "../lib/remarkQuire.ts";

interface MarkdownProps {
  markdown: string;
  /** The document's parsed links — how wikilinks resolve to vault paths. */
  links?: Link[];
  /** Indexed tasks for this document; enables clickable checkboxes. */
  tasks?: Task[];
  onToggleTask?: (task: Task) => void;
}

interface MarkdownContextValue {
  links: Link[];
  tasks: Task[];
  onToggleTask?: (task: Task) => void;
}

const MarkdownContext = createContext<MarkdownContextValue>({
  links: [],
  tasks: [],
});

/** Source line of the enclosing list item, for checkbox → task matching. */
const LineContext = createContext<number | null>(null);

export function Markdown({
  markdown,
  links = [],
  tasks = [],
  onToggleTask,
}: MarkdownProps) {
  const context = useMemo(
    () => ({ links, tasks, onToggleTask }),
    [links, tasks, onToggleTask],
  );
  return (
    <MarkdownContext.Provider value={context}>
      <div className="prose-quire">
        <ReactMarkdown
          remarkPlugins={[remarkGfm, remarkFrontmatter, remarkQuire]}
          components={COMPONENTS}
        >
          {markdown}
        </ReactMarkdown>
      </div>
    </MarkdownContext.Provider>
  );
}

// ---- Wikilinks ----

function resolveWikilink(inner: string, links: Link[]): string | null {
  const target = inner.split("|")[0]!.trim();
  const match = links.find((link) => link.raw === inner || link.raw === target);
  return match?.target ?? null;
}

function Anchor(props: ComponentProps<"a"> & ExtraProps) {
  const { links } = useContext(MarkdownContext);
  const href = props.href ?? "";
  if (!href.startsWith(WIKILINK_HREF_PREFIX)) {
    return <a {...props} target="_blank" rel="noreferrer" />;
  }
  const inner = decodeURIComponent(href.slice(WIKILINK_HREF_PREFIX.length));
  const target = resolveWikilink(inner, links);
  if (!target) {
    return (
      <span
        className="cursor-default border-b border-dashed border-muted text-muted"
        title={`Unresolved link: ${inner}`}
      >
        {props.children}
      </span>
    );
  }
  return (
    <RouterLink
      to={docHref(target)}
      className="text-accent no-underline hover:underline"
    >
      {props.children}
    </RouterLink>
  );
}

// ---- Task checkboxes ----

function ListItem(props: ComponentProps<"li"> & ExtraProps) {
  const { node, className, ...rest } = props;
  const line = node?.position?.start.line ?? null;
  const isTask = className?.includes("task-list-item");
  return (
    <LineContext.Provider value={line}>
      <li {...rest} className={isTask ? `${className} task-item` : className} />
    </LineContext.Provider>
  );
}

function Checkbox(props: ComponentProps<"input"> & ExtraProps) {
  const { tasks, onToggleTask } = useContext(MarkdownContext);
  const line = useContext(LineContext);
  const task =
    line === null
      ? undefined
      : tasks.find((candidate) => candidate.line === line);
  const interactive = task !== undefined && onToggleTask !== undefined;
  return (
    <input
      type="checkbox"
      checked={props.checked ?? false}
      disabled={!interactive}
      onChange={interactive ? () => onToggleTask(task) : undefined}
      aria-label={
        task ? `Toggle task: ${task.text}` : "Task checkbox (not indexed)"
      }
      className={`mr-1.5 size-3.5 translate-y-0.5 rounded-sm accent-(--accent) ${
        interactive ? "cursor-pointer" : ""
      }`}
    />
  );
}

// ---- Fenced code with copy ----

function Pre(props: ComponentProps<"pre"> & ExtraProps) {
  const codeRef = useRef<HTMLPreElement>(null);
  const [copied, setCopied] = useState(false);
  const copy = () => {
    const text = codeRef.current?.innerText ?? "";
    void navigator.clipboard.writeText(text).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1_500);
    });
  };
  return (
    <div className="group relative">
      <pre ref={codeRef}>{props.children}</pre>
      <button
        type="button"
        onClick={copy}
        aria-label="Copy code"
        className="absolute top-1.5 right-1.5 flex size-6 items-center justify-center rounded border border-border bg-raised text-muted opacity-0 transition-opacity group-hover:opacity-100 hover:text-heading focus-visible:opacity-100"
      >
        {copied ? (
          <Check className="size-3.5 text-ok" aria-hidden="true" />
        ) : (
          <Copy className="size-3.5" aria-hidden="true" />
        )}
      </button>
    </div>
  );
}

// ---- Callouts ----

const CALLOUT_STYLE: Record<
  CalloutType,
  { icon: LucideIcon; edge: string; text: string }
> = {
  note: { icon: StickyNote, edge: "border-l-accent", text: "text-accent" },
  info: { icon: Info, edge: "border-l-accent", text: "text-accent" },
  tip: { icon: Lightbulb, edge: "border-l-ok", text: "text-ok" },
  warning: { icon: AlertTriangle, edge: "border-l-warn", text: "text-warn" },
  danger: { icon: Flame, edge: "border-l-danger", text: "text-danger" },
  question: { icon: HelpCircle, edge: "border-l-warn", text: "text-warn" },
  success: { icon: CheckCircle2, edge: "border-l-ok", text: "text-ok" },
  example: { icon: FileCode2, edge: "border-l-muted", text: "text-muted" },
};

function Blockquote(props: ComponentProps<"blockquote"> & ExtraProps) {
  const extra = props as Record<string, unknown>;
  const type = extra["data-callout"] as CalloutType | undefined;
  if (!type || !(type in CALLOUT_STYLE)) {
    return <blockquote>{props.children as ReactNode}</blockquote>;
  }
  const title = (extra["data-callout-title"] as string) || type;
  const style = CALLOUT_STYLE[type];
  return (
    <div
      data-callout={type}
      className={`my-3 rounded border border-border border-l-2 bg-raised ${style.edge}`}
    >
      <p
        className={`flex items-center gap-1.5 px-3 pt-2 text-xs font-semibold capitalize ${style.text}`}
      >
        <style.icon className="size-3.5 shrink-0" aria-hidden="true" />
        {title}
      </p>
      <div className="px-3 pb-2 text-body [&>p:first-child]:mt-1">
        {props.children}
      </div>
    </div>
  );
}

// ---- Images ----

// Vault-relative references ("attachments/2026/09/x.png") are what lives in
// the markdown so files stay meaningful to external editors; the server
// serves those bytes at /api/v1/files/<path>.
function Img(props: ComponentProps<"img"> & ExtraProps) {
  const { node: _node, src, ...rest } = props;
  let resolved = typeof src === "string" ? src : "";
  if (resolved && !/^(https?:|data:|\/)/.test(resolved)) {
    resolved = `/api/v1/files/${resolved}`;
  }
  return <img {...rest} src={resolved} loading="lazy" />;
}

const COMPONENTS: Components = {
  a: Anchor,
  li: ListItem,
  input: Checkbox,
  pre: Pre,
  blockquote: Blockquote,
  img: Img,
};
