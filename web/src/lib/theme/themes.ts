export const themes = ["warm-house", "coder-terminal", "cutesy-pastel", "anime-editorial", "focus-minimal"] as const;
export type ThemeId = (typeof themes)[number];

export const themeLabels: Record<ThemeId, string> = {
  "warm-house": "Warm house",
  "coder-terminal": "Coder / Terminal",
  "cutesy-pastel": "Cutesy Pastel",
  "anime-editorial": "Anime-inspired Editorial",
  "focus-minimal": "Focus / Minimal",
};

export const THEME_STORAGE_KEY = "local-capture-theme";

export function isThemeId(value: string | null): value is ThemeId {
  return value !== null && themes.includes(value as ThemeId);
}

export function readThemePreference(storage: Pick<Storage, "getItem">): ThemeId {
  try {
    const value = storage.getItem(THEME_STORAGE_KEY);
    return isThemeId(value) ? value : "warm-house";
  } catch {
    return "warm-house";
  }
}

export function saveThemePreference(storage: Pick<Storage, "setItem">, value: ThemeId): void {
  try { storage.setItem(THEME_STORAGE_KEY, value); } catch { /* preference remains active for this session */ }
}
