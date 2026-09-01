// Share dialog for one document: lists its ACTIVE share links (copy / revoke /
// view count) and creates new ones with an expiry choice. Created links are
// auto-copied to the clipboard with a toast. Opened from the doc header or the
// ">share" palette command; the subject path lives in UiContext.
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Check, Copy, Link2, Trash2 } from "lucide-react";
import { useState } from "react";
import { api } from "../api/client.ts";
import { queryKeys, useShares } from "../api/queries.ts";
import type { ShareInfo } from "../api/types.ts";
import { formatRelativeTime, formatShortDate, todayISO } from "../lib/dates.ts";
import { useUi } from "../keys/UiContext.tsx";
import { ErrorState } from "./EmptyState.tsx";
import { Modal } from "./Modal.tsx";
import { SkeletonRows } from "./Skeleton.tsx";

type Expiry = "never" | 7 | 30;

/** Active = not revoked and not past its expiry. */
function isActiveShare(share: ShareInfo): boolean {
  if (share.revoked_at) return false;
  if (share.expires_at && new Date(share.expires_at).getTime() < Date.now()) {
    return false;
  }
  return true;
}

export function ShareDialog() {
  const { shareDocPath, setShareDocPath } = useUi();
  const close = () => setShareDocPath(null);
  return (
    <Modal
      open={shareDocPath !== null}
      onClose={close}
      variant="sheet"
      label="Share document"
    >
      {shareDocPath !== null ? <ShareContent path={shareDocPath} /> : null}
    </Modal>
  );
}

function ShareContent({ path }: { path: string }) {
  const { toast } = useUi();
  const queryClient = useQueryClient();
  const shares = useShares();
  const [expiry, setExpiry] = useState<Expiry>("never");

  const create = useMutation({
    mutationFn: () =>
      api.createShare(path, expiry === "never" ? undefined : expiry),
    onSuccess: async (share) => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.shares });
      await navigator.clipboard.writeText(share.url).catch(() => {});
      toast("Share link copied to clipboard");
    },
  });

  const revoke = useMutation({
    mutationFn: (token: string) => api.revokeShare(token),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: queryKeys.shares }),
  });

  const active = (shares.data ?? []).filter(
    (share) => share.doc_path === path && isActiveShare(share),
  );

  return (
    <div className="flex flex-col">
      <header className="flex items-center gap-2 border-b border-border px-3 py-2.5">
        <Link2 className="size-4 shrink-0 text-muted" aria-hidden="true" />
        <h2 className="text-sm font-semibold text-heading">Share</h2>
        <span className="truncate font-mono text-[10px] text-muted">
          {path}
        </span>
      </header>

      <div className="flex min-h-11 flex-wrap items-center gap-1.5 border-b border-border px-3 py-2">
        <span className="text-xs text-muted">Expires:</span>
        {(
          [
            ["never", "Never"],
            [7, "7 days"],
            [30, "30 days"],
          ] as const
        ).map(([value, label]) => (
          <button
            key={label}
            type="button"
            onClick={() => setExpiry(value)}
            className={`h-7 rounded-full border px-2.5 text-xs ${
              expiry === value
                ? "border-accent bg-selected text-heading"
                : "border-border text-muted hover:bg-hover hover:text-body"
            }`}
          >
            {label}
          </button>
        ))}
        <button
          type="button"
          onClick={() => create.mutate()}
          disabled={create.isPending}
          className="ml-auto h-7 rounded border border-border bg-accent px-2.5 text-xs font-medium text-white hover:opacity-90 disabled:opacity-50"
        >
          {create.isPending ? "Creating…" : "Create link"}
        </button>
      </div>
      {create.isError ? (
        <p className="border-b border-border px-3 py-2 text-xs text-danger">
          Couldn't create — {create.error.message}
        </p>
      ) : null}

      {shares.isPending ? (
        <SkeletonRows count={2} />
      ) : shares.isError ? (
        <div className="p-3">
          <ErrorState error={shares.error} />
        </div>
      ) : active.length === 0 ? (
        <p className="px-3 py-4 text-center text-xs text-muted">
          No active links for this document.
        </p>
      ) : (
        <ul className="divide-y divide-border">
          {active.map((share) => (
            <ShareRow
              key={share.token}
              share={share}
              onRevoke={() => revoke.mutate(share.token)}
            />
          ))}
        </ul>
      )}
    </div>
  );
}

function ShareRow({
  share,
  onRevoke,
}: {
  share: ShareInfo;
  onRevoke: () => void;
}) {
  const [copied, setCopied] = useState(false);
  const copy = () => {
    void navigator.clipboard.writeText(share.url).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1_500);
    });
  };
  return (
    <li className="flex items-center gap-2 px-3 py-2">
      <div className="min-w-0 flex-1">
        <p className="truncate font-mono text-xs text-heading">{share.url}</p>
        <p className="text-[10px] text-muted">
          created {formatRelativeTime(share.created_at)} · {share.view_count}{" "}
          {share.view_count === 1 ? "view" : "views"}
          {share.expires_at
            ? ` · expires ${formatShortDate(share.expires_at.slice(0, 10), todayISO())}`
            : ""}
        </p>
      </div>
      <button
        type="button"
        onClick={copy}
        aria-label="Copy link"
        className="flex size-7 shrink-0 items-center justify-center rounded border border-border text-muted hover:bg-hover hover:text-heading"
      >
        {copied ? (
          <Check className="size-3.5 text-ok" aria-hidden="true" />
        ) : (
          <Copy className="size-3.5" aria-hidden="true" />
        )}
      </button>
      <button
        type="button"
        onClick={onRevoke}
        aria-label="Revoke link"
        className="flex size-7 shrink-0 items-center justify-center rounded border border-border text-muted hover:bg-hover hover:text-danger"
      >
        <Trash2 className="size-3.5" aria-hidden="true" />
      </button>
    </li>
  );
}
