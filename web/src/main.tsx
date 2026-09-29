// App entry: fonts, query client, UI (keyboard/overlay) context, router.
import "@fontsource-variable/inter";
import "@fontsource-variable/jetbrains-mono";
import "./index.css";
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { createQueryClient } from "./api/queryClient.ts";
import { AuthGate } from "./components/auth/AuthGate.tsx";
import { UiProvider } from "./keys/UiContext.tsx";
import { router } from "./routes/router.tsx";

const queryClient = createQueryClient();

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <UiProvider>
        <AuthGate>
          <RouterProvider router={router} />
        </AuthGate>
      </UiProvider>
    </QueryClientProvider>
  </StrictMode>,
);
