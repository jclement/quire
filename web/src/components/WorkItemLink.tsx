// One work-item reference ("AB#2433", "#2433") as the owner configured it:
// a link to their tracker when Settings has a URL template, otherwise the
// text as written. Shared by rendered markdown and one-line task rows so a
// ticket number looks the same wherever it appears.
import type { ReactNode } from "react";
import { useWorkItemTemplate } from "../api/queries.ts";
import { workItemUrl } from "../lib/workItems.ts";

export function WorkItemLink({
  id,
  children,
}: {
  id: string;
  children: ReactNode;
}) {
  const href = workItemUrl(useWorkItemTemplate(), id);
  if (!href) return <>{children}</>;
  return (
    <a
      href={href}
      target="_blank"
      rel="noreferrer"
      title={`Work item ${id}`}
      // Inside a task row the row itself is clickable (it selects); the
      // link is its own destination.
      onClick={(event) => event.stopPropagation()}
      className="rounded bg-hover px-1 font-mono text-[0.85em] text-accent no-underline hover:underline"
    >
      {children}
    </a>
  );
}
