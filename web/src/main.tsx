// App entry: fonts, query client, UI (keyboard/overlay) context, router.
import "@fontsource-variable/inter";
import "@fontsource-variable/jetbrains-mono";
import "./index.css";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { ApiError } from "./api/client.ts";
import { UiProvider } from "./keys/UiContext.tsx";
import { router } from "./routes/router.tsx";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 15_000,
      // A dead backend fails fast and stays failed until SSE/refocus retries;
      // transient errors get one retry.
      retry: (failureCount, error) =>
        !(error instanceof ApiError && error.code === "UNREACHABLE") &&
        failureCount < 1,
    },
  },
});

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <UiProvider>
        <RouterProvider router={router} />
      </UiProvider>
    </QueryClientProvider>
  </StrictMode>,
);
