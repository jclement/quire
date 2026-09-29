// The app's one QueryClient, and the default that no failed write goes
// unreported. Mutations used to need their own onError to say anything, and
// most had none: a rejected toggle rolled back without a word, a failed
// snooze simply didn't happen. Now every mutation failure toasts unless the
// component shows the error itself and says so with `meta: { inlineError:
// true }`.
//
// The toast lives in React context (UiProvider) and the cache does not, so
// UiProvider registers its toast function here when it mounts.
import { MutationCache, QueryClient } from "@tanstack/react-query";
import { ApiError, errorMessage } from "./client.ts";

declare module "@tanstack/react-query" {
  interface Register {
    mutationMeta: {
      /** The component renders this mutation's error; don't toast it too. */
      inlineError?: boolean;
    };
  }
}

let showToast: ((message: string) => void) | null = null;

/** Routes mutation failures to message; returns the unregister function. */
export function setMutationErrorToast(
  toast: (message: string) => void,
): () => void {
  showToast = toast;
  return () => {
    if (showToast === toast) showToast = null;
  };
}

export function createQueryClient(): QueryClient {
  return new QueryClient({
    mutationCache: new MutationCache({
      onError: (error, _variables, _context, mutation) => {
        if (mutation.meta?.inlineError) return;
        // An expired session fails every call at once; AuthGate already
        // answers it with the sign-in screen, and a toast per call is noise.
        if (error instanceof ApiError && error.status === 401) return;
        showToast?.(errorMessage(error));
      },
    }),
    defaultOptions: {
      queries: {
        staleTime: 15_000,
        // A dead backend fails fast and stays failed until SSE/refocus
        // retries; transient errors get one retry.
        retry: (failureCount, error) =>
          !(error instanceof ApiError && error.code === "UNREACHABLE") &&
          failureCount < 1,
      },
    },
  });
}
