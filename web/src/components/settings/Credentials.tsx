// Everything that currently has access to the vault, and how to take it
// away: API tokens, connected OAuth apps, and live share links.
//
// This exists because the honest answer to "what can reach my notes?" used
// to require SSH and sqlite3 — tokens were CLI-only and OAuth clients were
// invisible entirely. Revocation is the point, so every row has a way out.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link as RouterLink } from "@tanstack/react-router";
import { Check, Copy, KeyRound, Link2, Plug, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { api, errorMessage } from "../../api/client.ts";
import type { NewToken } from "../../api/types.ts";
import { formatRelativeTime } from "../../lib/dates.ts";
import { docHref } from "../../lib/docs.ts";
import { noAutofill } from "../../lib/noAutofill.ts";
import { useUi } from "../../keys/UiContext.tsx";
import { ConfirmButton } from "./ConfirmButton.tsx";
import { SkeletonRows } from "../Skeleton.tsx";

const TOKENS_KEY = ["auth", "tokens"] as const;
const APPS_KEY = ["auth", "connected-apps"] as const;
const SHARES_KEY = ["shares"] as const;

/** The three scopes a token can carry, with what each actually permits. */
const SCOPES = [
  { id: "read", label: "Read", hint: "search and read documents" },
  { id: "write", label: "Write", hint: "create and edit documents" },
  { id: "tasks", label: "Tasks", hint: "create and complete tasks" },
] as const;

function SectionHeading({ children }: { children: React.ReactNode }) {
  return (
    <h2 className="mb-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted">
      {children}
    </h2>
  );
}

/** A muted "never" rather than an empty cell, so columns stay readable. */
function When({ iso, never = "never" }: { iso: string; never?: string }) {
  return <>{iso ? formatRelativeTime(iso) : never}</>;
}

// ---- API tokens ----

export function TokenSettings() {
  const queryClient = useQueryClient();
  const { toast } = useUi();
  const [creating, setCreating] = useState(false);
  const [minted, setMinted] = useState<NewToken | null>(null);

  const tokens = useQuery({ queryKey: TOKENS_KEY, queryFn: api.listTokens });
  const revoke = useMutation({
    mutationFn: (prefix: string) => api.revokeToken(prefix),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: TOKENS_KEY });
      toast("Token revoked");
    },
  });

  // Revoked tokens stay listed as an audit trail, but sort below live ones.
  const rows = (tokens.data ?? []).slice().sort((a, b) => {
    const revokedDiff = Number(!!a.revoked_at) - Number(!!b.revoked_at);
    return revokedDiff !== 0
      ? revokedDiff
      : b.created_at.localeCompare(a.created_at);
  });

  return (
    <section className="border-t border-border pt-4">
      <SectionHeading>API tokens</SectionHeading>
      <p className="mb-2 text-xs text-muted">
        For Claude Code, scripts, and anything else that pastes a header. Scopes
        are enforced: a read-only token cannot write, over the API or over MCP.
      </p>

      {minted ? (
        <MintedToken minted={minted} onDone={() => setMinted(null)} />
      ) : null}

      {tokens.isPending ? (
        <SkeletonRows count={2} />
      ) : tokens.isError ? (
        <p className="text-xs text-danger">{errorMessage(tokens.error)}</p>
      ) : rows.length === 0 ? (
        <p className="border-y border-border py-3 text-xs text-muted">
          No tokens yet.
        </p>
      ) : (
        <ul className="divide-y divide-border border-y border-border">
          {rows.map((token) => (
            <li
              key={token.id}
              className={`flex h-9 items-center gap-2 px-2 ${token.revoked_at ? "opacity-50" : ""}`}
            >
              <KeyRound
                className="size-3.5 shrink-0 text-muted"
                aria-hidden="true"
              />
              <span className="truncate text-sm text-body">{token.name}</span>
              <span className="shrink-0 font-mono text-xs text-muted">
                {token.prefix}…
              </span>
              <span className="hidden shrink-0 gap-1 sm:flex">
                {token.scopes.map((scope) => (
                  <span
                    key={scope}
                    className="rounded border border-border px-1 text-[10px] text-muted"
                  >
                    {scope}
                  </span>
                ))}
              </span>
              <span className="ml-auto shrink-0 text-xs text-muted">
                {token.revoked_at ? (
                  "revoked"
                ) : (
                  <>
                    used <When iso={token.last_used_at} />
                  </>
                )}
              </span>
              {token.revoked_at ? null : (
                <ConfirmButton
                  label={`Revoke token ${token.name}`}
                  confirmLabel="Revoke?"
                  onConfirm={() => revoke.mutate(token.prefix)}
                >
                  <Trash2 className="size-3.5" aria-hidden="true" />
                </ConfirmButton>
              )}
            </li>
          ))}
        </ul>
      )}
      {revoke.isError ? (
        <p className="mt-1.5 text-xs text-danger">
          {errorMessage(revoke.error)}
        </p>
      ) : null}

      {creating ? (
        <CreateTokenForm
          onCancel={() => setCreating(false)}
          onCreated={(result) => {
            setCreating(false);
            setMinted(result);
            void queryClient.invalidateQueries({ queryKey: TOKENS_KEY });
          }}
        />
      ) : (
        <button
          type="button"
          onClick={() => setCreating(true)}
          className="mt-2 flex h-8 items-center gap-1.5 rounded border border-border px-2.5 text-xs text-body hover:bg-hover hover:text-heading"
        >
          <Plus className="size-3.5" aria-hidden="true" />
          New token
        </button>
      )}
    </section>
  );
}

