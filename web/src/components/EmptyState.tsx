// Empty/error placeholder used by every list and page. Also maps backend-down
// errors to a calm message so a dev frontend without the Go server never looks
// broken.
import { CloudOff, type LucideIcon } from "lucide-react";
import { ApiError } from "../api/client.ts";

interface EmptyStateProps {
  icon: LucideIcon;
  title: string;
  hint?: string;
}

export function EmptyState({ icon: Icon, title, hint }: EmptyStateProps) {
  return (
    <div className="flex flex-col items-center gap-2 border border-dashed border-border px-4 py-10 text-center">
      <Icon className="size-5 text-muted" aria-hidden="true" />
      <p className="text-sm text-heading">{title}</p>
      {hint ? <p className="text-xs text-muted">{hint}</p> : null}
    </div>
  );
}

/** Standard rendering for a failed query. */
export function ErrorState({ error }: { error: unknown }) {
  const unreachable = error instanceof ApiError && error.code === "UNREACHABLE";
  return (
    <EmptyState
      icon={CloudOff}
      title={
        unreachable ? "Can't reach the quire server" : "Something went wrong"
      }
      hint={
        unreachable
          ? "Start the server, then this page will load itself."
          : error instanceof Error
            ? error.message
            : undefined
      }
    />
  );
}
