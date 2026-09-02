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
  return area.charAt(0).toUpperCase() + area.slice(1);
}

/** True when the value names a real area rather than a view of all/none. */
export function isRealArea(area: string): boolean {
  return area !== AREA_ALL && area !== AREA_NONE;
}
