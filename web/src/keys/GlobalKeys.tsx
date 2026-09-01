// The one global keydown listener. Single-key commands fire only when focus is
// outside inputs/contenteditable/CodeMirror; Cmd/Ctrl+K works everywhere.
// Renders nothing — mount once inside the router so navigation works.
import { useNavigate } from "@tanstack/react-router";
import { useEffect } from "react";
import { todayISO } from "../lib/dates.ts";
import { useUi } from "./UiContext.tsx";

/** How long a `g` chord waits for its second key. */
const CHORD_TIMEOUT_MS = 1_500;

/** Where each `g <key>` chord goes. */
const GO_CHORDS: Record<string, string> = {
  t: "/today",
  i: "/inbox",
  u: "/tasks/upcoming",
  n: "/browse/note",
  p: "/browse/project",
};

function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  if (target.isContentEditable) return true;
  const tag = target.tagName;
  return (
    tag === "INPUT" ||
    tag === "TEXTAREA" ||
    tag === "SELECT" ||
    target.closest(".cm-editor") !== null
  );
}

export function GlobalKeys() {
  const navigate = useNavigate();
  const {
    overlays,
    setOverlay,
    newDocType,
    listNavRef,
    escapeStackRef,
    keyActionsRef,
  } = useUi();

  useEffect(() => {
    let chordPending = false;
    let chordTimer: ReturnType<typeof setTimeout> | undefined;

    const goTo = (path: string) => void navigate({ to: path });

    const handleChord = (key: string): boolean => {
      if (key in GO_CHORDS) goTo(GO_CHORDS[key]!);
      else if (key === "d") goTo(`/daily/${todayISO()}`);
      else return false;
      return true;
    };

    const handleSingleKey = (event: KeyboardEvent): boolean => {
      switch (event.key) {
        case "g":
          chordPending = true;
          clearTimeout(chordTimer);
          chordTimer = setTimeout(() => (chordPending = false), CHORD_TIMEOUT_MS);
          return true;
        case "?":
          setOverlay("keymap", true);
          return true;
        case "/":
          goTo("/search");
          // Focus after navigation renders; covers the already-on-/search case
          // (the input also autofocuses on mount).
          setTimeout(() => {
            document.getElementById("global-search-input")?.focus();
          }, 50);
          return true;
        case "c":
          setOverlay("capture", true);
          return true;
        case "j":
        case "ArrowDown":
          listNavRef.current?.move(1);
          return true;
        case "k":
        case "ArrowUp":
          listNavRef.current?.move(-1);
          return true;
        case "Enter":
          if (!listNavRef.current) return false;
          listNavRef.current.open();
          return true;
        case "x":
          if (!listNavRef.current?.toggle) return false;
          listNavRef.current.toggle();
          return true;
        case "Escape": {
          const handler = escapeStackRef.current.at(-1);
          if (!handler) return false;
          handler();
          return true;
        }
        default: {
          const action = keyActionsRef.current.get(event.key);
          if (!action) return false;
          action();
          return true;
        }
      }
    };

    const onKeyDown = (event: KeyboardEvent) => {
      const modified = event.metaKey || event.ctrlKey || event.altKey;
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setOverlay("palette", !overlays.palette);
        return;
      }
      // Overlays own their keyboard handling while open.
      if (overlays.palette || overlays.capture || overlays.keymap) return;
      if (newDocType !== null) return;
      if (modified || isEditableTarget(event.target)) return;

      if (chordPending && event.key !== "g") {
        chordPending = false;
        clearTimeout(chordTimer);
        if (handleChord(event.key)) event.preventDefault();
        return;
      }
      if (handleSingleKey(event)) event.preventDefault();
    };

    window.addEventListener("keydown", onKeyDown);
    return () => {
      clearTimeout(chordTimer);
      window.removeEventListener("keydown", onKeyDown);
    };
  }, [
    navigate,
    overlays,
    setOverlay,
    newDocType,
    listNavRef,
    escapeStackRef,
    keyActionsRef,
  ]);

  return null;
}
