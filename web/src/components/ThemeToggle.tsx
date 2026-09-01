// The three-state theme button in the header: System → Light → Dark, persisted
// via lib/theme.ts (same storage key the pre-paint script in index.html reads).
import { Monitor, Moon, Sun } from "lucide-react";
import { useEffect, useState } from "react";
import {
  applyThemeSetting,
  getThemeSetting,
  nextThemeSetting,
  watchSystemTheme,
  type ThemeSetting,
} from "../lib/theme.ts";

const ICONS: Record<ThemeSetting, typeof Sun> = {
  system: Monitor,
  light: Sun,
  dark: Moon,
};

export function ThemeToggle() {
  const [setting, setSetting] = useState<ThemeSetting>(getThemeSetting);

  useEffect(() => watchSystemTheme(), []);

  const cycle = () => {
    const next = nextThemeSetting(setting);
    setSetting(next);
    applyThemeSetting(next);
  };

  const Icon = ICONS[setting];
  return (
    <button
      type="button"
      onClick={cycle}
      title={`Theme: ${setting} (click to change)`}
      aria-label={`Theme: ${setting}. Activate to cycle.`}
      className="flex size-7 items-center justify-center rounded text-muted hover:bg-hover hover:text-heading"
    >
      <Icon className="size-4" aria-hidden="true" />
    </button>
  );
}
