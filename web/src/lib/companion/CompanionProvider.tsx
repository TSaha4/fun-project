"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { defaultCompanionPreferences, nextCompanionState, readCompanionPreferences, transientDuration, writeCompanionPreferences, type CompanionEvent, type CompanionPreferences, type CompanionState } from "./companion-state";

type CompanionContextValue = CompanionPreferences & {
  state: CompanionState;
  setEnabled: (enabled: boolean) => void;
  setVisible: (visible: boolean) => void;
  emit: (event: CompanionEvent) => void;
};

const CompanionContext = createContext<CompanionContextValue | null>(null);

export function CompanionProvider({ children }: { children: ReactNode }) {
  const [preferences, setPreferences] = useState(defaultCompanionPreferences);
  const [state, setState] = useState<CompanionState>("idle");
  const transientTimer = useRef<number | undefined>(undefined);

  useEffect(() => {
    try { setPreferences(readCompanionPreferences(window.localStorage)); } catch { setPreferences(defaultCompanionPreferences); }
  }, []);

  const updatePreferences = useCallback((next: CompanionPreferences) => {
    setPreferences(next);
    try { writeCompanionPreferences(window.localStorage, next); } catch { /* storage may be blocked by browser policy */ }
    if (!next.enabled) {
      window.clearTimeout(transientTimer.current);
      setState("idle");
    }
  }, []);

  const setEnabled = useCallback((enabled: boolean) => updatePreferences({ ...preferences, enabled }), [preferences, updatePreferences]);
  const setVisible = useCallback((visible: boolean) => updatePreferences({ ...preferences, visible }), [preferences, updatePreferences]);

  const emit = useCallback((event: CompanionEvent) => {
    if (!preferences.enabled) return;
    window.clearTimeout(transientTimer.current);
    const next = nextCompanionState(event);
    setState(next);
    const duration = transientDuration[next];
    if (duration) transientTimer.current = window.setTimeout(() => setState(current => current === next ? "idle" : current), duration);
  }, [preferences.enabled]);

  useEffect(() => () => window.clearTimeout(transientTimer.current), []);

  useEffect(() => {
    if (!preferences.enabled || !preferences.visible) return;
    let sleepTimer = 0;
    let blinkTimer = 0;
    const wake = () => {
      window.clearTimeout(sleepTimer);
      setState(current => current === "sleepy" ? "blink" : current === "idle" ? "blink" : current);
      window.clearTimeout(blinkTimer);
      blinkTimer = window.setTimeout(() => setState(current => current === "blink" ? "idle" : current), 260);
      sleepTimer = window.setTimeout(() => setState(current => current === "idle" ? "sleepy" : current), 120_000);
    };
    window.addEventListener("pointerdown", wake, { passive: true });
    window.addEventListener("keydown", wake);
    sleepTimer = window.setTimeout(() => setState(current => current === "idle" ? "sleepy" : current), 120_000);
    return () => {
      window.clearTimeout(sleepTimer);
      window.clearTimeout(blinkTimer);
      window.removeEventListener("pointerdown", wake);
      window.removeEventListener("keydown", wake);
    };
  }, [preferences.enabled, preferences.visible]);

  const value = useMemo(() => ({ ...preferences, state, setEnabled, setVisible, emit }), [preferences, state, setEnabled, setVisible, emit]);
  return <CompanionContext.Provider value={value}>{children}</CompanionContext.Provider>;
}

export function useCompanion() {
  const context = useContext(CompanionContext);
  if (!context) throw new Error("useCompanion must be used inside CompanionProvider");
  return context;
}
