// Settings (/settings): everything that has access to the vault and how to
// revoke it — passkeys, API tokens, connected OAuth apps, share links —
// plus the agent guidance every MCP client is told to follow.
//
// Passkey management is meaningful only when the server runs passkey auth;
// in mode "none" the auth/status call 404s and a calm note renders instead.
// Server configuration itself lives in config.yaml, not here.
import { Link as RouterLink } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Fingerprint,
  LogOut,
  Plus,
  Settings as SettingsIcon,
  Trash2,
} from "lucide-react";
import { useState } from "react";
import { api, errorMessage } from "../api/client.ts";
import {
  queryKeys,
  useHealth,
  useSemanticEnabled,
  useSemanticStatus,
} from "../api/queries.ts";
import { AUTH_STATUS_KEY } from "../components/auth/AuthGate.tsx";
import { RegisterPanel } from "../components/auth/AuthScreens.tsx";
import { formatRelativeTime } from "../lib/dates.ts";
import { docHref } from "../lib/docs.ts";
import { noAutofill } from "../lib/noAutofill.ts";
import { useUi } from "../keys/UiContext.tsx";
import { EmptyState } from "../components/EmptyState.tsx";
import { ConfirmButton } from "../components/settings/ConfirmButton.tsx";
import {
  AgentActivity,
  AreaSettings,
  ConnectedAppSettings,
  ShareSettings,
  TemplateSettings,
  TokenSettings,
} from "../components/settings/Credentials.tsx";
import { SkeletonRows } from "../components/Skeleton.tsx";

const PASSKEYS_KEY = ["auth", "passkeys"] as const;
const GUIDANCE_KEY = ["agent-guidance"] as const;

const GUIDANCE_PLACEHOLDER =
  "File client work under projects/. Use British spelling. " +
  "Never edit meeting notes older than a week.";

export function SettingsPage() {
  const authStatus = useQuery({
    queryKey: AUTH_STATUS_KEY,
    queryFn: api.authStatus,
    staleTime: Infinity,
    retry: false,
    // AuthGate already fetched this; mounting a second observer must not
    // re-run a query that legitimately failed (auth disabled → 404).
    retryOnMount: false,
  });

  return (
    <div className="flex max-w-2xl flex-col gap-5">
      <header className="flex items-center gap-2 border-b border-border pb-2">
        <SettingsIcon className="size-4 text-muted" aria-hidden="true" />
        <h1 className="text-lg font-semibold text-heading">Settings</h1>
      </header>
      {authStatus.isError ? (
        <EmptyState
          icon={Fingerprint}
          title="Authentication is not enabled on this server"
          hint="Runs in local mode (QUIRE_AUTH_MODE=none) — nothing to manage here."
        />
      ) : (
        <PasskeySettings />
      )}
      <TokenSettings />
      <ConnectedAppSettings />
      <ShareSettings />
      <AreaSettings />
      <TemplateSettings />
      <AgentGuidanceSection />
      <SemanticSettings />
      <EmailSettings />
      <AgentActivity />
      <AboutSection />
    </div>
  );
}

