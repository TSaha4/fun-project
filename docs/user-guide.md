# User guide (development scaffold)

There is not yet an end-user-ready application. The commands below expose only the parts that can currently be run.

## Dashboard

From `web`, run `npm ci` and `npm run dev`, then open `http://127.0.0.1:3000`. The dashboard interface can load, but note actions fail because no HTTP API executable/composition root exists. The local API proxy also requires `LOCAL_CAPTURE_API_URL`, `LOCAL_CAPTURE_API_TOKEN`, and `LOCAL_CAPTURE_DASHBOARD_ORIGIN`; setting them does not create the missing backend.

## Desktop capture

Run `go run ./cmd/desktop` on Windows. The tray/quick capture shell launches, but it remains locked and Save is disabled. Vault unlock, data saving, dashboard launch integration, and shortcut availability are not guaranteed. Exit through the tray menu.

## MCP development only

The MCP process starts with `go run ./cmd/mcp --client-id <id> --data-dir <directory> --grant-file <path>`, using a matching local grant. This creates/opens a plaintext SQLite database. Do not connect it to personal notes or an external AI client with real content. See [MCP setup](mcp-server.md).

## Not available

The Chrome extension cannot be loaded because no generated `manifest.json` exists. There is no vault creation/unlock, working integrated note capture, backup/restore, or running reminder scheduler. See [known limitations](known-limitations.md).
