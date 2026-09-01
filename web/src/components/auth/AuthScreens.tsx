// The screens behind AuthGate: sign-in (passkey button + recovery-code path),
// the first-run "claim this quire" bootstrap, passkey registration, and the
// one-time recovery-codes display. Calm and small — a centered column, no
// chrome. All ceremonies run through api/auth.ts.
import { Check, Copy, Fingerprint, KeyRound, ShieldCheck } from "lucide-react";
import { useState, type ReactNode } from "react";
import { api, errorMessage } from "../../api/client.ts";
import { loginWithPasskey, registerPasskey } from "../../api/auth.ts";

interface ScreenProps {
  onAuthed: () => void;
}

/** Shared centered-column chrome for every auth screen. */
function AuthFrame({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-dvh flex-col items-center justify-center gap-6 px-6 py-10">
      <span className="font-serif text-2xl font-semibold italic text-heading">
        quire
      </span>
      <div className="w-full max-w-sm">{children}</div>
    </div>
  );
}

function AuthError({ message }: { message: string | null }) {
  if (!message) return null;
  return <p className="mt-3 text-center text-xs text-danger">{message}</p>;
}

/** The browser throws NotAllowedError when the user dismisses the prompt —
 * that's a cancel, not a failure worth alarming anyone about. */
function ceremonyError(error: unknown): string {
  if (error instanceof DOMException && error.name === "NotAllowedError") {
    return "Passkey prompt was dismissed — try again when ready.";
  }
  return errorMessage(error);
}

// ---- Sign in ----

export function LoginScreen({ onAuthed }: ScreenProps) {
  const [view, setView] = useState<"passkey" | "recovery" | "add-passkey">(
    "passkey",
  );
  if (view === "recovery") {
    return (
      <RecoveryCodeEntry
        onAuthed={onAuthed}
        onRegisterPasskey={() => setView("add-passkey")}
        onBack={() => setView("passkey")}
      />
    );
  }
  if (view === "add-passkey") {
    // A recovery code just burned; the session exists — add a replacement key.
    return (
      <AuthFrame>
        <p className="mb-4 text-center text-sm text-body">
          Recovery code accepted. Add a new passkey for next time.
        </p>
        <RegisterPanel onDone={() => onAuthed()} />
      </AuthFrame>
    );
  }
  return (
    <PasskeyLogin onAuthed={onAuthed} onRecovery={() => setView("recovery")} />
  );
}

function PasskeyLogin({
  onAuthed,
  onRecovery,
}: ScreenProps & { onRecovery: () => void }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const signIn = async () => {
    setBusy(true);
    setError(null);
    try {
      await loginWithPasskey();
      onAuthed();
    } catch (caught) {
      setError(ceremonyError(caught));
    } finally {
      setBusy(false);
    }
  };

  return (
    <AuthFrame>
      <button
        type="button"
        onClick={() => void signIn()}
        disabled={busy}
        className="flex h-11 w-full items-center justify-center gap-2 rounded border border-border bg-accent text-sm font-medium text-white hover:opacity-90 disabled:opacity-50"
      >
        <Fingerprint className="size-4" aria-hidden="true" />
        {busy ? "Waiting for passkey…" : "Sign in with passkey"}
      </button>
      <AuthError message={error} />
      <button
        type="button"
        onClick={onRecovery}
        className="mx-auto mt-4 block text-xs text-muted underline decoration-border underline-offset-2 hover:text-heading"
      >
        use a recovery code
      </button>
    </AuthFrame>
  );
}

function RecoveryCodeEntry({
  onAuthed,
  onRegisterPasskey,
  onBack,
}: ScreenProps & { onRegisterPasskey: () => void; onBack: () => void }) {
  const [code, setCode] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = async () => {
    if (!code.trim() || busy) return;
    setBusy(true);
    setError(null);
    try {
      const result = await api.authRecover(code.trim());
      if (result.register_passkey) onRegisterPasskey();
      else onAuthed();
    } catch (caught) {
      setError(errorMessage(caught));
    } finally {
      setBusy(false);
    }
  };

  return (
    <AuthFrame>
      <label
        className="mb-1.5 block text-xs text-muted"
        htmlFor="recovery-code"
      >
        Recovery code
      </label>
      <div className="flex gap-2">
        <input
          id="recovery-code"
          autoFocus
          value={code}
          onChange={(event) => setCode(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter") void submit();
          }}
          spellCheck={false}
          autoComplete="off"
          className="h-10 w-full rounded border border-border bg-raised px-2.5 font-mono text-sm text-heading outline-none focus:border-accent"
        />
        <button
          type="button"
          onClick={() => void submit()}
          disabled={busy || !code.trim()}
          className="flex h-10 shrink-0 items-center gap-1.5 rounded border border-border px-3 text-sm text-body hover:bg-hover hover:text-heading disabled:opacity-50"
        >
          <KeyRound className="size-4" aria-hidden="true" />
          {busy ? "Checking…" : "Recover"}
        </button>
      </div>
      <p className="mt-2 text-xs text-muted">
        Each code works once; a used code should be replaced with a new passkey.
      </p>
      <AuthError message={error} />
      <button
        type="button"
        onClick={onBack}
        className="mx-auto mt-4 block text-xs text-muted underline decoration-border underline-offset-2 hover:text-heading"
      >
        back to passkey sign-in
      </button>
    </AuthFrame>
  );
}

