// 404 — light-hearted and theme-aware (all colors come from the semantic
// tokens, so it dresses for dark mode like everything else).
import { Link as RouterLink } from "@tanstack/react-router";
import { BookX } from "lucide-react";

export function NotFoundPage() {
  return (
    <div className="flex flex-col items-center gap-3 py-24 text-center">
      <BookX className="size-8 text-muted" aria-hidden="true" />
      <p className="font-mono text-xs text-muted">404</p>
      <h1 className="text-lg font-semibold text-heading">
        This page slipped out of the quire.
      </h1>
      <p className="max-w-sm text-sm text-muted">
        A quire is twenty-five sheets of paper, and none of them are this one.
        Try the palette (
        <kbd className="rounded border border-border bg-hover px-1 font-mono text-[10px]">
          ⌘K
        </kbd>
        ) — it knows where everything is.
      </p>
      <RouterLink
        to="/today"
        className="mt-2 flex h-8 items-center rounded border border-border px-3 text-sm text-body hover:bg-hover hover:text-heading"
      >
        Back to Today
      </RouterLink>
    </div>
  );
}
