// Project statuses as the UI shows them: label, colour, and the board's
// rules about which closed projects are worth a column's space. The server
// decides what a file's status *is* (internal/service/projects.go); this is
// only presentation and the board's arithmetic.
import type { DocMeta, ProjectStatus } from "../api/types.ts";

export interface StatusInfo {
  status: ProjectStatus;
  label: string;
  /** An area palette name, so statuses sit in the same visual family. */
  color: string;
  /** A closed status: done with, one way or the other. */
  closed: boolean;
}

/** Every status, in board order. */
export const PROJECT_STATUSES: StatusInfo[] = [
  { status: "planned", label: "Planned", color: "slate", closed: false },
  { status: "active", label: "Active", color: "blue", closed: false },
  { status: "hold", label: "On hold", color: "amber", closed: false },
  { status: "complete", label: "Complete", color: "green", closed: true },
  { status: "cancelled", label: "Cancelled", color: "red", closed: true },
];

export const STATUS_INFO = Object.fromEntries(
  PROJECT_STATUSES.map((info) => [info.status, info]),
) as Record<ProjectStatus, StatusInfo>;

/** A project's status; anything without project fields reads as active. */
export function statusOf(doc: DocMeta): ProjectStatus {
  return doc.project?.status ?? "active";
}

/** How long a closed project stays on the board before it folds away. */
export const RECENT_CLOSED_DAYS = 15;

/**
 * Whether a closed project closed recently enough to show. The stamped
 * completed:/cancelled: date says when; a project closed by hand without
 * one falls back to when its file last changed.
 */
export function closedRecently(
  doc: DocMeta,
  now: Date = new Date(),
  days = RECENT_CLOSED_DAYS,
): boolean {
  const closed = doc.project?.closed;
  const when = closed ? new Date(`${closed}T23:59:59`) : new Date(doc.mtime);
  return now.getTime() - when.getTime() <= days * 86_400_000;
}

/** Board order within a column: ranked first by rank, then newest first. */
export function compareForBoard(a: DocMeta, b: DocMeta): number {
  const ra = a.project?.rank ?? null;
  const rb = b.project?.rank ?? null;
  if (ra !== null && rb !== null && ra !== rb) return ra - rb;
  if (ra !== null && rb === null) return -1;
  if (ra === null && rb !== null) return 1;
  return b.mtime.localeCompare(a.mtime);
}

/** Spacing between ranks when a column is numbered afresh. */
export const RANK_STEP = 1000;

/**
 * The rank writes that put a column into the given order: just the moved
 * project when there is room between its neighbours, or the whole column
 * renumbered when there is not (a column nobody has ordered yet has no
 * ranks at all). Returns path → rank for the documents whose rank changes.
 */
export function rankWrites(
  column: DocMeta[],
  movedPath: string,
): Map<string, number> {
  const at = column.findIndex((doc) => doc.path === movedPath);
  const rankAt = (i: number) => column[i]?.project?.rank ?? null;
  const before = at > 0 ? rankAt(at - 1) : undefined;
  const after = at < column.length - 1 ? rankAt(at + 1) : undefined;
  const writes = new Map<string, number>();
  if (at < 0) return writes;

  let rank: number | null = null;
  if (before === undefined && after === undefined) rank = RANK_STEP;
  else if (before === undefined && after !== null && after !== undefined)
    rank = after - RANK_STEP;
  else if (after === undefined && before !== null && before !== undefined)
    rank = before + RANK_STEP;
  else if (
    before !== null &&
    after !== null &&
    before !== undefined &&
    after !== undefined &&
    after - before > 1e-6
  ) {
    const mid = (before + after) / 2;
    // Prefer a round number when one fits: ranks are read by people too.
    const round = Math.round(mid);
    rank = round > before && round < after ? round : mid;
  }
  if (rank !== null) {
    writes.set(movedPath, rank);
    return writes;
  }
  column.forEach((doc, i) => {
    const next = (i + 1) * RANK_STEP;
    if (doc.path === movedPath || doc.project?.rank !== next) {
      writes.set(doc.path, next);
    }
  });
  return writes;
}