// ---- First run: claim + recovery codes ----

export function WelcomeScreen({ onAuthed }: ScreenProps) {
  const [codes, setCodes] = useState<string[] | null>(null);
  if (codes) return <RecoveryCodesScreen codes={codes} onConfirm={onAuthed} />;
  return (
    <AuthFrame>
      <h1 className="mb-1 text-center text-lg font-semibold text-heading">
        Claim this quire
      </h1>
      <p className="mb-5 text-center text-xs text-muted">
        No one has registered yet. Create the first passkey and this instance is
        yours.
      </p>
      <RegisterPanel
        onDone={(recoveryCodes) => {
          if (recoveryCodes) setCodes(recoveryCodes);
          else onAuthed();
        }}
      />
    </AuthFrame>
  );
}

/** Name + create-passkey button; used by first-run, recovery, and Settings. */
export function RegisterPanel({
  onDone,
}: {
  onDone: (recoveryCodes: string[] | null) => void;
}) {
  const [name, setName] = useState("My passkey");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const register = async () => {
    if (busy) return;
    setBusy(true);
    setError(null);
    try {
      const codes = await registerPasskey(name.trim() || "Passkey");
      onDone(codes);
    } catch (caught) {
      setError(ceremonyError(caught));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div>
      <label className="mb-1.5 block text-xs text-muted" htmlFor="passkey-name">
        Passkey name
      </label>
      <input
        id="passkey-name"
        value={name}
        onChange={(event) => setName(event.target.value)}
        className="mb-3 h-10 w-full rounded border border-border bg-raised px-2.5 text-sm text-heading outline-none focus:border-accent"
      />
      <button
        type="button"
        onClick={() => void register()}
        disabled={busy}
        className="flex h-11 w-full items-center justify-center gap-2 rounded border border-border bg-accent text-sm font-medium text-white hover:opacity-90 disabled:opacity-50"
      >
        <Fingerprint className="size-4" aria-hidden="true" />
        {busy ? "Waiting for passkey…" : "Create passkey"}
      </button>
      <AuthError message={error} />
    </div>
  );
}

function RecoveryCodesScreen({
  codes,
  onConfirm,
}: {
  codes: string[];
  onConfirm: () => void;
}) {
  const [copied, setCopied] = useState(false);
  const copyAll = () => {
    void navigator.clipboard.writeText(codes.join("\n")).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2_000);
    });
  };
  return (
    <AuthFrame>
      <h1 className="mb-1 flex items-center justify-center gap-2 text-center text-lg font-semibold text-heading">
        <ShieldCheck className="size-5 text-ok" aria-hidden="true" />
        Save your recovery codes
      </h1>
      <p className="mb-4 text-center text-xs text-muted">
        These unlock quire if you lose every passkey. They are shown exactly
        once — store them somewhere safe.
      </p>
      <ul className="grid grid-cols-2 gap-x-6 gap-y-1.5 rounded border border-border bg-raised px-5 py-4">
        {codes.map((code) => (
          <li key={code} className="font-mono text-sm text-heading">
            {code}
          </li>
        ))}
      </ul>
      <div className="mt-4 flex gap-2">
        <button
          type="button"
          onClick={copyAll}
          className="flex h-10 flex-1 items-center justify-center gap-1.5 rounded border border-border text-sm text-body hover:bg-hover hover:text-heading"
        >
          {copied ? (
            <Check className="size-4 text-ok" aria-hidden="true" />
          ) : (
            <Copy className="size-4" aria-hidden="true" />
          )}
          {copied ? "Copied" : "Copy all"}
        </button>
        <button
          type="button"
          onClick={onConfirm}
          className="flex h-10 flex-1 items-center justify-center rounded border border-border bg-accent text-sm font-medium text-white hover:opacity-90"
        >
          I saved them
        </button>
      </div>
    </AuthFrame>
  );
}
