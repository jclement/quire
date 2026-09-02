// The current area — a per-device view preference, like the theme. "" is
// every area; "none" is the unclassified set; anything else is a frontmatter
// area value (work, personal, …). Persisted so the switcher survives reload.
export const AREA_ALL = "";
export const AREA_NONE = "none";

const STORAGE_KEY = "quire-area";

export function loadArea(): string {
  try {
    return localStorage.getItem(STORAGE_KEY) ?? AREA_ALL;
  } catch {
    return AREA_ALL;
  }
}

export function storeArea(area: string): void {
  try {
    if (area === AREA_ALL) localStorage.removeItem(STORAGE_KEY);
    else localStorage.setItem(STORAGE_KEY, area);
  } catch {
    // Storage unavailable (private mode): the choice just does not persist.
  }
}

/** A human label for an area value. */
export function areaLabel(area: string): string {
  if (area === AREA_ALL) return "All areas";
  if (area === AREA_NONE) return "Unclassified";
  if (area.includes(",")) return splitAreas(area).map(areaLabel).join(", ");
  return area.charAt(0).toUpperCase() + area.slice(1);
}

/** True when the value names a real area rather than a view of all/none. */
export function isRealArea(area: string): boolean {
  return primaryArea(area) !== "";
}

/** The palette, in the order swatches are offered. Mirrors settings.Colors. */
export const AREA_COLORS = [
  "slate",
  "red",
  "orange",
  "amber",
  "green",
  "teal",
  "blue",
  "violet",
  "pink",
] as const;

/** CSS colour for an area colour name; unknown names fall back to slate. */
export function areaColorVar(color: string | undefined): string {
  const name = (AREA_COLORS as readonly string[]).includes(color ?? "")
    ? color
    : "slate";
  return `var(--area-${name})`;
}

// ---- multiple areas ----
// The switcher can narrow to several areas at once; the value is then a
// comma-separated list ("work,personal", "none,work"), which is also what
// the API's area parameter accepts.

/** The individual areas in a filter value, normalised and de-duplicated. */
export function splitAreas(area: string): string[] {
  const out: string[] = [];
  for (const part of area.split(",")) {
    const a = part.trim().toLowerCase();
    if (a && a !== "all" && !out.includes(a)) out.push(a);
  }
  return out;
}

export function joinAreas(areas: string[]): string {
  return splitAreas(areas.join(",")).join(",");
}

/** The single real area a selection names, or "" when it names none or several. */
export function primaryArea(area: string): string {
  const areas = splitAreas(area);
  return areas.length === 1 && areas[0] !== AREA_NONE ? areas[0]! : "";
}
