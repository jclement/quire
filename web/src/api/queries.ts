// TanStack Query bindings for the API: query keys, hooks, and the optimistic
// task-toggle mutation. All cache keys are defined here so the SSE hook and
// mutations invalidate the same names the views read.
import {
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query";
import { api, type ListDocumentsParams } from "./client.ts";
import type { Task, TaskView } from "./types.ts";

export const queryKeys = {
  health: ["health"] as const,
  documents: (params: ListDocumentsParams) => ["documents", params] as const,
  document: (path: string) => ["document", path] as const,
  search: (q: string) => ["search", q] as const,
  tasks: (view: TaskView) => ["tasks", view] as const,
  today: ["today"] as const,
};

/** Fetched once for the footer version — never polled (per the API contract). */
export function useHealth() {
  return useQuery({
    queryKey: queryKeys.health,
    queryFn: api.health,
    staleTime: Infinity,
    retry: false,
  });
}

export function useDocumentList(params: ListDocumentsParams, enabled = true) {
  return useQuery({
    queryKey: queryKeys.documents(params),
    queryFn: () => api.listDocuments(params),
    enabled,
  });
}

export function useDocument(path: string) {
  return useQuery({
    queryKey: queryKeys.document(path),
    queryFn: () => api.getDocument(path),
  });
}

export function useSearch(q: string) {
  return useQuery({
    queryKey: queryKeys.search(q),
    queryFn: () => api.search(q),
    enabled: q.trim().length > 0,
  });
}

export function useTasks(view: TaskView) {
  return useQuery({
    queryKey: queryKeys.tasks(view),
    queryFn: () => api.listTasks(view),
  });
}

export function useToday() {
  return useQuery({ queryKey: queryKeys.today, queryFn: api.today });
}

/** Invalidates every cache a task state change can affect. */
export function invalidateTaskCaches(queryClient: QueryClient): void {
  void queryClient.invalidateQueries({ queryKey: ["tasks"] });
  void queryClient.invalidateQueries({ queryKey: queryKeys.today });
  void queryClient.invalidateQueries({ queryKey: ["document"] });
}

/**
 * Task toggle with optimistic flip: the checkbox changes instantly, reconciles
 * with the server response, and rolls back if the write fails.
 */
export function useToggleTask() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (task: Task) => api.toggleTask(task.id),
    onMutate: async (task) => {
      await queryClient.cancelQueries({ queryKey: ["tasks"] });
      const snapshots = queryClient.getQueriesData<Task[]>({
        queryKey: ["tasks"],
      });
      for (const [key, tasks] of snapshots) {
        if (!tasks) continue;
        queryClient.setQueryData(
          key,
          tasks.map((candidate) =>
            candidate.id === task.id
              ? { ...candidate, done: !candidate.done }
              : candidate,
          ),
        );
      }
      return { snapshots };
    },
    onError: (_error, _task, context) => {
      for (const [key, tasks] of context?.snapshots ?? []) {
        queryClient.setQueryData(key, tasks);
      }
    },
    onSettled: () => invalidateTaskCaches(queryClient),
  });
}

/** Quick capture: new task into today's daily note. */
export function useCreateTask() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { text: string; due?: string }) =>
      api.createTask(input.text, input.due),
    onSuccess: () => invalidateTaskCaches(queryClient),
  });
}
