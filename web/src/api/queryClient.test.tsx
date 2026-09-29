// A failed write used to vanish: most mutations had no onError, so a
// rejected toggle or a lost save looked, to the user, like it had worked.
// The client now reports every mutation failure unless the component says
// it shows the error itself.
import { afterEach, describe, expect, test } from "bun:test";
import { QueryClientProvider, useMutation } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { ApiError } from "./client.ts";
import { createQueryClient, setMutationErrorToast } from "./queryClient.ts";

let toasts: string[] = [];
let unregister = setMutationErrorToast((message) => toasts.push(message));

afterEach(() => {
  toasts = [];
  unregister();
  unregister = setMutationErrorToast((message) => toasts.push(message));
});

function failingMutation(error: Error, meta?: { inlineError?: boolean }) {
  const client = createQueryClient();
  return renderHook(
    () =>
      useMutation({
        mutationFn: () => Promise.reject(error),
        meta,
      }),
    {
      wrapper: ({ children }) => (
        <QueryClientProvider client={client}>{children}</QueryClientProvider>
      ),
    },
  );
}

describe("mutation failures", () => {
  test("toast by default", async () => {
    const { result } = failingMutation(
      new ApiError("CONFLICT", "the file changed on disk", 409),
    );
    act(() => result.current.mutate());
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(toasts).toEqual(["the file changed on disk"]);
  });

  test("stay quiet when the component shows the error inline", async () => {
    const { result } = failingMutation(new Error("name taken"), {
      inlineError: true,
    });
    act(() => result.current.mutate());
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(toasts).toEqual([]);
  });

  test("stay quiet for an expired session, which the auth gate handles", async () => {
    const { result } = failingMutation(
      new ApiError("UNAUTHENTICATED", "sign in again", 401),
    );
    act(() => result.current.mutate());
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(toasts).toEqual([]);
  });
});
