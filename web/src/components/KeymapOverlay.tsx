// The `?` cheat sheet: a static, grouped listing of every global key the app
// honors. If a binding changes in GlobalKeys.tsx, change it here in the same
// commit — this overlay is documentation.
import { Modal } from "./Modal.tsx";
import { useUi } from "../keys/UiContext.tsx";

interface KeyBinding {
  keys: string;
  action: string;
}

const GROUPS: { title: string; bindings: KeyBinding[] }[] = [
  {
    title: "Everywhere",
    bindings: [
      { keys: "⌘K", action: "Command palette" },
      { keys: "c", action: "Quick capture" },
      { keys: "/", action: "Search" },
      { keys: "⌘[ / ⌘]", action: "History back / forward" },
      { keys: "⌘P", action: "Print / Save as PDF" },
      { keys: "?", action: "This cheat sheet" },
      { keys: "esc", action: "One level out" },
    ],
  },
  {
    title: "Lists",
    bindings: [
      { keys: "j / k", action: "Move selection down / up" },
      { keys: "↵", action: "Open selection" },
      { keys: "x", action: "Toggle selected task" },
      { keys: "s", action: "Snooze selected task" },
    ],
  },
  {
    title: "Go to (g, then…)",
    bindings: [
      { keys: "g t", action: "Today" },
      { keys: "g i", action: "Inbox" },
      { keys: "g u", action: "Upcoming" },
      { keys: "g n", action: "Notes" },
      { keys: "g p", action: "Projects" },
      { keys: "g d", action: "Today's daily note" },
    ],
  },
  {
    title: "Calendar",
    bindings: [{ keys: "[ / ]", action: "Previous / next month" }],
  },
  {
    title: "Documents",
    bindings: [
      { keys: "e", action: "Edit" },
      { keys: "⌘E", action: "Cycle read / edit / split" },
      { keys: "⌘↵", action: "Save and return to reading" },
      { keys: "⌘S", action: "Save" },
      { keys: "⌘L", action: "Toggle checkbox on line" },
    ],
  },
];

export function KeymapOverlay() {
  const { overlays, setOverlay } = useUi();
  return (
    <Modal
      open={overlays.keymap}
      onClose={() => setOverlay("keymap", false)}
      variant="center"
      label="Keyboard shortcuts"
    >
      <div className="max-h-[70vh] overflow-y-auto p-4">
        <h2 className="mb-3 text-sm font-semibold text-heading">
          Keyboard shortcuts
        </h2>
        <div className="grid gap-4 sm:grid-cols-2">
          {GROUPS.map((group) => (
            <section key={group.title}>
              <h3 className="mb-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted">
                {group.title}
              </h3>
              <dl>
                {group.bindings.map((binding) => (
                  <div
                    key={binding.keys}
                    className="flex items-center justify-between gap-4 border-b border-border py-1 last:border-b-0"
                  >
                    <dt className="text-xs text-body">{binding.action}</dt>
                    <dd className="rounded border border-border bg-hover px-1.5 py-0.5 font-mono text-[10px] text-heading">
                      {binding.keys}
                    </dd>
                  </div>
                ))}
              </dl>
            </section>
          ))}
        </div>
      </div>
    </Modal>
  );
}
