// Rendered markdown (read mode + Today's daily note). react-markdown with GFM,
// frontmatter (parsed away, shown by DocumentScreen as a properties strip), and
// the quire flavor from remarkQuire: wikilinks resolved through the document's
// links array, Obsidian callouts, clickable task checkboxes matched to indexed
// tasks by source line, and fenced code with a copy button, lazy syntax
// highlighting, and lazy ```mermaid diagram rendering.
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
  lazy,
  Suspense,
  useContext,
  useMemo,
  useState,
  type ComponentProps,
  type ReactNode,
} from "react";
import type { ElementContent } from "hast";
import ReactMarkdown, {
  type Components,
  type ExtraProps,
} from "react-markdown";
import remarkFrontmatter from "remark-frontmatter";
import remarkGfm from "remark-gfm";
import type { Link, Task } from "../api/types.ts";
import type { CalloutType } from "../lib/callouts.ts";
import { docHref } from "../lib/docs.ts";
import { extractHeadings } from "../lib/headings.ts";
import { remarkQuire, WIKILINK_HREF_PREFIX } from "../lib/remarkQuire.ts";

// Both are heavy and rare per-page; each stays in its own chunk and loads only
// when a matching fence is actually rendered.
const MermaidDiagram = lazy(() => import("./MermaidDiagram.tsx"));
const HighlightedCode = lazy(() => import("./HighlightedCode.tsx"));

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
  /** Source line → anchor id, shared with the outline (see lib/headings.ts). */
  headingIdsByLine: Map<number, string>;
}

const MarkdownContext = createContext<MarkdownContextValue>({
  links: [],
  tasks: [],
  headingIdsByLine: new Map(),
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
    () => ({
      links,
      tasks,
      onToggleTask,
      headingIdsByLine: new Map(
        extractHeadings(markdown).map((heading) => [heading.line, heading.id]),
      ),
    }),
    [links, tasks, onToggleTask, markdown],
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

// ---- Headings ----

/**
 * Anchor ids for the outline to scroll to. Ids are looked up by the heading's
 * source line rather than recomputed here: extractHeadings already resolved
 * duplicate-title collisions, and a line lookup is deterministic (a render-time
 * counter would drift under StrictMode's double render).
 */
function makeHeadingComponent(level: 1 | 2 | 3 | 4 | 5 | 6) {
  const Tag = `h${level}` as const;
  return function HeadingWithAnchor(props: ComponentProps<"h1"> & ExtraProps) {
    const { headingIdsByLine } = useContext(MarkdownContext);
    const line = props.node?.position?.start.line;
    return (
      <Tag id={line === undefined ? undefined : headingIdsByLine.get(line)}>
        {props.children}
      </Tag>
    );
  };
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

// ---- Fenced code: copy button, highlighting, mermaid ----

/** Concatenated text content of a hast subtree. */
function hastText(node: ElementContent | undefined): string {
  if (!node) return "";
  if (node.type === "text") return node.value;
  if ("children" in node) return node.children.map(hastText).join("");
  return "";
}

/** The fence's ```lang, read off the generated <code class="language-lang">. */
function fenceLanguage(node: ExtraProps["node"]): {
  language: string | null;
  code: string;
} {
  const codeEl = node?.children.find(
    (child): child is Extract<ElementContent, { type: "element" }> =>
      child.type === "element" && child.tagName === "code",
  );
  const classes = Array.isArray(codeEl?.properties?.className)
    ? (codeEl.properties.className as string[])
    : [];
  const langClass = classes.find(
    (name) => typeof name === "string" && name.startsWith("language-"),
  );
  return {
    language: langClass ? langClass.slice("language-".length) : null,
    code: hastText(codeEl).replace(/\n$/, ""),
  };
}

function Pre(props: ComponentProps<"pre"> & ExtraProps) {
  const [copied, setCopied] = useState(false);
  const { language, code } = fenceLanguage(props.node);

  if (language === "mermaid") {
    return (
      <Suspense
        fallback={
          <div className="my-3 h-24 animate-pulse rounded border border-border bg-hover" />
        }
      >
        <MermaidDiagram code={code} />
      </Suspense>
    );
  }

  const copy = () => {
    void navigator.clipboard.writeText(code).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1_500);
    });
  };
  return (
    <div className="group relative">
      <pre>
        {language ? (
          <Suspense fallback={<code>{code}</code>}>
            <HighlightedCode code={code} language={language} />
          </Suspense>
        ) : (
          props.children
        )}
      </pre>
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
  h1: makeHeadingComponent(1),
  h2: makeHeadingComponent(2),
  h3: makeHeadingComponent(3),
  h4: makeHeadingComponent(4),
  h5: makeHeadingComponent(5),
  h6: makeHeadingComponent(6),
};
