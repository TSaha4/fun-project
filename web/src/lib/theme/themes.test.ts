import assert from "node:assert/strict";
import { test } from "node:test";
import { isThemeId, readThemePreference, saveThemePreference, THEME_STORAGE_KEY, themeLabels, themes } from "./themes.ts";

test("recognizes only supported theme IDs", () => {
  assert.equal(themes.length, 5);
  for (const id of themes) assert.ok(themeLabels[id]);
  assert.equal(isThemeId("warm-house"), true);
  assert.equal(isThemeId("not-a-theme"), false);
  assert.equal(isThemeId(null), false);
});

test("loads and saves only the selected visual preference", () => {
  const entries = new Map<string, string>([[THEME_STORAGE_KEY, "coder-terminal"]]);
  const storage = { getItem: (key: string) => entries.get(key) ?? null, setItem: (key: string, value: string) => entries.set(key, value) };
  assert.equal(readThemePreference(storage), "coder-terminal");
  saveThemePreference(storage, "focus-minimal");
  assert.equal(entries.get(THEME_STORAGE_KEY), "focus-minimal");
  assert.deepEqual([...entries.keys()], [THEME_STORAGE_KEY]);
});

test("falls back to warm house when storage is unavailable or invalid", () => {
  assert.equal(readThemePreference({ getItem: () => "invalid" }), "warm-house");
  assert.equal(readThemePreference({ getItem: () => { throw new Error("disabled"); } }), "warm-house");
});
