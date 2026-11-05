import assert from "node:assert/strict";
import { test } from "node:test";
import { isThemeId, readThemePreference, saveThemePreference, THEME_STORAGE_KEY, themeLabels, themes } from "./themes.ts";

test("recognizes only the five supported theme IDs", () => {
  assert.deepEqual(themes, ["cozy", "midnight", "sakura", "forest", "minimal"]);
  for (const id of themes) assert.ok(themeLabels[id]);
  assert.equal(isThemeId("cozy"), true);
  assert.equal(isThemeId("not-a-theme"), false);
  assert.equal(isThemeId(null), false);
});

test("loads and saves only the selected visual preference", () => {
  const entries = new Map<string, string>([[THEME_STORAGE_KEY, "midnight"]]);
  const storage = { getItem: (key: string) => entries.get(key) ?? null, setItem: (key: string, value: string) => entries.set(key, value) };
  assert.equal(readThemePreference(storage), "midnight");
  saveThemePreference(storage, "minimal");
  assert.equal(entries.get(THEME_STORAGE_KEY), "minimal");
  assert.deepEqual([...entries.keys()], [THEME_STORAGE_KEY]);
});

test("migrates a prior theme preference to a close visual match", () => {
  const entries = new Map<string, string>([[THEME_STORAGE_KEY, "cutesy-pastel"]]);
  const storage = { getItem: (key: string) => entries.get(key) ?? null, setItem: (key: string, value: string) => entries.set(key, value) };
  assert.equal(readThemePreference(storage), "sakura");
  assert.equal(entries.get(THEME_STORAGE_KEY), "sakura");
});

test("falls back to cozy when storage is unavailable or invalid", () => {
  assert.equal(readThemePreference({ getItem: () => "invalid", setItem() {} }), "cozy");
  assert.equal(readThemePreference({ getItem: () => { throw new Error("disabled"); }, setItem() {} }), "cozy");
});
