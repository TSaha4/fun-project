import assert from "node:assert/strict";
import { test } from "node:test";
import { LocalCaptureAPI } from "./client.ts";
import { describeAPIError, LocalAPIError } from "./errors.ts";

test("client uses same-origin proxy and sends typed search body without a bearer token", async () => {
  let url = ""; let init: RequestInit | undefined;
  const api = new LocalCaptureAPI(async (input, requestInit) => {
    url = String(input); init = requestInit;
    return Response.json([]);
  });
  await api.searchNotes({ text: "private phrase" });
  assert.equal(new URL(url, "http://localhost").pathname, "/api/local/notes/search");
  const headers = new Headers(init!.headers);
  assert.equal(headers.get("X-Local-Dashboard"), "1");
  assert.equal(headers.get("Authorization"), null);
  assert.deepEqual(JSON.parse(String(init!.body)), { text: "private phrase" });
});

test("attachment upload sends raw bytes and filename header", async () => {
  let url = ""; let init: RequestInit | undefined;
  const api = new LocalCaptureAPI(async (input, requestInit) => { url = String(input); init = requestInit; return Response.json({ id: "a" }); });
  await api.uploadAttachment("n1", new File(["bytes"], "image.png", { type: "image/png" }));
  const headers = new Headers(init!.headers);
  assert.equal(new URL(url, "http://localhost").searchParams.get("note_id"), "n1");
  assert.equal(headers.get("X-Filename"), "image.png");
  assert.equal(headers.get("Content-Type"), "image/png");
  assert.equal(await (init!.body as Blob).text(), "bytes");
});

test("maps authentication, locked vault, and backend errors", async t => {
  for (const [status, code, kind] of [[401, "unauthorized", "authentication"], [423, "vault_locked", "vault_locked"], [501, "not_implemented", "backend_unavailable"]] as const) {
    await t.test(String(status), async () => {
      const api = new LocalCaptureAPI(async () => Response.json({ error: { code, message: "operation unavailable" } }, { status }));
      await assert.rejects(api.listNotes(), (error: unknown) => error instanceof LocalAPIError && error.kind === kind);
    });
  }
});

test("network failures become a generic local API error", async () => {
  const api = new LocalCaptureAPI(async () => { throw new Error("secret internal detail"); });
  await assert.rejects(api.listNotes(), (error: unknown) => error instanceof LocalAPIError && error.kind === "network" && !error.message.includes("secret"));
});

test("important dashboard error states have actionable, privacy-safe messages", () => {
  const locked = describeAPIError(new LocalAPIError({ kind: "vault_locked", code: "vault_locked", message: "Vault locked" }));
  const auth = describeAPIError(new LocalAPIError({ kind: "authentication", code: "unauthorized", message: "Unauthorized" }));
  const offline = describeAPIError(new LocalAPIError({ kind: "network", code: "network_error", message: "offline" }));
  assert.match(locked.title, /locked/i);
  assert.match(auth.message, /trusted local launcher/i);
  assert.match(offline.message, /retry/i);
  assert.equal(describeAPIError(new Error("private note text")).message.includes("private note text"), false);
});
