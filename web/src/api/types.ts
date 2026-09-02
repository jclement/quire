// The REST API contract.
//
// The server-derived shapes are GENERATED from the Go structs in
// internal/service/apitypes.go (see generated.ts, `mise run gen`) and simply
// re-exported here, so this file can no longer drift from the server — CI
// fails if the generated file is stale.
//
// What stays hand-written below: shapes the Go side returns as ad-hoc JSON
// (the small /auth/* payloads) and types that exist only in the client.
export type {
  AgentGuidanceResponse,
  Attachment,
  DocType,
  Birthday,
  CalendarDay,
  CalendarDoc,
  CalendarMonth,
  ConnectedApp,
  DocMeta,
  Document,
  Health,
  Link,
  NewToken,
  RenameResult,
  SearchResult,
  ShareInfo,
  Task,
  TodayPayload,
  TokenInfo,
} from "./generated.ts";

/**
 * PATCH /tasks/<id> body — a REQUEST shape, so it is hand-written: the Go
 * struct uses nil pointers for "leave unchanged", which generation renders
 * as `| null` rather than as optional keys.
 *
 * NOTE: task ids are content-derived, so an edit (or toggle) can change the
 * id — always adopt the returned Task and invalidate task lists.
 */
export interface TaskEdit {
  /** Omitted = unchanged; empty string = clear the date. */
  due?: string;
  defer?: string;
  priority?: 0 | 1 | 2 | 3;
}

export type TaskView = "inbox" | "today" | "upcoming" | "waiting" | "logbook";

/** SSE payload for "doc" events on /api/v1/events. */
export interface DocEvent {
  path: string;
  action: "upsert" | "delete";
}

/** POST /attachments response — the same shape as the generated Attachment. */
export interface AttachmentUpload {
  path: string;
  /** Ready-to-insert markdown reference, e.g. ![](attachments/img.png). */
  markdown: string;
}

// ---- Passkey auth (only live when QUIRE_AUTH_MODE=passkey; in mode "none"
// every /auth/* endpoint 404s and the SPA skips all auth UI). These are
// hand-written because the Go handlers return ad-hoc JSON objects rather
// than named structs. ----

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
