// Code-based route tree. Pages are plain prop-driven components; the thin
// wrappers here pull params/search out of the router. Router type registration
// (declare module Register) is deliberately skipped for now: nearly every link
// targets a dynamic vault path via the /doc/* splat, where literal-typed `to`
// props cost more than they catch.
import {
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  redirect,
} from "@tanstack/react-router";
import { useDocEvents } from "../api/useEvents.ts";
import { useTimezoneSync } from "../api/queries.ts";
import { currentMonthKey, isMonthKey } from "../lib/calendar.ts";
import { isDocType } from "../lib/docs.ts";
import { AppShell } from "../components/AppShell.tsx";
import { CommandPalette } from "../components/CommandPalette.tsx";
import { KeymapOverlay } from "../components/KeymapOverlay.tsx";
import { NewDocDialog } from "../components/NewDocDialog.tsx";
import { QuickCapture } from "../components/QuickCapture.tsx";
import { DeleteDocDialog } from "../components/DeleteDocDialog.tsx";
import { MarkdownHelp } from "../components/MarkdownHelp.tsx";
import { TableEditorDialog } from "../components/TableEditorDialog.tsx";
import { DrawingDialog } from "../components/DrawingDialog.tsx";
import { RenameDialog } from "../components/RenameDialog.tsx";
import { ShareDialog } from "../components/ShareDialog.tsx";
import { Toasts } from "../components/Toasts.tsx";
import { DocumentScreen } from "../components/DocumentScreen.tsx";
import { GlobalKeys } from "../keys/GlobalKeys.tsx";
import { BrowsePage } from "./BrowsePage.tsx";
import { CalendarPage } from "./CalendarPage.tsx";
import { DailyPage } from "./DailyPage.tsx";
import { JournalPage } from "./JournalPage.tsx";
import { TagsPage } from "./TagsPage.tsx";
import { UnwrittenPage } from "./UnwrittenPage.tsx";
import { NotFoundPage } from "./NotFoundPage.tsx";
import { SearchPage } from "./SearchPage.tsx";
import { SettingsPage } from "./SettingsPage.tsx";
import { TasksPage } from "./TasksPage.tsx";
import { TodayPage } from "./TodayPage.tsx";
import type { TaskView } from "../api/types.ts";

const TASK_VIEWS: TaskView[] = ["today", "upcoming", "waiting", "logbook"];

function RootLayout() {
  useDocEvents();
  useTimezoneSync();
  return (
    <>
      <GlobalKeys />
      <AppShell>
        <Outlet />
      </AppShell>
      <CommandPalette />
      <QuickCapture />
      <KeymapOverlay />
      <NewDocDialog />
      <ShareDialog />
      <RenameDialog />
      <DeleteDocDialog />
      <MarkdownHelp />
      <TableEditorDialog />
      <DrawingDialog />
      <Toasts />
    </>
  );
}

const rootRoute = createRootRoute({
  component: RootLayout,
  notFoundComponent: NotFoundPage,
});

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  beforeLoad: () => {
    throw redirect({ to: "/today" });
  },
});

const todayRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/today",
  component: TodayPage,
});

const inboxRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/inbox",
  component: () => <TasksPage view="inbox" />,
});

const tasksRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/tasks/$view",
  component: function TasksRouteComponent() {
    const { view } = tasksRoute.useParams();
    if (!TASK_VIEWS.includes(view as TaskView)) return <NotFoundPage />;
    return <TasksPage view={view as TaskView} />;
  },
});

const browseRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/browse/$type",
  component: function BrowseRouteComponent() {
    const { type } = browseRoute.useParams();
    if (!isDocType(type)) return <NotFoundPage />;
    return <BrowsePage type={type} />;
  },
});

/** ?edit=true opens documents straight into the editor. */
function validateEditSearch(search: Record<string, unknown>): {
  edit?: boolean;
} {
  return search.edit === true || search.edit === "true" ? { edit: true } : {};
}

const docRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/doc/$",
  validateSearch: validateEditSearch,
  component: function DocRouteComponent() {
    const { _splat } = docRoute.useParams();
    const { edit } = docRoute.useSearch();
    const path = _splat ?? "";
    if (!path) return <NotFoundPage />;
    // Keyed by path so editor/save state resets when navigating doc → doc.
    return (
      <DocumentScreen key={path} path={path} initialEdit={edit === true} />
    );
  },
});

const dailyRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/daily/$date",
  validateSearch: validateEditSearch,
  component: function DailyRouteComponent() {
    const { date } = dailyRoute.useParams();
    const { edit } = dailyRoute.useSearch();
    return <DailyPage key={date} date={date} edit={edit === true} />;
  },
});

/** Bare /calendar is this month; the month key is a route param, not search,
 * so a month is a link you can send someone. */
const calendarRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/calendar",
  component: function CalendarRouteComponent() {
    return <CalendarPage month={currentMonthKey()} />;
  },
});

const calendarMonthRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/calendar/$month",
  component: function CalendarMonthRouteComponent() {
    const { month } = calendarMonthRoute.useParams();
    if (!isMonthKey(month)) return <NotFoundPage />;
    return <CalendarPage month={month} />;
  },
});

const searchRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/search",
  validateSearch: (
    search: Record<string, unknown>,
  ): { q?: string; mode?: "semantic" } => ({
    ...(typeof search.q === "string" && search.q ? { q: search.q } : {}),
    ...(search.mode === "semantic" ? { mode: "semantic" as const } : {}),
  }),
  component: function SearchRouteComponent() {
    const { q, mode } = searchRoute.useSearch();
    // Keyed so arriving with a fresh ?q= reseeds the input.
    return (
      <SearchPage key={q ?? ""} initialQuery={q ?? ""} initialMode={mode} />
    );
  },
});

const settingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/settings",
  component: SettingsPage,
});

const journalRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/journal",
  component: JournalPage,
});

const tagsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/tags",
  component: TagsPage,
});

const unwrittenRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/unwritten",
  component: UnwrittenPage,
});

const routeTree = rootRoute.addChildren([
  journalRoute,
  tagsRoute,
  unwrittenRoute,
  indexRoute,
  todayRoute,
  inboxRoute,
  tasksRoute,
  browseRoute,
  docRoute,
  dailyRoute,
  calendarRoute,
  calendarMonthRoute,
  searchRoute,
  settingsRoute,
]);

export const router = createRouter({ routeTree });
