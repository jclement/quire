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
  /** Recurrence spec, e.g. "every year" / "every 3 months when done". */
  recur: string | null;
}

/**
 * PATCH /tasks/<id> body. Omitted field = unchanged; empty string = clear.
 * NOTE: task IDs are content-derived, so an edit (or toggle) can change the
 * id — always adopt the returned Task and invalidate task lists.
 */
export interface TaskEdit {
  due?: string;
  defer?: string;
  priority?: 0 | 1 | 2 | 3;
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

/** An upcoming birthday from a person doc's frontmatter. */
export interface Birthday {
  path: string;
  title: string;
  date: string;
  days_until: number;
  /** Age they turn on that date; null when the birth year is unknown. */
  age: number | null;
}

export interface TodayPayload {
  date: string;
  daily: Document | null;
  meetings: DocMeta[];
  birthdays: Birthday[];
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

/** A public share link for one document (served at /s/<token>). */
export interface ShareInfo {
  token: string;
  doc_path: string;
  created_at: string;
  expires_at?: string | null;
  revoked_at?: string | null;
  view_count: number;
  last_viewed_at?: string | null;
  /** Absolute URL (tailnet/funnel hostname when configured). */
  url: string;
}

/** POST /rename result: the moved document + paths whose links were rewritten. */
export interface RenameResult {
  document: Document;
  rewritten: string[];
}

// ---- Passkey auth (only live when QUIRE_AUTH_MODE=passkey; in mode "none"
// every /auth/* endpoint 404s and the SPA skips all auth UI) ----

export interface AuthStatus {
  registered: boolean;
  authenticated: boolean;
}

export interface PasskeyInfo {
  id: string;
  name: string;
  created_at: string;
}

export interface RegisterFinishResult {
  /** Non-null ONLY for the very first passkey — show them once, full screen. */
  recovery_codes: string[] | null;
}

export interface RecoverResult {
  /** True = the code worked and the user should register a passkey now. */
  register_passkey: boolean;
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
