export const themes = ["cozy", "midnight", "sakura", "forest", "minimal"] as const;
export type ThemeId = (typeof themes)[number];

export const themeLabels: Record<ThemeId, string> = {
  cozy: "Cozy",
  midnight: "Midnight",
  sakura: "Sakura",
  forest: "Forest",
  minimal: "Minimal",
};

export const THEME_STORAGE_KEY = "local-capture-theme";
const legacyThemes: Record<string, ThemeId> = {
  "warm-house": "cozy",
  "coder-terminal": "midnight",
  "cutesy-pastel": "sakura",
  "anime-editorial": "sakura",
  "focus-minimal": "minimal",
};

export function isThemeId(value: string | null): value is ThemeId {
  return value !== null && themes.includes(value as ThemeId);
}

export function readThemePreference(storage: Pick<Storage, "getItem" | "setItem">): ThemeId {
  try {
    const value = storage.getItem(THEME_STORAGE_KEY);
    if (isThemeId(value)) return value;
    if (value && legacyThemes[value]) {
      storage.setItem(THEME_STORAGE_KEY, legacyThemes[value]);
      return legacyThemes[value];
    }
    return "cozy";
  } catch {
    return "cozy";
  }
}

export function saveThemePreference(storage: Pick<Storage, "setItem">, value: ThemeId): void {
  try { storage.setItem(THEME_STORAGE_KEY, value); } catch { /* preference remains active for this session */ }
}
