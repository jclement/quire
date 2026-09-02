// The popover behind both area badges: a list of areas with their dots,
// picked one at a time (a document's area) or several (the switcher).
// Plain buttons with option semantics so it reads as a list to a screen
// reader and to the tests, and closes on Escape or a click outside.
import { Check } from "lucide-react";
import { useEffect } from "react";
import type { AreaCount } from "../api/types.ts";
import { AREA_NONE, areaLabel } from "../lib/area.ts";
import { AreaDot } from "./AreaDot.tsx";

interface AreaPickerProps {
  label: string;
  areas: AreaCount[];
  /** Selected area values; empty means all (multi) or unassigned (single). */
  selected: string[];
  multi: boolean;
  /** Wording for the "no area" row. */
  noneLabel: string;
  onToggle: (area: string) => void;
  onClear: () => void;
  onClose: () => void;
}

export function AreaPicker({
  label,
  areas,
  selected,
  multi,
  noneLabel,
  onToggle,
  onClear,
  onClose,
}: AreaPickerProps) {
  // Escape closes from anywhere: focus is usually still on the badge that
  // opened the picker, not inside it, so a listener on the popup alone
  // would leave the click-away layer sitting over the page.
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      event.stopPropagation();
      onClose();
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [onClose]);
  const isSelected = (value: string) => selected.includes(value);
  const noneSelected = selected.length === 0;
  const row = (
    value: string,
    text: string,
    color: string | undefined,
    chosen: boolean,
    onPick: () => void,
  ) => (
    <li key={value || "__none"}>
      <button
        type="button"
        role="option"
        aria-selected={chosen}
        onClick={onPick}
        className={`flex h-8 w-full items-center gap-2 px-2 text-left text-xs hover:bg-hover ${
          chosen ? "text-heading" : "text-body"
        }`}
      >
        {color !== undefined ? (
          <AreaDot color={color} />
        ) : (
          <span
            className="inline-block size-2 shrink-0 rounded-full border border-border"
            aria-hidden="true"
          />
        )}
        <span className="flex-1 truncate">{text}</span>
        {chosen ? (
          <Check className="size-3.5 text-accent" aria-hidden="true" />
        ) : null}
      </button>
    </li>
  );
  return (
    <>
      <div
        className="fixed inset-0 z-10"
        aria-hidden="true"
        onClick={onClose}
      />
      <div
        role="dialog"
        aria-label={label}
        onKeyDown={(event) => {
          if (event.key === "Escape") {
            event.stopPropagation();
            onClose();
          }
        }}
        className="absolute top-full left-0 z-20 mt-1 w-52 rounded border border-border bg-raised py-1 font-sans shadow-lg"
      >
        <ul role="listbox" aria-multiselectable={multi} aria-label={label}>
          {multi
            ? row("", "All areas", undefined, noneSelected, () => {
                onClear();
                onClose();
              })
            : row("", noneLabel, undefined, noneSelected, () => {
                onClear();
                onClose();
              })}
          {areas.map((area) =>
            row(
              area.area,
              areaLabel(area.area),
              area.color,
              isSelected(area.area),
              () => {
                onToggle(area.area);
                if (!multi) onClose();
              },
            ),
          )}
          {multi
            ? row(
                AREA_NONE,
                "Unclassified",
                undefined,
                isSelected(AREA_NONE),
                () => onToggle(AREA_NONE),
              )
            : null}
        </ul>
        {multi ? (
          <p className="border-t border-border px-2 pt-1 text-[10px] text-muted">
            Pick several to see them together.
          </p>
        ) : null}
      </div>
    </>
  );
}