/** The plaintext exists exactly once; this is that once. */
function MintedToken({
  minted,
  onDone,
}: {
  minted: NewToken;
  onDone: () => void;
}) {
  const [copied, setCopied] = useState(false);
  const copy = () => {
    void navigator.clipboard.writeText(minted.plaintext).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2_000);
    });
  };
  return (
    <div className="mb-3 rounded border border-accent bg-raised p-3">
      <p className="mb-2 text-xs text-heading">
        Copy <strong>{minted.token.name}</strong> now — this is the only time it
        is shown.
      </p>
      <div className="flex items-center gap-2">
        <code className="min-w-0 flex-1 truncate rounded border border-border bg-surface px-2 py-1.5 font-mono text-xs text-body">
          {minted.plaintext}
        </code>
        <button
          type="button"
          onClick={copy}
          className="flex h-8 shrink-0 items-center gap-1.5 rounded border border-border px-2.5 text-xs text-body hover:bg-hover hover:text-heading"
        >
          {copied ? (
            <Check className="size-3.5" aria-hidden="true" />
          ) : (
            <Copy className="size-3.5" aria-hidden="true" />
          )}
          {copied ? "Copied" : "Copy"}
        </button>
      </div>
      <button
        type="button"
        onClick={onDone}
        className="mt-2 text-xs text-muted underline decoration-border underline-offset-2 hover:text-heading"
      >
        I've saved it
      </button>
    </div>
  );
}

function CreateTokenForm({
  onCreated,
  onCancel,
}: {
  onCreated: (result: NewToken) => void;
  onCancel: () => void;
}) {
  const [name, setName] = useState("");
  // read+tasks is the useful default: enough for an agent to run your todos
  // without letting it rewrite documents.
  const [scopes, setScopes] = useState<string[]>(["read", "tasks"]);
  const [expiresDays, setExpiresDays] = useState("");

  const create = useMutation({
    mutationFn: () =>
      api.createToken(name.trim(), scopes, Number(expiresDays) || undefined),
    onSuccess: onCreated,
  });

  const toggle = (scope: string) =>
    setScopes((current) =>
      current.includes(scope)
        ? current.filter((s) => s !== scope)
        : [...current, scope],
    );

  const valid = name.trim() !== "" && scopes.length > 0;
  return (
    <div className="mt-3 rounded border border-border bg-raised p-3">
      <label className="mb-1.5 block text-xs text-muted" htmlFor="token-name">
        What is it for?
      </label>
      <input
        id="token-name"
        value={name}
        onChange={(event) => setName(event.target.value)}
        placeholder="Claude Code on the laptop"
        {...noAutofill("token-name")}
        className="mb-3 h-9 w-full rounded border border-border bg-surface px-2.5 text-sm text-heading outline-none placeholder:text-muted focus:border-accent"
      />

      <span className="mb-1.5 block text-xs text-muted">Scopes</span>
      <div className="mb-3 flex flex-wrap gap-2">
        {SCOPES.map((scope) => (
          <label
            key={scope.id}
            title={scope.hint}
            className="flex cursor-pointer items-center gap-1.5 rounded border border-border px-2 py-1 text-xs text-body hover:bg-hover"
          >
            <input
              type="checkbox"
              checked={scopes.includes(scope.id)}
              onChange={() => toggle(scope.id)}
              className="accent-accent"
            />
            {scope.label}
          </label>
        ))}
      </div>

      <label className="mb-1.5 block text-xs text-muted" htmlFor="token-expiry">
        Expires after (days) — blank for never
      </label>
      <input
        id="token-expiry"
        type="number"
        min="1"
        value={expiresDays}
        onChange={(event) => setExpiresDays(event.target.value)}
        placeholder="never"
        {...noAutofill("token-expiry")}
        className="mb-3 h-9 w-28 rounded border border-border bg-surface px-2.5 text-sm text-heading outline-none placeholder:text-muted focus:border-accent"
      />

      <div className="flex items-center gap-2">
        <button
          type="button"
          disabled={!valid || create.isPending}
          onClick={() => create.mutate()}
          className="flex h-8 items-center rounded border border-border bg-accent px-2.5 text-xs font-medium text-white hover:opacity-90 disabled:opacity-50"
        >
          {create.isPending ? "Creating…" : "Create token"}
        </button>
        <button
          type="button"
          onClick={onCancel}
          className="flex h-8 items-center rounded border border-border px-2.5 text-xs text-body hover:bg-hover hover:text-heading"
        >
          Cancel
        </button>
        {create.isError ? (
          <span className="text-xs text-danger">
            {errorMessage(create.error)}
          </span>
        ) : null}
      </div>
    </div>
  );
}

