// The REST API contract — mirrors the Go server's /api/v1 responses exactly.
// If a shape changes here it changed on the server first; this file has no
// logic on purpose.

export type DocType =
  "note" | "person" | "company" | "project" | "meeting" | "daily";

export interface DocMeta {
  path: string;
  type: DocType;
  title: string;
  mtime: string;
  sha256: string;
  tags: string[];
}

export interface Task {
  id: string;
  doc_path: string;
  doc_title: string;
  line: number;
  text: string;
  done: boolean;
  due: string | null;
  defer: string | null;
  /** 0 none, 1 high, 2 medium, 3 low. */
  priority: number;
  waiting: boolean;
  project: string | null;
  tags: string[];
  completed_on: string | null;
}

export interface Link {
  /** Resolved vault path, or null when the wikilink dangles. */
  target: string | null;
  raw: string;
  display: string;
}

export interface Document extends DocMeta {
  markdown: string;
  frontmatter: Record<string, unknown>;
  links: Link[];
  backlinks: DocMeta[];
  tasks: Task[];
}

export interface SearchResult {
  path: string;
  type: DocType;
  title: string;
  /** Plain text with literal <mark> tags around hits — parse, never inject. */
  snippet: string;
}

export interface TodayPayload {
  date: string;
  daily: Document | null;
  meetings: DocMeta[];
  overdue: Task[];
  due_today: Task[];
  available: Task[];
  waiting: Task[];
  recent: DocMeta[];
}

export interface Health {
  status: string;
  version: string;
  update_available: boolean;
}

export interface AttachmentUpload {
  path: string;
  /** Ready-to-insert markdown reference, e.g. ![](attachments/img.png). */
  markdown: string;
}

export type TaskView = "inbox" | "today" | "upcoming" | "waiting" | "logbook";

/** SSE payload for "doc" events on /api/v1/events. */
export interface DocEvent {
  path: string;
  action: "upsert" | "delete";
}
