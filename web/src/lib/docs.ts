// Per-doc-type UI metadata (labels, icons, browse routes) and vault-path
// helpers, so every view renders a "person" or a "meeting" the same way.
import {
  Building2,
  CalendarDays,
  FileText,
  FolderKanban,
  LayoutTemplate,
  NotebookPen,
  Users,
  type LucideIcon,
  CalendarRange,
} from "lucide-react";
import type { DocType } from "../api/types.ts";

export interface DocTypeInfo {
  label: string;
  plural: string;
  icon: LucideIcon;
}

export const DOC_TYPE_INFO: Record<DocType, DocTypeInfo> = {
  note: { label: "Note", plural: "Notes", icon: FileText },
  person: { label: "Person", plural: "People", icon: Users },
  company: { label: "Company", plural: "Companies", icon: Building2 },
  project: { label: "Project", plural: "Projects", icon: FolderKanban },
  meeting: { label: "Meeting", plural: "Meetings", icon: CalendarDays },
  daily: { label: "Daily", plural: "Daily", icon: NotebookPen },
  weekly: { label: "Weekly", plural: "Weekly", icon: CalendarRange },
  template: { label: "Template", plural: "Templates", icon: LayoutTemplate },
};

export const DOC_TYPES = Object.keys(DOC_TYPE_INFO) as DocType[];

export function isDocType(value: string): value is DocType {
  return value in DOC_TYPE_INFO;
}

/** SPA route for a vault document path. */
export function docHref(path: string): string {
  return `/doc/${path}`;
}

/** Vault path of a daily note for an ISO date. */
export function dailyPath(date: string): string {
  return `daily/${date}.md`;
}

/**
 * The vault path a route is showing, or null when the route isn't a document
 * page — how context-sensitive palette commands (share, rename) find their
 * subject.
 */
export function vaultPathFromRoute(pathname: string): string | null {
  const decoded = decodeURIComponent(pathname);
  if (decoded.startsWith("/doc/")) return decoded.slice("/doc/".length) || null;
  if (decoded.startsWith("/daily/")) {
    const date = decoded.slice("/daily/".length);
    return date ? dailyPath(date) : null;
  }
  return null;
}
