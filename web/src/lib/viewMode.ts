// Which editing mode a person prefers — Edit or Split — remembered per
// device like the theme. A new document opens straight into it; Split only
// counts on a screen wide enough to show two columns.
export type EditMode = "edit" | "split";

const STORAGE_KEY = "quire-edit-mode";

export function loadEditMode(): EditMode {
  try {
    return localStorage.getItem(STORAGE_KEY) === "split" ? "split" : "edit";
  } catch {
    return "edit";
  }
}

export function storeEditMode(mode: EditMode): void {
  try {
    localStorage.setItem(STORAGE_KEY, mode);
  } catch {
    // Storage unavailable: the preference just does not persist.
  }
}

/** The stored preference, downgraded to Edit where Split cannot fit. */
export function preferredEditMode(): EditMode {
  const mode = loadEditMode();
  if (mode === "split" && !window.matchMedia("(min-width: 768px)").matches) {
    return "edit";
  }
  return mode;
}
