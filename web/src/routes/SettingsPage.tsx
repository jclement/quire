// Settings (/settings): passkey management (list, add, delete) and logout —
// only meaningful when the server runs passkey auth; in mode "none" the
// auth/status call 404s and a calm note renders instead. Kept deliberately
// small; the server config itself lives in config.yaml, not here.
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
import { useHealth } from "../api/queries.ts";
import { AUTH_STATUS_KEY } from "../components/auth/AuthGate.tsx";
import { RegisterPanel } from "../components/auth/AuthScreens.tsx";
import { formatRelativeTime } from "../lib/dates.ts";
import { useUi } from "../keys/UiContext.tsx";
import { EmptyState } from "../components/EmptyState.tsx";
import { SkeletonRows } from "../components/Skeleton.tsx";

const PASSKEYS_KEY = ["auth", "passkeys"] as const;

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
    <div className="flex max-w-lg flex-col gap-5">
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
      <AboutSection />
    </div>
  );
}

// Version and update status used to sit in a page footer; it belongs here,
// where it costs no screen space on a phone.
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
                <button
                  type="button"
                  onClick={() => remove.mutate(passkey.id)}
                  aria-label={`Delete passkey ${passkey.name}`}
                  className="flex size-7 shrink-0 items-center justify-center rounded border border-border text-muted hover:bg-hover hover:text-danger"
                >
                  <Trash2 className="size-3.5" aria-hidden="true" />
                </button>
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
