// Application chrome: fixed header (brand, breadcrumb, theme toggle), desktop
// sidebar, mobile bottom nav + capture FAB + slide-over drawer, and the footer
// with the server version. Routed pages render into <main> via children.
import { Link, useRouterState } from "@tanstack/react-router";
import {
  CheckSquare,
  Inbox,
  Menu,
  Plus,
  Search,
  Sunrise,
  X,
  type LucideIcon,
} from "lucide-react";
import { useState, type ReactNode } from "react";
import { useHealth } from "../api/queries.ts";
import { todayISO } from "../lib/dates.ts";
import { DOC_TYPE_INFO } from "../lib/docs.ts";
import { useUi } from "../keys/UiContext.tsx";
import { ThemeToggle } from "./ThemeToggle.tsx";

interface NavEntry {
  to: string;
  label: string;
  icon: LucideIcon;
}

const PRIMARY_NAV: NavEntry[] = [
  { to: "/today", label: "Today", icon: Sunrise },
  { to: "/inbox", label: "Inbox", icon: Inbox },
  { to: "/tasks/today", label: "Tasks", icon: CheckSquare },
];

const LIBRARY_NAV: NavEntry[] = [
  { to: "/browse/note", label: "Notes", icon: DOC_TYPE_INFO.note.icon },
  { to: "/browse/person", label: "People", icon: DOC_TYPE_INFO.person.icon },
  {
    to: "/browse/company",
    label: "Companies",
    icon: DOC_TYPE_INFO.company.icon,
  },
  {
    to: "/browse/project",
    label: "Projects",
    icon: DOC_TYPE_INFO.project.icon,
  },
  {
    to: "/browse/meeting",
    label: "Meetings",
    icon: DOC_TYPE_INFO.meeting.icon,
  },
];

const MOBILE_NAV: NavEntry[] = [
  { to: "/today", label: "Today", icon: Sunrise },
  { to: "/tasks/today", label: "Tasks", icon: CheckSquare },
  { to: "/search", label: "Search", icon: Search },
  { to: "/browse/note", label: "Notes", icon: DOC_TYPE_INFO.note.icon },
];

/** Daily changes at midnight, so compute the link target per render. */
function dailyNavEntry(): NavEntry {
  return {
    to: `/daily/${todayISO()}`,
    label: "Daily",
    icon: DOC_TYPE_INFO.daily.icon,
  };
}

function NavLink({
  entry,
  onNavigate,
}: {
  entry: NavEntry;
  onNavigate?: () => void;
}) {
  return (
    <Link
      to={entry.to}
      onClick={onNavigate}
      className="flex h-8 items-center gap-2.5 rounded px-2 text-sm text-body hover:bg-hover hover:text-heading"
      activeProps={{ className: "bg-selected text-heading" }}
    >
      <entry.icon className="size-4 shrink-0 text-muted" aria-hidden="true" />
      {entry.label}
    </Link>
  );
}

function NavSections({ onNavigate }: { onNavigate?: () => void }) {
  return (
    <div className="flex h-full flex-col gap-0.5 p-2">
      {PRIMARY_NAV.map((entry) => (
        <NavLink key={entry.to} entry={entry} onNavigate={onNavigate} />
      ))}
      <div className="my-2 border-t border-border" />
      {LIBRARY_NAV.map((entry) => (
        <NavLink key={entry.to} entry={entry} onNavigate={onNavigate} />
      ))}
      <NavLink entry={dailyNavEntry()} onNavigate={onNavigate} />
      <div className="mt-auto border-t border-border pt-2">
        <NavLink
          entry={{ to: "/search", label: "Search", icon: Search }}
          onNavigate={onNavigate}
        />
      </div>
    </div>
  );
}

/** "people / sarah-chen.md" etc. from the current pathname. */
function breadcrumb(pathname: string): string {
  const parts = decodeURIComponent(pathname).split("/").filter(Boolean);
  if (parts[0] === "doc") return parts.slice(1).join(" / ");
  return parts.join(" / ");
}

