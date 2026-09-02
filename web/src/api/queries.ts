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
import type { Task, TaskEdit, TaskView } from "./types.ts";
import { useUi } from "../keys/UiContext.tsx";

export const queryKeys = {
  health: ["health"] as const,
  documents: (params: ListDocumentsParams) => ["documents", params] as const,
  document: (path: string) => ["document", path] as const,
  search: (q: string) => ["search", q] as const,
  tasks: (view: TaskView, area = "") => ["tasks", view, area] as const,
  today: ["today"] as const,
  todayIn: (area: string) => ["today", area] as const,
  areas: ["areas"] as const,
  calendar: (month: string) => ["calendar", month] as const,
  shares: ["shares"] as const,
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

/** Lists within the current area unless the caller sets one explicitly. */
export function useDocumentList(params: ListDocumentsParams, enabled = true) {
  const { area } = useUi();
  const scoped = { ...params, area: params.area ?? area };
  return useQuery({
    queryKey: queryKeys.documents(scoped),
    queryFn: () => api.listDocuments(scoped),
    enabled,
  });
}

export function useAreas() {
  return useQuery({ queryKey: queryKeys.areas, queryFn: api.listAreas });
}

export function useDocument(path: string) {
  return useQuery({
    queryKey: queryKeys.document(path),
    queryFn: () => api.getDocument(path),
  });
}

export function useSearch(q: string) {
  const { area } = useUi();
  // The search grammar carries the area itself, so a typed area: wins over
  // the switcher and the URL stays the whole query.
  const scoped = area && !/\barea:/.test(q) ? `${q} area:${area}` : q;
  return useQuery({
    queryKey: queryKeys.search(scoped),
    queryFn: () => api.search(scoped),
    enabled: q.trim().length > 0,
  });
}

export function useTasks(view: TaskView) {
  const { area } = useUi();
  return useQuery({
    queryKey: queryKeys.tasks(view, area),
    queryFn: () => api.listTasks(view, area),
  });
}

export function useToday() {
  const { area } = useUi();
  return useQuery({
    queryKey: queryKeys.todayIn(area),
    queryFn: () => api.today(area),
  });
}

/** One month of the calendar; the month key ("YYYY-MM") is the cache key. */
export function useCalendar(month: string) {
  return useQuery({
    queryKey: queryKeys.calendar(month),
    queryFn: () => api.calendar(month),
  });
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

/**
 * Task field edits (snooze, priority). Not optimistic: an edit can change the
 * content-derived task id, so we adopt the server's answer via invalidation
 * rather than patching caches under a stale id.
 */
export function useEditTask() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { id: string; edit: TaskEdit }) =>
      api.editTask(input.id, input.edit),
    onSettled: () => invalidateTaskCaches(queryClient),
  });
}

export function useShares() {
  return useQuery({ queryKey: queryKeys.shares, queryFn: api.listShares });
}

/**
 * Entity linking from the properties strip (add or remove one wikilink in a
 * frontmatter key). The response is the rewritten document, so it seeds the
 * cache directly; invalidation then reconciles the lists it can appear in.
 */
export function useLinkEntity(path: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { key: string; target: string; remove?: boolean }) =>
      api.link(path, input.key, input.target, input.remove ?? false),
    onSuccess: (doc) => {
      queryClient.setQueryData(queryKeys.document(path), doc);
    },
    onSettled: () => {
      void queryClient.invalidateQueries({
        queryKey: queryKeys.document(path),
      });
      void queryClient.invalidateQueries({ queryKey: ["documents"] });
    },
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
