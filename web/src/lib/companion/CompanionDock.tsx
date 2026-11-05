"use client";

import { useTheme } from "@/lib/theme/ThemeProvider";
import type { ThemeId } from "@/lib/theme/themes";
import { PandaCharacter } from "./PandaCharacter";
import { useCompanion } from "./CompanionProvider";
import type { CompanionState } from "./companion-state";
import type { ComponentType } from "react";

type CharacterProps = { state: CompanionState };
type CharacterID = "panda";
const characterRegistry: Record<CharacterID, ComponentType<CharacterProps>> = { panda: PandaCharacter };
const themeCharacters: Record<ThemeId, CharacterID> = {
  "cozy": "panda", "midnight": "panda", "sakura": "panda", "forest": "panda", "minimal": "panda",
};
const stateText: Record<CompanionState, string> = {
  idle: "Here with you.", blink: "Hi!", working: "On it…", saving: "Saving your note…", save_success: "Safely captured!",
  sleepy: "Taking a tiny rest.", inbox_cleared: "Inbox clear! Nice work.", reminder_notification: "A reminder is due.",
};

export function CompanionDock() {
  const { theme } = useTheme();
  const { enabled, visible, state, setVisible } = useCompanion();
  if (!enabled || !visible) return null;
  const Character = characterRegistry[themeCharacters[theme]];
  const announce = state === "save_success" || state === "inbox_cleared" || state === "reminder_notification";
  return <section className="companion-dock" aria-label="Optional panda companion">
    <Character state={state} />
    <div className="companion-bubble" aria-live={announce ? "polite" : "off"} aria-atomic="true">{stateText[state]}</div>
    <button className="companion-hide" type="button" onClick={() => setVisible(false)} aria-label="Hide companion">Hide</button>
  </section>;
}

export function CompanionSettings() {
  const { enabled, visible, setEnabled, setVisible } = useCompanion();
  return <fieldset className="companion-settings">
    <legend>Companion</legend>
    <label><input type="checkbox" checked={enabled} onChange={event => setEnabled(event.target.checked)} /> Enable panda companion</label>
    {enabled && <label><input type="checkbox" checked={visible} onChange={event => setVisible(event.target.checked)} /> Show companion</label>}
    <small>No sounds. You can hide or turn it off at any time.</small>
  </fieldset>;
}
