// Typed fetch client for /api/v1. All responses use the house envelope
// ({"data":…} / {"error":{code,message}}); every non-2xx becomes a thrown
// ApiError carrying the server's code so callers can branch on e.g. CONFLICT
// without string-matching messages.
import type {
  AgentGuidanceResponse,
  AreaCount,
  AttachmentUpload,
  AuditEntry,
  AuthStatus,
  CalendarMonth,
  ConnectedApp,
  DocMeta,
  DocType,
  Document,
  Health,
  PasskeyInfo,
  RecoverResult,
  RegisterFinishResult,
  RenameResult,
  NewToken,
  SearchResult,
  ShareInfo,
  TagCount,
  Task,
  TemplateInfo,
  TaskEdit,
  TaskView,
  TodayPayload,
  TokenInfo,
} from "./types.ts";

export class ApiError extends Error {
  readonly code: string;
  readonly status: number;

  constructor(code: string, message: string, status: number) {
    super(message);
    this.name = "ApiError";
    this.code = code;
    this.status = status;
  }
}

/** True when an ApiError is the document-write 409 the editor must handle. */
export function isConflictError(error: unknown): error is ApiError {
  return error instanceof ApiError && error.code === "CONFLICT";
}

/** User-facing message for a failed call; rate limits get a retry-later note. */
export function errorMessage(error: unknown): string {
  if (error instanceof ApiError && error.status === 429) {
    return `${error.message} — wait a minute and try again.`;
  }
  if (error instanceof Error) return error.message;
  return "Something went wrong.";
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let response: Response;
  try {
    response = await fetch(path, init);
  } catch {
    // Backend down (dev without the Go server) surfaces as a typed error too.
    throw new ApiError("UNREACHABLE", "The quire server is not reachable.", 0);
  }
  if (response.status === 204) return undefined as T;

  const body = await response.json().catch(() => null);
  if (!response.ok) {
    // In passkey mode an expired session turns every call into a 401; the
    // AuthGate listens for this event and re-checks auth status.
    if (response.status === 401) {
      window.dispatchEvent(new Event("quire:unauthorized"));
    }
    const code = body?.error?.code ?? "UNKNOWN";
    const message =
      body?.error?.message ?? `Request failed (${response.status})`;
    throw new ApiError(code, message, response.status);
  }
  return body.data as T;
}

// enrollQuery renders the optional bootstrap enrollment code as a query
// fragment, or "" when there is none to send.
function enrollQuery(code?: string): string {
  const trimmed = code?.trim();
  return trimmed ? `?enroll_code=${encodeURIComponent(trimmed)}` : "";
}

function jsonInit(method: string, payload: unknown): RequestInit {
  return {
    method,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  };
}

/** Encodes a vault path for use in a URL, keeping its slashes literal. */
function encodeVaultPath(path: string): string {
  return path.split("/").map(encodeURIComponent).join("/");
}

export interface ListDocumentsParams {
  type?: DocType;
  q?: string;
  limit?: number;
  /** "" = every area, "none" = unclassified, else an area name. */
  area?: string;
}