// The one piece of server behavior that is genuinely the owner's to write:
// this text is handed to every MCP client as part of the server's
// instructions, so it is how you tell an agent the things the API can't —
// where work belongs, what never to touch. It is stored as a normal vault
// document, which is why the link at the bottom opens it in the editor.
function AgentGuidanceSection() {
  const queryClient = useQueryClient();
  const { toast } = useUi();
  const guidance = useQuery({
    queryKey: GUIDANCE_KEY,
    queryFn: api.agentGuidance,
  });
  // null = untouched, so the textarea keeps following the server until edited.
  const [draft, setDraft] = useState<string | null>(null);

  const save = useMutation({
    mutationFn: (text: string) => api.setAgentGuidance(text),
    onSuccess: (saved) => {
      queryClient.setQueryData(GUIDANCE_KEY, saved);
      void queryClient.invalidateQueries({
        queryKey: queryKeys.document(saved.path),
      });
      setDraft(null);
      toast("Agent guidance saved");
    },
  });

  const path = guidance.data?.path;
  const text = draft ?? guidance.data?.text ?? "";
  return (
    <section className="border-t border-border pt-4">
      <h2 className="mb-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted">
        Agent guidance
      </h2>
      <p className="mb-2 text-xs text-muted">
        House rules for AI agents working in the vault. quire sends this to
        every MCP client as part of the server's instructions, so saving it
        reaches the next agent session — nothing needs restarting.
      </p>
      {guidance.isPending ? (
        <SkeletonRows count={3} />
      ) : guidance.isError ? (
        <p className="text-xs text-danger">{errorMessage(guidance.error)}</p>
      ) : (
        <>
          <textarea
            rows={10}
            value={text}
            onChange={(event) => setDraft(event.target.value)}
            placeholder={GUIDANCE_PLACEHOLDER}
            aria-label="Agent guidance"
            {...noAutofill("agent-guidance")}
            className="field-bare w-full rounded border border-border bg-raised p-2 font-mono text-xs leading-relaxed text-body outline-none placeholder:text-muted focus:border-accent"
          />
          <div className="mt-2 flex items-center gap-2">
            <button
              type="button"
              onClick={() => save.mutate(text)}
              disabled={draft === null || save.isPending}
              className="flex h-8 items-center rounded border border-border px-2.5 text-xs text-body hover:bg-hover hover:text-heading disabled:opacity-50"
            >
              {save.isPending ? "Saving…" : "Save"}
            </button>
            {save.isError ? (
              <span className="text-xs text-danger">
                {errorMessage(save.error)}
              </span>
            ) : null}
            {path ? (
              <span className="ml-auto truncate text-xs text-muted">
                Stored as{" "}
                {guidance.data.text ? (
                  <RouterLink
                    to={docHref(path)}
                    className="font-mono text-accent hover:underline"
                  >
                    {path}
                  </RouterLink>
                ) : (
                  <span className="font-mono">{path}</span>
                )}{" "}
                in the vault. Clearing it deletes the file.
              </span>
            ) : null}
          </div>
        </>
      )}
    </section>
  );
}

// Version and update status used to sit in a page footer; it belongs here,
// where it costs no screen space on a phone.
/** Email is configured by environment; this shows what is set and sends a
 * test digest so a bad relay is found here, not by a missing morning mail. */
function EmailSettings() {
  const { toast } = useUi();
  const status = useQuery({ queryKey: ["email"], queryFn: api.emailStatus });
  const send = useMutation({
    mutationFn: api.sendTestEmail,
    onSuccess: () => toast("Test email sent"),
    onError: (error) => toast(errorMessage(error)),
  });
  return (
    <section className="flex flex-col gap-2">
      <h2 className="text-sm font-semibold text-heading">Email</h2>
      {!status.data?.configured ? (
        <p className="text-xs text-muted">
          Off. Set <code className="font-mono">QUIRE_SMTP_HOST</code>,{" "}
          <code className="font-mono">QUIRE_SMTP_FROM</code> and{" "}
          <code className="font-mono">QUIRE_DIGEST_TO</code> for a morning
          digest of meetings, birthdays and dated tasks.
        </p>
      ) : (
        <div className="flex flex-wrap items-end gap-4">
          <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-xs">
            <dt className="text-muted">From</dt>
            <dd className="font-mono text-heading">{status.data.from}</dd>
            <dt className="text-muted">Digest to</dt>
            <dd className="font-mono text-heading">{status.data.digest_to}</dd>
            <dt className="text-muted">Sent at</dt>
            <dd className="text-heading">
              {status.data.digest_time || "not scheduled (QUIRE_DIGEST_TIME)"}
            </dd>
          </dl>
          <button
            type="button"
            onClick={() => send.mutate()}
            disabled={send.isPending}
            className="flex h-8 items-center rounded border border-border px-2.5 text-xs text-body hover:bg-hover hover:text-heading disabled:opacity-50"
          >
            {send.isPending ? "Sending…" : "Send test email"}
          </button>
        </div>
      )}
    </section>
  );
}

/** Semantic search is configured by environment, not here; this shows
 * whether it is on and how the embedding backlog is doing. */
