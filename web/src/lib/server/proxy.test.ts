import assert from "node:assert/strict";
import { test } from "node:test";
import { proxyLocalAPI } from "./proxy.ts";

const env = { LOCAL_CAPTURE_API_URL: "http://127.0.0.1:43127", LOCAL_CAPTURE_API_TOKEN: "x".repeat(32), LOCAL_CAPTURE_DASHBOARD_ORIGIN: "http://localhost:3000" };

function request(path = "/api/local/notes", headers: Record<string, string> = {}) {
  return new Request(`http://localhost:3000${path}`, { method: "GET", headers: { "X-Local-Dashboard": "1", Origin: "http://localhost:3000", ...headers } });
}

test("rejects invalid origins and hosts before contacting the local API", async () => {
  let contacted = false;
  const fetcher: typeof fetch = async () => { contacted = true; return Response.json([]); };
  const response = await proxyLocalAPI(request("/api/local/notes", { Origin: "http://evil.example" }), env, fetcher);
  assert.equal(response.status, 403);
  assert.equal(contacted, false);
});

test("forwards bearer token server-side to fixed loopback API and does not return it", async () => {
  let received: Request | undefined;
  const response = await proxyLocalAPI(request(), env, async (input, init) => {
    received = new Request(String(input), init);
    return Response.json([{ id: "private" }]);
  });
  assert.equal(received!.url, "http://127.0.0.1:43127/api/v1/notes");
  assert.equal(received!.headers.get("Authorization"), `Bearer ${env.LOCAL_CAPTURE_API_TOKEN}`);
  assert.equal(response.headers.get("Set-Cookie"), null);
  assert.equal((await response.text()).includes(env.LOCAL_CAPTURE_API_TOKEN), false);
});

test("rejects unlisted API routes", async () => {
  const response = await proxyLocalAPI(request("/api/local/admin"), env, async () => Response.json({}));
  assert.equal(response.status, 404);
});

test("reports missing server configuration without leaking detail", async () => {
  const response = await proxyLocalAPI(request(), {}, async () => Response.json({}));
  assert.equal(response.status, 503);
  assert.equal((await response.text()).includes("TOKEN"), false);
});
