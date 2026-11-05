const API_PREFIX = "/api/local";
const MAX_REQUEST_BYTES = 1 << 20;

type ProxyConfig = { upstream: URL; token: string; dashboardOrigin: URL };
type ProxyEnvironment = { LOCAL_CAPTURE_API_URL?: string; LOCAL_CAPTURE_API_TOKEN?: string; LOCAL_CAPTURE_DASHBOARD_ORIGIN?: string };

function jsonError(status: number, code: string, message: string): Response {
  return Response.json({ error: { code, message } }, { status, headers: { "Cache-Control": "no-store" } });
}

function configFromEnvironment(env: ProxyEnvironment): ProxyConfig | Response {
  const rawURL = env.LOCAL_CAPTURE_API_URL;
  const token = env.LOCAL_CAPTURE_API_TOKEN;
  const rawOrigin = env.LOCAL_CAPTURE_DASHBOARD_ORIGIN;
  if (!rawURL || !token || !rawOrigin) return jsonError(503, "api_not_configured", "The local API connection is not configured.");
  if (token.length < 32) return jsonError(503, "api_not_configured", "The local API connection is not configured.");
  try {
    const upstream = new URL(rawURL);
    const dashboardOrigin = new URL(rawOrigin);
    const loopback = ["127.0.0.1", "localhost", "[::1]"].includes(upstream.hostname);
    if (upstream.protocol !== "http:" || !loopback || upstream.username || upstream.password || upstream.search || upstream.hash || upstream.pathname !== "/") {
      return jsonError(503, "api_not_configured", "The local API connection is not configured.");
    }
    if (!["http:", "https:"].includes(dashboardOrigin.protocol) || dashboardOrigin.origin !== rawOrigin.replace(/\/$/, "")) {
      return jsonError(503, "api_not_configured", "The local API connection is not configured.");
    }
    return { upstream, token, dashboardOrigin };
  } catch {
    return jsonError(503, "api_not_configured", "The local API connection is not configured.");
  }
}

function routeAllowed(path: string, method: string): boolean {
  const s = "[A-Za-z0-9_-]+";
  const rules: Array<[RegExp, string[]]> = [
    [/^\/notes$/, ["GET", "POST"]], [/^\/notes\/search$/, ["POST"]],
    [new RegExp(`^/notes/${s}$`), ["GET", "PUT", "DELETE"]],
    [new RegExp(`^/notes/${s}/(?:archive|restore)$`), ["POST"]],
    [new RegExp(`^/notes/${s}/tags/${s}$`), ["DELETE"]],
    [/^\/collections$/, ["GET", "POST"]], [new RegExp(`^/collections/${s}$`), ["PUT", "DELETE"]],
    [/^\/tags$/, ["GET", "POST"]], [/^\/tags\/assign$/, ["POST"]],
    [/^\/attachments$/, ["POST"]], [new RegExp(`^/attachments/${s}$`), ["GET", "DELETE"]],
    [/^\/reminders$/, ["GET", "POST"]], [/^\/reminders\/due$/, ["GET"]],
    [new RegExp(`^/reminders/${s}$`), ["PUT"]],
    [new RegExp(`^/reminders/${s}/(?:snooze|complete)$`), ["POST"]],
    [new RegExp(`^/reminders/${s}/history$`), ["GET"]],
  ];
  return rules.some(([pattern, methods]) => pattern.test(path) && methods.includes(method));
}

export async function proxyLocalAPI(request: Request, env: ProxyEnvironment = process.env as ProxyEnvironment, fetcher: typeof fetch = fetch): Promise<Response> {
  const config = configFromEnvironment(env);
  if (config instanceof Response) return config;
  const incoming = new URL(request.url);
  const expectedHost = config.dashboardOrigin.host;
  if (incoming.host !== expectedHost || request.headers.get("X-Local-Dashboard") !== "1") return jsonError(403, "origin_rejected", "Dashboard request was rejected.");
  const origin = request.headers.get("Origin");
  if (origin && origin !== config.dashboardOrigin.origin) return jsonError(403, "origin_rejected", "Dashboard request was rejected.");
  const fetchSite = request.headers.get("Sec-Fetch-Site");
  if (fetchSite && fetchSite !== "same-origin" && fetchSite !== "none") return jsonError(403, "origin_rejected", "Dashboard request was rejected.");
  const path = incoming.pathname.slice(API_PREFIX.length);
  if (!incoming.pathname.startsWith(`${API_PREFIX}/`) || !routeAllowed(path, request.method)) return jsonError(404, "route_not_found", "Local API route was not found.");
  if (request.method !== "GET" && request.method !== "HEAD") {
    const length = Number(request.headers.get("Content-Length") || "0");
    if (length > MAX_REQUEST_BYTES) return jsonError(413, "request_too_large", "Request exceeds the local API size limit.");
  }
  let body: ArrayBuffer | undefined;
  if (request.method !== "GET" && request.method !== "HEAD") {
    body = await request.arrayBuffer();
    if (body.byteLength > MAX_REQUEST_BYTES) return jsonError(413, "request_too_large", "Request exceeds the local API size limit.");
  }
  const headers = new Headers();
  headers.set("Authorization", `Bearer ${config.token}`);
  for (const name of ["Content-Type", "X-Filename"]) {
    const value = request.headers.get(name);
    if (value) headers.set(name, value);
  }
  let upstream: Response;
  try {
    upstream = await fetcher(new URL(`/api/v1${path}${incoming.search}`, config.upstream), {
      method: request.method, headers, body, cache: "no-store", redirect: "error",
    });
  } catch {
    return jsonError(502, "local_api_unavailable", "The local API could not be reached.");
  }
  const responseHeaders = new Headers({ "Cache-Control": "no-store", "X-Content-Type-Options": "nosniff" });
  for (const name of ["Content-Type", "Content-Disposition", "Content-Length", "X-Request-ID"]) {
    const value = upstream.headers.get(name);
    if (value) responseHeaders.set(name, value);
  }
  return new Response(upstream.body, { status: upstream.status, headers: responseHeaders });
}
