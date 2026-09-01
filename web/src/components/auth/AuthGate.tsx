// The auth gate at app root. Checks GET /auth/status once: a 404 (auth mode
// "none") or an unreachable backend means no auth UI at all — render the app
// exactly as before. Otherwise an unauthenticated user gets the login screen
// (passkeys registered) or the first-run "claim this quire" flow (none yet).
// Any 401 from the API mid-session re-checks status via the
// "quire:unauthorized" event dispatched in client.ts.
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, type ReactNode } from "react";
import { api } from "../../api/client.ts";
import { LoginScreen, WelcomeScreen } from "./AuthScreens.tsx";

export const AUTH_STATUS_KEY = ["auth", "status"] as const;

export function AuthGate({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient();
  const status = useQuery({
    queryKey: AUTH_STATUS_KEY,
    queryFn: api.authStatus,
    staleTime: Infinity,
    retry: false,
  });

  useEffect(() => {
    const onUnauthorized = () => {
      void queryClient.invalidateQueries({ queryKey: AUTH_STATUS_KEY });
    };
    window.addEventListener("quire:unauthorized", onUnauthorized);
    return () =>
      window.removeEventListener("quire:unauthorized", onUnauthorized);
  }, [queryClient]);

  // A login/registration just completed: mark authed and refetch everything
  // (the earlier 401s left error states behind).
  const onAuthed = () => {
    queryClient.setQueryData(AUTH_STATUS_KEY, {
      registered: true,
      authenticated: true,
    });
    void queryClient.invalidateQueries();
  };

  // Only the very first check may blank the app. A later refetch — another
  // component mounting an observer on this key, or the 401 re-check — must
  // keep rendering children: hiding them unmounts whatever triggered the
  // refetch, which settles the query, which remounts it… an infinite loop
  // (this is exactly what made /settings never load).
  if (status.isPending && !status.isFetched) {
    return (
      <div className="flex h-dvh items-center justify-center">
        <span className="font-serif text-xl font-semibold italic text-muted">
          quire
        </span>
      </div>
    );
  }
  // Errors (404 = auth disabled, UNREACHABLE = server down) → plain app, as
  // does a refetch in flight (error cleared, data not back yet).
  if (status.isError || !status.data || status.data.authenticated) {
    return <>{children}</>;
  }
  return status.data.registered ? (
    <LoginScreen onAuthed={onAuthed} />
  ) : (
    <WelcomeScreen onAuthed={onAuthed} />
  );
}