export const api = {
  health: () => request<Health>("/api/v1/health"),

  listDocuments: (params: ListDocumentsParams = {}) => {
    const query = new URLSearchParams();
    if (params.type) query.set("type", params.type);
    if (params.q) query.set("q", params.q);
    if (params.limit) query.set("limit", String(params.limit));
    if (params.area) query.set("area", params.area);
    const suffix = query.size > 0 ? `?${query}` : "";
    return request<DocMeta[]>(`/api/v1/documents${suffix}`);
  },

  getDocument: (path: string) =>
    request<Document>(`/api/v1/documents/${encodeVaultPath(path)}`),

  putDocument: (path: string, markdown: string, baseSha256: string) =>
    request<Document>(
      `/api/v1/documents/${encodeVaultPath(path)}`,
      jsonInit("PUT", { markdown, base_sha256: baseSha256 }),
    ),

  createDocument: (
    type: DocType,
    title: string,
    markdown?: string,
    area?: string,
    template?: string,
  ) =>
    request<Document>(
      "/api/v1/documents",
      jsonInit("POST", {
        type,
        title,
        ...(markdown ? { markdown } : {}),
        ...(area ? { area } : {}),
        ...(template ? { template } : {}),
      }),
    ),

  listTemplates: () => request<TemplateInfo[]>("/api/v1/templates"),

  installStarterTemplates: () =>
    request<{ written: string[] }>("/api/v1/templates/starter", {
      method: "POST",
    }),

  /** Sets frontmatter keys surgically; a null value removes the key. */
  setFrontmatter: (path: string, values: Record<string, unknown>) =>
    request<Document>(
      `/api/v1/documents/${encodeVaultPath(path)}`,
      jsonInit("PATCH", { set: values }),
    ),

  listAreas: () => request<AreaCount[]>("/api/v1/areas"),

  deleteDocument: (path: string) =>
    request<void>(`/api/v1/documents/${encodeVaultPath(path)}`, {
      method: "DELETE",
    }),

  /**
   * Adds or removes one entity link in a frontmatter key ("put this person on
   * that meeting"). Idempotent, and the server decides whether the key holds a
   * scalar or a list; `target` is a document title or a `[[wikilink]]`.
   */
  link: (path: string, key: string, target: string, remove = false) =>
    request<Document>(
      "/api/v1/link",
      jsonInit("POST", { path, key, target, ...(remove ? { remove } : {}) }),
    ),

  search: (q: string) =>
    request<SearchResult[]>(`/api/v1/search?q=${encodeURIComponent(q)}`),

  listTasks: (view: TaskView, area = "") =>
    request<Task[]>(
      `/api/v1/tasks?view=${view}${area ? `&area=${encodeURIComponent(area)}` : ""}`,
    ),

  createTask: (text: string, due?: string, defer?: string) =>
    request<Task>(
      "/api/v1/tasks",
      jsonInit("POST", {
        text,
        ...(due ? { due } : {}),
        ...(defer ? { defer } : {}),
      }),
    ),

  toggleTask: (id: string) =>
    request<Task>(`/api/v1/tasks/${encodeURIComponent(id)}/toggle`, {
      method: "POST",
    }),

  editTask: (id: string, edit: TaskEdit) =>
    request<Task>(
      `/api/v1/tasks/${encodeURIComponent(id)}`,
      jsonInit("PATCH", edit),
    ),

  getDaily: (date: string) => request<Document>(`/api/v1/daily/${date}`),

  /** The journal's page of history: daily notes before `before`, newest first. */
  listDaily: (before: string, limit = 10) =>
    request<Document[]>(
      `/api/v1/daily?before=${encodeURIComponent(before)}&limit=${limit}`,
    ),

  listTags: () => request<TagCount[]>("/api/v1/tags"),

  listAudit: (limit = 100) =>
    request<AuditEntry[]>(`/api/v1/audit?limit=${limit}`),

  createDaily: (date: string) =>
    request<Document>(`/api/v1/daily/${date}`, { method: "POST" }),

  today: (area = "") =>
    request<TodayPayload>(
      `/api/v1/today${area ? `?area=${encodeURIComponent(area)}` : ""}`,
    ),

  /** One month of days ("YYYY-MM"), each with its notes, meetings and tasks. */
  calendar: (month: string) =>
    request<CalendarMonth>(
      `/api/v1/calendar?month=${encodeURIComponent(month)}`,
    ),

  agentGuidance: () => request<AgentGuidanceResponse>("/api/v1/agent-guidance"),

  /** Empty text deletes the guidance document. */
  setAgentGuidance: (text: string) =>
    request<AgentGuidanceResponse>(
      "/api/v1/agent-guidance",
      jsonInit("PUT", { text }),
    ),

  listShares: () => request<ShareInfo[]>("/api/v1/shares"),

  createShare: (path: string, expiresInDays?: number) =>
    request<ShareInfo>(
      "/api/v1/shares",
      jsonInit("POST", {
        path,
        ...(expiresInDays ? { expires_in_days: expiresInDays } : {}),
      }),
    ),

  revokeShare: (token: string) =>
    request<void>(`/api/v1/shares/${encodeURIComponent(token)}`, {
      method: "DELETE",
    }),

  // ---- credentials ----

  listTokens: () => request<TokenInfo[]>("/api/v1/tokens"),

  createToken: (name: string, scopes: string[], expiresInDays?: number) =>
    request<NewToken>(
      "/api/v1/tokens",
      jsonInit("POST", {
        name,
        scopes,
        ...(expiresInDays ? { expires_in_days: expiresInDays } : {}),
      }),
    ),

  revokeToken: (prefix: string) =>
    request<void>(`/api/v1/tokens/${encodeURIComponent(prefix)}`, {
      method: "DELETE",
    }),

  listConnectedApps: () => request<ConnectedApp[]>("/api/v1/connected-apps"),

  disconnectApp: (clientId: string) =>
    request<void>(`/api/v1/connected-apps/${encodeURIComponent(clientId)}`, {
      method: "DELETE",
    }),

  rename: (path: string, newPath: string, rewriteLinks: boolean) =>
    request<RenameResult>(
      "/api/v1/rename",
      jsonInit("POST", {
        path,
        new_path: newPath,
        rewrite_links: rewriteLinks,
      }),
    ),

  uploadAttachment: (file: File) => {
    const form = new FormData();
    form.append("file", file);
    return request<AttachmentUpload>("/api/v1/attachments", {
      method: "POST",
      body: form,
    });
  },

  /** Photo→task capture: any of file/text/due, at least one of file/text. */
  capture: (input: { file?: File; text?: string; due?: string }) => {
    const form = new FormData();
    if (input.file) form.append("file", input.file);
    if (input.text) form.append("text", input.text);
    if (input.due) form.append("due", input.due);
    return request<Task>("/api/v1/capture", { method: "POST", body: form });
  },

  // ---- Auth (404s wholesale when the server runs auth mode "none") ----

  authStatus: () => request<AuthStatus>("/api/v1/auth/status"),

  /** WebAuthn creation options in JSON form; api/auth.ts decodes and calls. */
  authRegisterBegin: (enrollCode?: string) =>
    request<Record<string, unknown>>(
      `/api/v1/auth/register/begin${enrollQuery(enrollCode)}`,
      { method: "POST" },
    ),

  authRegisterFinish: (
    name: string,
    credential: unknown,
    enrollCode?: string,
  ) =>
    request<RegisterFinishResult>(
      `/api/v1/auth/register/finish?name=${encodeURIComponent(name)}` +
        enrollQuery(enrollCode).replace("?", "&"),
      jsonInit("POST", credential),
    ),

  authLoginBegin: () =>
    request<Record<string, unknown>>("/api/v1/auth/login/begin", {
      method: "POST",
    }),

  authLoginFinish: (assertion: unknown) =>
    request<void>("/api/v1/auth/login/finish", jsonInit("POST", assertion)),

  authRecover: (code: string) =>
    request<RecoverResult>("/api/v1/auth/recover", jsonInit("POST", { code })),

  authLogout: () => request<void>("/api/v1/auth/logout", { method: "POST" }),

  listPasskeys: () => request<PasskeyInfo[]>("/api/v1/auth/passkeys"),

  deletePasskey: (id: string) =>
    request<void>(`/api/v1/auth/passkeys/${encodeURIComponent(id)}`, {
      method: "DELETE",
    }),
};
