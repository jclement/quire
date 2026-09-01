// Typed fetch client for /api/v1. All responses use the house envelope
// ({"data":…} / {"error":{code,message}}); every non-2xx becomes a thrown
// ApiError carrying the server's code so callers can branch on e.g. CONFLICT
// without string-matching messages.
import type {
  AttachmentUpload,
  DocMeta,
  DocType,
  Document,
  Health,
  RenameResult,
  SearchResult,
  ShareInfo,
  Task,
  TaskEdit,
  TaskView,
  TodayPayload,
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
    const code = body?.error?.code ?? "UNKNOWN";
    const message =
      body?.error?.message ?? `Request failed (${response.status})`;
    throw new ApiError(code, message, response.status);
  }
  return body.data as T;
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
}

export const api = {
  health: () => request<Health>("/api/v1/health"),

  listDocuments: (params: ListDocumentsParams = {}) => {
    const query = new URLSearchParams();
    if (params.type) query.set("type", params.type);
    if (params.q) query.set("q", params.q);
    if (params.limit) query.set("limit", String(params.limit));
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

  createDocument: (type: DocType, title: string, markdown?: string) =>
    request<Document>(
      "/api/v1/documents",
      jsonInit("POST", { type, title, ...(markdown ? { markdown } : {}) }),
    ),

  deleteDocument: (path: string) =>
    request<void>(`/api/v1/documents/${encodeVaultPath(path)}`, {
      method: "DELETE",
    }),

  search: (q: string) =>
    request<SearchResult[]>(`/api/v1/search?q=${encodeURIComponent(q)}`),

  listTasks: (view: TaskView) => request<Task[]>(`/api/v1/tasks?view=${view}`),

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

  createDaily: (date: string) =>
    request<Document>(`/api/v1/daily/${date}`, { method: "POST" }),

  today: () => request<TodayPayload>("/api/v1/today"),

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

  rename: (path: string, newPath: string, rewriteLinks: boolean) =>
    request<RenameResult>(
      "/api/v1/rename",
      jsonInit("POST", { path, new_path: newPath, rewrite_links: rewriteLinks }),
    ),

  uploadAttachment: (file: File) => {
    const form = new FormData();
    form.append("file", file);
    return request<AttachmentUpload>("/api/v1/attachments", {
      method: "POST",
      body: form,
    });
  },
};
