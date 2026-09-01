// Theme state: a three-way System → Light → Dark toggle persisted in
// localStorage. The `.dark` class on <html> is the single switch the CSS keys
// off; index.html applies it pre-paint with the same storage key, so keep the
// two in sync.

export type ThemeSetting = "system" | "light" | "dark";

const STORAGE_KEY = "quire-theme";
const CYCLE: ThemeSetting[] = ["system", "light", "dark"];

export function getThemeSetting(): ThemeSetting {
  const stored = localStorage.getItem(STORAGE_KEY);
  return stored === "light" || stored === "dark" ? stored : "system";
}

export function nextThemeSetting(current: ThemeSetting): ThemeSetting {
  return CYCLE[(CYCLE.indexOf(current) + 1) % CYCLE.length]!;
}

/** Persists the setting and applies the `.dark` class immediately. */
export function applyThemeSetting(setting: ThemeSetting): void {
  if (setting === "system") localStorage.removeItem(STORAGE_KEY);
  else localStorage.setItem(STORAGE_KEY, setting);
  const dark =
    setting === "dark" ||
    (setting === "system" &&
      window.matchMedia("(prefers-color-scheme: dark)").matches);
  document.documentElement.classList.toggle("dark", dark);
}

/** Re-applies "system" when the OS theme changes while the app is open. */
export function watchSystemTheme(): () => void {
  const media = window.matchMedia("(prefers-color-scheme: dark)");
  const onChange = () => {
    if (getThemeSetting() === "system") applyThemeSetting("system");
  };
  media.addEventListener("change", onChange);
  return () => media.removeEventListener("change", onChange);
}
