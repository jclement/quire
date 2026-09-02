// AuthGate decides whether anyone sees the app at all, and it has already
// shipped one bug that made a whole route unreachable: it blanked children
// on *any* pending fetch, so a second component mounting an observer on the
// same key triggered a refetch → children unmounted → query settled →
// remounted → refetched, forever. /settings simply never loaded.
//
// The rule that fixes it is narrow enough to be worth pinning: only the very
// first check may hide the app.
import { describe, expect, test } from "bun:test";
import {
  QueryClient,
  QueryClientProvider,
  useQuery,
} from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import { AUTH_STATUS_KEY, AuthGate } from "./AuthGate.tsx";
import { api } from "../../api/client.ts";
import type { AuthStatus } from "../../api/types.ts";

/** A client that does not retry, so a rejected query settles immediately. */
function testClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
}

function renderGate(children: React.ReactNode = <div>the app</div>) {
  const client = testClient();
  return {
    client,
    ...render(
      <QueryClientProvider client={client}>
        <AuthGate>{children}</AuthGate>
      </QueryClientProvider>,
    ),
  };
}

/** Replaces api.authStatus for one test and restores it afterwards. */
function stubStatus(impl: () => Promise<AuthStatus>) {
  const original = api.authStatus;
  api.authStatus = impl as typeof api.authStatus;
  return () => {
    api.authStatus = original;
  };
}

describe("AuthGate", () => {
  test("auth disabled (status 404) renders the app untouched", async () => {
    const restore = stubStatus(() => Promise.reject(new Error("NOT_FOUND")));
    try {
      renderGate();
      await waitFor(() => expect(screen.getByText("the app")).toBeDefined());
    } finally {
      restore();
    }
  });

  test("an authenticated session renders the app", async () => {
    const restore = stubStatus(async () => ({
      registered: true,
      authenticated: true,
    }));
    try {
      renderGate();
      await waitFor(() => expect(screen.getByText("the app")).toBeDefined());
    } finally {
      restore();
    }
  });

  test("registered but signed out gets the login screen, not the app", async () => {
    const restore = stubStatus(async () => ({
      registered: true,
      authenticated: false,
    }));
    try {
      renderGate();
      await waitFor(() =>
        expect(
          screen.getByRole("button", { name: /sign in with passkey/i }),
        ).toBeDefined(),
      );
      expect(screen.queryByText("the app")).toBeNull();
    } finally {
      restore();
    }
  });

  test("an unclaimed instance gets the claim screen", async () => {
    const restore = stubStatus(async () => ({
      registered: false,
      authenticated: false,
    }));
    try {
      renderGate();
      await waitFor(() =>
        expect(screen.getByText("Claim this quire")).toBeDefined(),
      );
    } finally {
      restore();
    }
  });

  // The regression, reconstructed exactly. The loop needed all three of:
  //   1. a status query that legitimately FAILS (404 = auth mode "none"),
  //   2. a second component mounting an observer on the same key, and
  //   3. that observer refetching on mount (react-query's default for a
  //      query in an error state).
  // With the gate hiding children on any pending fetch, the cycle was:
  // observer mounts → refetch → pending → children hidden → observer
  // unmounts → query settles → children shown → observer mounts → …
  // /settings never rendered. Nothing here is hypothetical: this is the
  // shape of the bug that shipped.
  test("a failing status plus a second observer does not spin forever", async () => {
    let fetches = 0;
    const restore = stubStatus(() => {
      fetches += 1;
      return Promise.reject(new Error("NOT_FOUND"));
    });
    try {
      function SettingsLike() {
        // Deliberately WITHOUT retryOnMount:false — the gate must survive a
        // child that refetches on mount, rather than depending on every
        // child remembering to opt out.
        useQuery({
          queryKey: AUTH_STATUS_KEY,
          queryFn: api.authStatus,
          retry: false,
        });
        return <div>settings</div>;
      }

      renderGate(<SettingsLike />);
      await waitFor(() => expect(screen.getByText("settings")).toBeDefined());

      // Let any loop run: if the gate is wrong, fetches climb without bound
      // while the page flickers between the splash and the content.
      await new Promise((resolve) => setTimeout(resolve, 500));

      expect(screen.getByText("settings")).toBeDefined();
      expect(fetches).toBeLessThan(10);
    } finally {
      restore();
    }
  });
});