function SemanticSettings() {
  const enabled = useSemanticEnabled();
  const status = useSemanticStatus(enabled);
  return (
    <section className="flex flex-col gap-2">
      <h2 className="text-sm font-semibold text-heading">Semantic search</h2>
      {!enabled ? (
        <p className="text-xs text-muted">
          Off. Set <code className="font-mono">QUIRE_OPENAI_API_KEY</code> to
          search by meaning — note text is then sent to that embeddings endpoint
          (OpenAI, or any compatible server via{" "}
          <code className="font-mono">QUIRE_OPENAI_BASE_URL</code>).
        </p>
      ) : (
        <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-xs">
          <dt className="text-muted">Model</dt>
          <dd className="font-mono text-heading">
            {status.data?.model ?? "…"}
          </dd>
          <dt className="text-muted">Embedded</dt>
          <dd className="text-heading" data-testid="semantic-documents">
            {status.data ? `${status.data.documents} documents` : "…"}
            {status.data?.pending ? ` · ${status.data.pending} pending` : ""}
          </dd>
          {status.data?.last_error ? (
            <>
              <dt className="text-muted">Last error</dt>
              <dd className="text-danger">{status.data.last_error}</dd>
            </>
          ) : null}
        </dl>
      )}
    </section>
  );
}

function AboutSection() {
  const health = useHealth();
  return (
    <section className="border-t border-border pt-3 text-xs text-muted">
      <div className="flex items-center justify-between">
        <span>quire</span>
        <span className="font-mono">
          {health.data?.version ? `v${health.data.version}` : "…"}
        </span>
      </div>
      {health.data?.update_available ? (
        <p className="mt-1 text-warn">An update is available.</p>
      ) : null}
    </section>
  );
}

function PasskeySettings() {
  const queryClient = useQueryClient();
  const { toast } = useUi();
  const [adding, setAdding] = useState(false);
  const passkeys = useQuery({
    queryKey: PASSKEYS_KEY,
    queryFn: api.listPasskeys,
  });

  const remove = useMutation({
    mutationFn: (id: string) => api.deletePasskey(id),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: PASSKEYS_KEY }),
  });

  const logout = useMutation({
    mutationFn: api.authLogout,
    onSuccess: () => {
      // Drop the session state; AuthGate takes over with the login screen.
      queryClient.setQueryData(AUTH_STATUS_KEY, {
        registered: true,
        authenticated: false,
      });
      queryClient.removeQueries({
        predicate: (query) => query.queryKey[0] !== "auth",
      });
    },
  });

  return (
    <>
      <section>
        <h2 className="mb-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted">
          Passkeys
        </h2>
        {passkeys.isPending ? (
          <SkeletonRows count={2} />
        ) : passkeys.isError ? (
          <p className="text-xs text-danger">{errorMessage(passkeys.error)}</p>
        ) : (
          <ul className="divide-y divide-border border-y border-border">
            {passkeys.data.map((passkey) => (
              <li key={passkey.id} className="flex h-9 items-center gap-2 px-2">
                <Fingerprint
                  className="size-3.5 shrink-0 text-muted"
                  aria-hidden="true"
                />
                <span className="truncate text-sm text-body">
                  {passkey.name}
                </span>
                <span className="ml-auto shrink-0 text-xs text-muted">
                  added {formatRelativeTime(passkey.created_at)}
                </span>
                <ConfirmButton
                  label={`Delete passkey ${passkey.name}`}
                  confirmLabel="Delete?"
                  onConfirm={() => remove.mutate(passkey.id)}
                >
                  <Trash2 className="size-3.5" aria-hidden="true" />
                </ConfirmButton>
              </li>
            ))}
          </ul>
        )}
        {remove.isError ? (
          <p className="mt-1.5 text-xs text-danger">
            {errorMessage(remove.error)}
          </p>
        ) : null}
        {adding ? (
          <div className="mt-3 rounded border border-border bg-raised p-3">
            <RegisterPanel
              onDone={() => {
                setAdding(false);
                toast("Passkey added");
                void queryClient.invalidateQueries({ queryKey: PASSKEYS_KEY });
              }}
            />
          </div>
        ) : (
          <button
            type="button"
            onClick={() => setAdding(true)}
            className="mt-2 flex h-8 items-center gap-1.5 rounded border border-border px-2.5 text-xs text-body hover:bg-hover hover:text-heading"
          >
            <Plus className="size-3.5" aria-hidden="true" />
            Add passkey
          </button>
        )}
      </section>

      <section className="border-t border-border pt-4">
        <button
          type="button"
          onClick={() => logout.mutate()}
          disabled={logout.isPending}
          className="flex h-8 items-center gap-1.5 rounded border border-border px-2.5 text-xs text-body hover:bg-hover hover:text-danger disabled:opacity-50"
        >
          <LogOut className="size-3.5" aria-hidden="true" />
          {logout.isPending ? "Signing out…" : "Sign out"}
        </button>
      </section>
    </>
  );
}
