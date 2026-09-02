// Application chrome: fixed header (brand, breadcrumb, theme toggle), desktop
// sidebar, mobile bottom nav with a center capture button, and the slide-over
// drawer. Routed pages render into <main> via children. Version and update
// status live in Settings, not in page furniture.
import { AreaDot } from "./AreaDot.tsx";
import { AreaPicker } from "./AreaPicker.tsx";
import { useAreas, useAreasEnabled } from "../api/queries.ts";
import {
  AREA_ALL,
  AREA_NONE,
  areaLabel,
  joinAreas,
  splitAreas,
} from "../lib/area.ts";
import { Link, useRouterState } from "@tanstack/react-router";
import {
  CalendarRange,
  CheckSquare,
  Inbox,
  Menu,
  Plus,
  Search,
  Settings,
  BookOpen,
  Hash,
  Sunrise,
  X,
  type LucideIcon,
  ChevronDown,
} from "lucide-react";
import { useState, type ReactNode } from "react";
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

/** The month view and the journal sit with Daily: all three are the vault
 *  by date, not by type. */
const CALENDAR_NAV: NavEntry = {
  to: "/calendar",
  label: "Calendar",
  icon: CalendarRange,
};
const JOURNAL_NAV: NavEntry = {
  to: "/journal",
  label: "Journal",
  icon: BookOpen,
};
const TAGS_NAV: NavEntry = { to: "/tags", label: "Tags", icon: Hash };

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

/**
 * The area switcher: All · Work · Personal · … · Unclassified. Narrows
 * Browse, Search, Tasks and Today; the journal and calendar are by date and
 * stay whole. New documents file under the chosen area.
 */
function AreaSwitcher() {
  const { area, setArea } = useUi();
  const areas = useAreas();
  const enabled = useAreasEnabled();
  const [open, setOpen] = useState(false);
  if (!enabled) return null;
  const list = areas.data ?? [];
  const selected = splitAreas(area);
  const colorOf = (name: string) => list.find((a) => a.area === name)?.color;
  const toggle = (value: string) => {
    const next = selected.includes(value)
      ? selected.filter((a) => a !== value)
      : [...selected, value];
    setArea(joinAreas(next));
  };
  return (
    <div className="relative mb-1 px-2">
      <button
        type="button"
        aria-label="Area"
        aria-expanded={open}
        aria-haspopup="listbox"
        title="Which areas to show — pick one or several"
        onClick={() => setOpen(!open)}
        className="flex h-8 w-full min-w-0 items-center gap-1.5 rounded border border-border bg-raised px-2 text-sm text-heading hover:bg-hover"
      >
        <span className="text-xs text-muted">Area:</span>
        {selected.length === 0 ? (
          <span className="truncate">all</span>
        ) : (
          <>
            <span className="flex shrink-0 items-center gap-0.5">
              {selected.map((name) =>
                name === AREA_NONE ? (
                  <span
                    key={name}
                    className="inline-block size-2 shrink-0 rounded-full border border-border"
                    aria-hidden="true"
                  />
                ) : (
                  <AreaDot
                    key={name}
                    color={colorOf(name)}
                    title={areaLabel(name)}
                  />
                ),
              )}
            </span>
            {/* One area is named; several are told apart by their dots alone. */}
            {selected.length === 1 ? (
              <span className="truncate">{areaLabel(area)}</span>
            ) : null}
          </>
        )}
        <ChevronDown
          className="ml-auto size-3.5 shrink-0 text-muted"
          aria-hidden="true"
        />
      </button>
      {open ? (
        <AreaPicker
          label="Choose areas"
          areas={list}
          selected={selected}
          multi
          noneLabel="All areas"
          onToggle={toggle}
          onClear={() => setArea(AREA_ALL)}
          onClose={() => setOpen(false)}
        />
      ) : null}
    </div>
  );
}