// ---- connected apps (OAuth) ----

export function ConnectedAppSettings() {
  const queryClient = useQueryClient();
  const { toast } = useUi();
  const apps = useQuery({ queryKey: APPS_KEY, queryFn: api.listConnectedApps });
  const disconnect = useMutation({
    mutationFn: (clientId: string) => api.disconnectApp(clientId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: APPS_KEY });
      toast("App disconnected");
    },
  });

  return (
    <section className="border-t border-border pt-4">
      <SectionHeading>Connected apps</SectionHeading>
      <p className="mb-2 text-xs text-muted">
        Clients that completed the OAuth flow — claude.ai connectors, Claude
        Desktop. Disconnecting revokes their tokens immediately and forces a
        fresh approval next time.
      </p>
      {apps.isPending ? (
        <SkeletonRows count={2} />
      ) : apps.isError ? (
        <p className="text-xs text-danger">{errorMessage(apps.error)}</p>
      ) : apps.data.length === 0 ? (
        <p className="border-y border-border py-3 text-xs text-muted">
          Nothing connected.
        </p>
      ) : (
        <ul className="divide-y divide-border border-y border-border">
          {apps.data.map((app) => (
            <li
              key={app.client_id}
              className={`flex h-9 items-center gap-2 px-2 ${app.active_grant ? "" : "opacity-50"}`}
            >
              <Plug
                className="size-3.5 shrink-0 text-muted"
                aria-hidden="true"
              />
              <span className="truncate text-sm text-body">
                {app.name || app.client_id}
              </span>
              <span className="hidden shrink-0 gap-1 sm:flex">
                {app.scopes.map((scope) => (
                  <span
                    key={scope}
                    className="rounded border border-border px-1 text-[10px] text-muted"
                  >
                    {scope}
                  </span>
                ))}
              </span>
              <span className="ml-auto shrink-0 text-xs text-muted">
                {app.active_grant ? (
                  <>
                    used <When iso={app.last_used_at} />
                  </>
                ) : (
                  "grant expired"
                )}
              </span>
              <ConfirmButton
                label={`Disconnect ${app.name || app.client_id}`}
                confirmLabel="Disconnect?"
                onConfirm={() => disconnect.mutate(app.client_id)}
              >
                <Trash2 className="size-3.5" aria-hidden="true" />
              </ConfirmButton>
            </li>
          ))}
        </ul>
      )}
      {disconnect.isError ? (
        <p className="mt-1.5 text-xs text-danger">
          {errorMessage(disconnect.error)}
        </p>
      ) : null}
    </section>
  );
}

// ---- share links ----

export function ShareSettings() {
  const queryClient = useQueryClient();
  const { toast } = useUi();
  const shares = useQuery({ queryKey: SHARES_KEY, queryFn: api.listShares });
  const revoke = useMutation({
    mutationFn: (token: string) => api.revokeShare(token),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: SHARES_KEY });
      toast("Share link revoked");
    },
  });

  const live = (shares.data ?? []).filter((share) => !share.revoked_at);
  return (
    <section className="border-t border-border pt-4">
      <SectionHeading>Share links</SectionHeading>
      <p className="mb-2 text-xs text-muted">
        Every live link, in one place. Anyone holding one can read that document
        without signing in — the link is the secret.
      </p>
      {shares.isPending ? (
        <SkeletonRows count={2} />
      ) : shares.isError ? (
        <p className="text-xs text-danger">{errorMessage(shares.error)}</p>
      ) : live.length === 0 ? (
        <p className="border-y border-border py-3 text-xs text-muted">
          Nothing is shared.
        </p>
      ) : (
        <ul className="divide-y divide-border border-y border-border">
          {live.map((share) => (
            <li key={share.token} className="flex h-9 items-center gap-2 px-2">
              <Link2
                className="size-3.5 shrink-0 text-muted"
                aria-hidden="true"
              />
              <RouterLink
                to={docHref(share.doc_path)}
                className="truncate font-mono text-xs text-accent hover:underline"
              >
                {share.doc_path}
              </RouterLink>
              <span className="ml-auto shrink-0 text-xs text-muted">
                {share.view_count} view{share.view_count === 1 ? "" : "s"}
                {share.expires_at ? (
                  <>
                    {" · expires "}
                    <When iso={share.expires_at} />
                  </>
                ) : null}
              </span>
              <ConfirmButton
                label={`Revoke share link for ${share.doc_path}`}
                confirmLabel="Revoke?"
                onConfirm={() => revoke.mutate(share.token)}
              >
                <Trash2 className="size-3.5" aria-hidden="true" />
              </ConfirmButton>
            </li>
          ))}
        </ul>
      )}
      {revoke.isError ? (
        <p className="mt-1.5 text-xs text-danger">
          {errorMessage(revoke.error)}
        </p>
      ) : null}
    </section>
  );
}
