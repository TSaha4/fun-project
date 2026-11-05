export type CompanionState = "idle" | "blink" | "working" | "saving" | "save_success" | "sleepy" | "inbox_cleared" | "reminder_notification";
export type CompanionEvent = CompanionState | "activity";
export type CompanionPreferences = { enabled: boolean; visible: boolean };

export const COMPANION_STORAGE_KEY = "local-capture-companion-preferences";
export const defaultCompanionPreferences: CompanionPreferences = { enabled: false, visible: true };
export const transientDuration: Partial<Record<CompanionState, number>> = {
  blink: 260, save_success: 1900, inbox_cleared: 2200, reminder_notification: 2600,
};

export function nextCompanionState(event: CompanionEvent): CompanionState {
  return event === "activity" ? "blink" : event;
}

export function readCompanionPreferences(storage: Pick<Storage, "getItem">): CompanionPreferences {
  try {
    const raw = storage.getItem(COMPANION_STORAGE_KEY);
    if (!raw) return defaultCompanionPreferences;
    const parsed = JSON.parse(raw) as Partial<CompanionPreferences>;
    return { enabled: parsed.enabled === true, visible: parsed.visible !== false };
  } catch { return defaultCompanionPreferences; }
}

export function writeCompanionPreferences(storage: Pick<Storage, "setItem">, value: CompanionPreferences): void {
  try { storage.setItem(COMPANION_STORAGE_KEY, JSON.stringify(value)); } catch { /* preference stays active for this session */ }
}