function NavSections({ onNavigate }: { onNavigate?: () => void }) {
  return (
    <div className="flex h-full flex-col gap-0.5 p-2">
      <AreaSwitcher />
      {PRIMARY_NAV.map((entry) => (
        <NavLink key={entry.to} entry={entry} onNavigate={onNavigate} />
      ))}
      <div className="my-2 border-t border-border" />
      {LIBRARY_NAV.map((entry) => (
        <NavLink key={entry.to} entry={entry} onNavigate={onNavigate} />
      ))}
      <NavLink entry={dailyNavEntry()} onNavigate={onNavigate} />
      <NavLink entry={JOURNAL_NAV} onNavigate={onNavigate} />
      <NavLink entry={CALENDAR_NAV} onNavigate={onNavigate} />
      <NavLink entry={TAGS_NAV} onNavigate={onNavigate} />
      <div className="mt-auto border-t border-border pt-2">
        <NavLink
          entry={{ to: "/search", label: "Search", icon: Search }}
          onNavigate={onNavigate}
        />
        <NavLink
          entry={{
            to: "/browse/template",
            label: "Templates",
            icon: DOC_TYPE_INFO.template.icon,
          }}
          onNavigate={onNavigate}
        />
        <NavLink
          entry={{ to: "/settings", label: "Settings", icon: Settings }}
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
    <header className="flex h-10 shrink-0 items-center gap-3 border-b border-border bg-raised px-3 print:hidden">
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

function MobileDrawer({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  if (!open) return null;
  return (
    <div className="fixed inset-0 z-40 md:hidden print:hidden">
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

// Capture sits in the middle of the bar rather than floating above it: it is
// the most-used action on a phone, the center is the easiest thumb reach, and
// nothing ends up covering page content.
function MobileBottomBar() {
  const { setOverlay } = useUi();
  const half = Math.ceil(MOBILE_NAV.length / 2);
  const navItem = (entry: NavEntry) => (
    <Link
      key={entry.to}
      to={entry.to}
      className="flex h-14 min-w-11 flex-1 flex-col items-center justify-center gap-0.5 text-[10px] text-muted hover:text-heading"
      activeProps={{ className: "text-accent" }}
    >
      <entry.icon className="size-5" aria-hidden="true" />
      {entry.label}
    </Link>
  );
  return (
    <nav
      aria-label="Primary"
      className="fixed inset-x-0 bottom-0 z-30 flex items-center border-t border-border bg-raised pb-[env(safe-area-inset-bottom)] md:hidden print:hidden"
    >
      {MOBILE_NAV.slice(0, half).map(navItem)}
      <div className="flex h-14 flex-1 items-center justify-center">
        <button
          type="button"
          onClick={() => setOverlay("capture", true)}
          aria-label="Quick capture"
          className="flex size-11 items-center justify-center rounded-full bg-accent text-white shadow-sm hover:opacity-90"
        >
          <Plus className="size-5" aria-hidden="true" />
        </button>
      </div>
      {MOBILE_NAV.slice(half).map(navItem)}
    </nav>
  );
}

// Print drops every layout constraint the shell exists for: the viewport-tall
// flex column and the scrolling <main> would otherwise print as a single
// clipped page (a scroll container prints only its visible slice), and the
// content column takes the whole sheet minus the @page margin.
export function AppShell({ children }: { children: ReactNode }) {
  const [drawerOpen, setDrawerOpen] = useState(false);
  return (
    <div className="flex h-dvh flex-col print:block print:h-auto">
      <Header onMenu={() => setDrawerOpen(true)} />
      <div className="flex min-h-0 flex-1 print:block">
        <aside className="hidden w-48 shrink-0 border-r border-border bg-raised md:block print:hidden">
          <NavSections />
        </aside>
        <main className="min-w-0 flex-1 overflow-y-auto print:overflow-y-visible">
          <div className="mx-auto flex min-h-full max-w-4xl flex-col px-4 pt-4 pb-20 md:px-6 md:pb-4 print:block print:min-h-0 print:max-w-none print:p-0">
            <div className="flex-1">{children}</div>
          </div>
        </main>
      </div>
      <MobileDrawer open={drawerOpen} onClose={() => setDrawerOpen(false)} />
      <MobileBottomBar />
    </div>
  );
}
