"use client";

import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { readThemePreference, saveThemePreference, type ThemeId } from "./themes";

type ThemeContextValue = { theme: ThemeId; setTheme: (theme: ThemeId) => void };
const ThemeContext = createContext<ThemeContextValue | null>(null);

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<ThemeId>("warm-house");

  useEffect(() => {
    let storedTheme: ThemeId = "warm-house";
    try { storedTheme = readThemePreference(window.localStorage); } catch { /* storage can be disabled by browser policy */ }
    setThemeState(storedTheme);
    document.documentElement.dataset.theme = storedTheme;
  }, []);

  const setTheme = (nextTheme: ThemeId) => {
    setThemeState(nextTheme);
    document.documentElement.dataset.theme = nextTheme;
    try { saveThemePreference(window.localStorage, nextTheme); } catch { /* keep the selected theme for this session */ }
  };

  const value = useMemo(() => ({ theme, setTheme }), [theme]);
  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme() {
  const context = useContext(ThemeContext);
  if (!context) throw new Error("useTheme must be used inside ThemeProvider");
  return context;
}
