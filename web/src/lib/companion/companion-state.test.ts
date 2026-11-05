import assert from "node:assert/strict";
import { test } from "node:test";
import { COMPANION_STORAGE_KEY, defaultCompanionPreferences, nextCompanionState, readCompanionPreferences, writeCompanionPreferences } from "./companion-state.ts";

test("optional companion is disabled by default and maps semantic events to states", () => {
  assert.deepEqual(defaultCompanionPreferences, { enabled: false, visible: true });
  assert.equal(nextCompanionState("activity"), "blink");
  assert.equal(nextCompanionState("saving"), "saving");
  assert.equal(nextCompanionState("inbox_cleared"), "inbox_cleared");
  assert.equal(nextCompanionState("reminder_notification"), "reminder_notification");
});

test("persists only companion display preferences and handles blocked storage", () => {
  const values = new Map<string, string>();
  const storage = { getItem: (key: string) => values.get(key) ?? null, setItem: (key: string, value: string) => values.set(key, value) };
  writeCompanionPreferences(storage, { enabled: true, visible: false });
  assert.deepEqual(readCompanionPreferences(storage), { enabled: true, visible: false });
  assert.deepEqual([...values.keys()], [COMPANION_STORAGE_KEY]);
  assert.deepEqual(readCompanionPreferences({ getItem: () => { throw new Error("blocked"); } }), defaultCompanionPreferences);
});