function Header({ onMenu }: { onMenu: () => void }) {
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  });
  return (
    <header className="flex h-10 shrink-0 items-center gap-3 border-b border-border bg-raised px-3">
      <button
        type="button"
        onClick={onMenu}
        aria-label="Open menu"
        className="flex size-7 items-center justify-center rounded text-muted hover:bg-hover hover:text-heading md:hidden"
      >
        <Menu className="size-4" aria-hidden="true" />
      </button>
      <Link
        to="/today"
        className="font-serif text-base italic font-semibold text-heading"
      >
        quire
      </Link>
      <span className="truncate font-mono text-xs text-muted">
        {breadcrumb(pathname)}
      </span>
      <div className="ml-auto flex items-center gap-1">
        <ThemeToggle />
      </div>
    </header>
  );
}

function Footer() {
  const health = useHealth();
  const version = health.data?.version;
  return (
    <footer className="mt-12 border-t border-border pt-3 pb-4 text-xs text-muted">
      © 2026 Jeff Clement{version ? ` · v${version}` : ""}
      {health.data?.update_available ? " · update available" : ""}
    </footer>
  );
}

function MobileDrawer({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  if (!open) return null;
  return (
    <div className="fixed inset-0 z-40 md:hidden">
      <div
        className="absolute inset-0 bg-black/40"
        aria-hidden="true"
        onClick={onClose}
      />
      <div className="absolute inset-y-0 left-0 w-64 border-r border-border bg-raised pt-[env(safe-area-inset-top)]">
        <div className="flex h-10 items-center justify-between border-b border-border px-3">
          <span className="font-serif text-base font-semibold italic text-heading">
            quire
          </span>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close menu"
            className="flex size-7 items-center justify-center rounded text-muted hover:bg-hover"
          >
            <X className="size-4" aria-hidden="true" />
          </button>
        </div>
        <NavSections onNavigate={onClose} />
      </div>
    </div>
  );
}

function MobileBottomBar() {
  const { setOverlay } = useUi();
  return (
    <>
      <button
        type="button"
        onClick={() => setOverlay("capture", true)}
        aria-label="Quick capture"
        className="fixed right-4 z-30 flex size-11 items-center justify-center rounded-full bg-accent text-white shadow-lg hover:opacity-90 md:hidden"
        style={{ bottom: "calc(env(safe-area-inset-bottom) + 4.25rem)" }}
      >
        <Plus className="size-5" aria-hidden="true" />
      </button>
      <nav
        aria-label="Primary"
        className="fixed inset-x-0 bottom-0 z-30 flex border-t border-border bg-raised pb-[env(safe-area-inset-bottom)] md:hidden"
      >
        {MOBILE_NAV.map((entry) => (
          <Link
            key={entry.to}
            to={entry.to}
            className="flex h-14 min-w-11 flex-1 flex-col items-center justify-center gap-0.5 text-[10px] text-muted hover:text-heading"
            activeProps={{ className: "text-accent" }}
          >
            <entry.icon className="size-5" aria-hidden="true" />
            {entry.label}
          </Link>
        ))}
      </nav>
    </>
  );
}

export function AppShell({ children }: { children: ReactNode }) {
  const [drawerOpen, setDrawerOpen] = useState(false);
  return (
    <div className="flex h-dvh flex-col">
      <Header onMenu={() => setDrawerOpen(true)} />
      <div className="flex min-h-0 flex-1">
        <aside className="hidden w-48 shrink-0 border-r border-border bg-raised md:block">
          <NavSections />
        </aside>
        <main className="min-w-0 flex-1 overflow-y-auto">
          <div className="mx-auto flex min-h-full max-w-4xl flex-col px-4 pt-4 pb-20 md:px-6 md:pb-4">
            <div className="flex-1">{children}</div>
            <Footer />
          </div>
        </main>
      </div>
      <MobileDrawer open={drawerOpen} onClose={() => setDrawerOpen(false)} />
      <MobileBottomBar />
    </div>
  );
}
