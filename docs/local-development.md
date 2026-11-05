# Local development on Windows

This guide describes the commands that exist in this checkout. Several modules are scaffolds, so this repository cannot currently run as a fully integrated application.

## Current component status

| Component | Available now | Limitation |
| --- | --- | --- |
| Go application/domain and SQLite store | Yes | SQLite stores plaintext. Opening a store applies migrations automatically. |
| Local HTTP API | Library and handler tests only | There is no `cmd/api` executable or production composition root to bind the listener. |
| Vault creation / unlock | No | `internal/vault` is a placeholder. No vault, encrypted database, or key lifecycle exists. |
| Dashboard | Yes | Can run by itself, but its proxy has no backend to contact in this checkout. |
| Windows tray app | Yes | Launches locked; Save is disabled because no vault or service is wired. The global shortcut was unavailable on this machine during verification. |
| Chrome extension | TypeScript scaffold only | There is no packaged `manifest.json`, popup, service worker, capture implementation, or API pairing. Chrome cannot load this checkout as an extension. |
| MCP server | Yes, local stdio process | Opens/creates a plaintext SQLite database. It has no vault integration; do not use private notes. |
| Reminder scheduler | Library only | The scheduler is not wired into the desktop or an API process; Windows notifications do not start. |

No unified startup script is provided: the HTTP backend, vault, extension package, and reminder service needed for reliable orchestration do not exist yet.

## Prerequisites verified

`go.mod` requires Go 1.25.0. This computer has Go 1.27.3 (`windows/amd64`). The dashboard and extension lockfiles use npm; Node 22.14.0 and npm 11.10.0 are installed. Dashboard npm scripts run Next with `NEXT_TELEMETRY_DISABLED=1`; this is scoped to those scripts and does not change global Next.js settings. Windows is required for the native tray UI. The SQLite driver is pure Go (`modernc.org/sqlite`); no separate SQLite installation is required.

Run these in PowerShell to check your own machine:

```powershell
go version
node --version
npm --version
```

## Install dependencies and run checks

From the repository root:

```powershell
go mod download
go test ./...
go vet ./...
```

The SQLite schema is versioned in `internal/store/sqlite/migrations.go` (current schema version 2). `sqlite.Open(ctx, path)` applies missing migrations in transactions when a store is opened. There is no separate migration command. The migration integration tests use temporary databases; to run them without Go's test cache:

```powershell
go test -count=1 ./internal/store/sqlite
```

The migration tests passed in this checkout. No existing application database was opened or changed during verification.

Dashboard dependencies and checks (run from the repository root):

```powershell
Push-Location .\web
npm ci
npm test
npm run lint
npm run typecheck
npm run build
Pop-Location
```

Extension scaffold checks (also from the repository root):

```powershell
Push-Location .\extension
npm ci
npm run build
npm run lint
npm run typecheck
Pop-Location
```

`extension`'s `build` script is `tsc --noEmit`; it does not create an installable package. Do not select the source folder in Chrome's “Load unpacked” dialog: it has no `manifest.json`.

## Dashboard startup

Open a PowerShell terminal at the repository root and run:

```powershell
Set-Location .\web
npm ci
npm run dev -- --hostname 127.0.0.1
```

Open [http://127.0.0.1:3000](http://127.0.0.1:3000). Stop it with **Ctrl+C** in that terminal.

Verified here: the dashboard responded with HTTP 200. Its `/api/local/notes` proxy returned HTTP 503 (`api_not_configured`), because `LOCAL_CAPTURE_API_URL`, `LOCAL_CAPTURE_API_TOKEN`, and `LOCAL_CAPTURE_DASHBOARD_ORIGIN` are not configured. More importantly, there is no backend executable to serve the requests. Setting these variables alone will not make notes work.

The proxy reads these variables in the Next.js server process (it does not expose the token to page JavaScript):

```text
LOCAL_CAPTURE_API_URL=http://127.0.0.1:<api-port>/
LOCAL_CAPTURE_API_TOKEN=<random API bearer token>
LOCAL_CAPTURE_DASHBOARD_ORIGIN=http://127.0.0.1:3000
```

They are documented for a future API composition root; no token-generation/startup workflow is wired into a runnable backend today. Do not create a token or config file expecting it to start the missing API.

## Windows desktop tray app

In a repository-root PowerShell terminal:

```powershell
go run .\cmd\desktop
```

The app opens a “Quick Capture” window and creates a tray icon. Closing the window hides it to the tray. Use **Exit** in the tray menu to quit. The global shortcut is intended to be **Ctrl+Shift+Space**; if another app owns it, use the tray menu.

Verified here: `go build -o $env:TEMP\local-capture-desktop.exe .\cmd\desktop` succeeded and `go run .\cmd\desktop` opened the native capture window. The window reported “Shortcut unavailable (Ctrl+Shift+Space may be in use). Use the tray menu.” Save was disabled and reported the vault locked. The `go run` test process was stopped afterward. A separate `bin\desktop.exe` process with no visible window was already running before the check and remains running; it may account for the shortcut conflict, but that is unconfirmed. I left that process untouched because it may be the user's existing tray instance. The tray setup path in the test process completed without a startup error, but icon/menu actions were not exercised.

No startup-on-login registration is installed. No clipboard polling is performed.

## Local API, database, and vault

The HTTP server package can bind to loopback when a caller constructs `httpapi.NewServer`, but this repository has no API `main` or command that constructs its dependencies, token, origins, and permissions. Therefore there is no supported API startup command, listening port, or localhost address to give the dashboard. Do not run `go run ./internal/httpapi`; it is not an executable.

Vault creation and password unlock are not implemented. Current SQLite persistence is plaintext, including note content and reminder metadata. Do not put private/personal notes into the current database. There is no safe backup key or recovery procedure to document for an encrypted vault yet.

## Browser extension

The current `extension/src/manifest.ts` is only a TypeScript object. `npm run build` does not emit `manifest.json` or extension JavaScript, and there are no Chrome permissions or save-page/selection workflows implemented. It cannot be loaded unpacked and cannot connect to the local API. Chrome setup and capture tests must wait for a packaged extension and an approved communication/pairing path.

## MCP server

The MCP adapter is a stdio subprocess, not an HTTP service. Each MCP-compatible client should start its own process. First choose a local data folder. This example creates an empty development database under the current Windows user profile; it is plaintext and should not hold personal content:

```powershell
$env:APP_DATA_DIR = Join-Path $env:LOCALAPPDATA 'local-capture-dev'
New-Item -ItemType Directory -Force -Path $env:APP_DATA_DIR | Out-Null
$grantFile = Join-Path $env:APP_DATA_DIR 'mcp-grants.json'
```

Grant only the scopes needed by a particular MCP client (read-only example):

```powershell
go run .\cmd\mcp grant --client desktop-assistant --grant-file $grantFile --scopes notes:read,collections:read,metadata:read
go run .\cmd\mcp list-grants --client desktop-assistant --grant-file $grantFile
```

Configure the MCP client to launch this command from the repository root:

```powershell
go run .\cmd\mcp --client-id desktop-assistant --data-dir $env:APP_DATA_DIR --grant-file $grantFile
```

For clients requiring a direct executable, build it first:

```powershell
New-Item -ItemType Directory -Force .\bin | Out-Null
go build -o .\bin\capture-mcp.exe .\cmd\mcp
```

Use an MCP client's existing local stdio-server configuration with command `C:\path\to\fun_project\bin\capture-mcp.exe` and args `--client-id`, `desktop-assistant`, `--data-dir`, `<same data folder>`, `--grant-file`, `<same grant file>`. The JSON configuration shape varies by MCP client; the adapter itself does not read a client config file. See [MCP server details](mcp-server.md) for scopes and tools. Stop the MCP process from the MCP client; if running it manually in a terminal, use **Ctrl+C**.

Verified here: the MCP command built and started against a newly created temporary directory, opened `capture.db`, applied migrations, and exited on stdin EOF. This only verifies process/database startup; no MCP client session or note operation was tested. The MCP command opens plaintext SQLite and has no vault lock. Restrict access to the data folder and do not use personal content.

Revoke a client's access with:

```powershell
go run .\cmd\mcp revoke --client desktop-assistant --grant-file $grantFile
```

## Reminders and notifications

`internal/reminders` implements the timer-driven scheduler and has unit tests, but no current executable composes it with a vault gate and Windows notifier. Reminder records can be stored by the SQLite adapter, but there is no application startup that schedules them, and no Windows reminder notification service is running.

## Where data lives

- The MCP executable defaults to `./data/capture.db` relative to its working directory. `APP_DATA_DIR` or `--data-dir` changes that location. The grant file defaults to `mcp-grants.json` in that directory unless `--grant-file` is provided.
- SQLite creates schema tables on open; the database currently stores plaintext.
- Dashboard theme and optional companion display preferences are stored in browser `localStorage`. Note content is held in page memory, not browser storage.
- The desktop command currently has no persistence wiring and does not save notes.
- `.env.example` documents `APP_API_ADDR` and `APP_DATA_DIR`; Go does not automatically load `.env`, and `APP_API_ADDR` is not consumed by a runnable API command in this checkout.

## Troubleshooting

- **Dashboard says local API is not configured:** expected in this checkout; there is no API process and no token/origin setup path.
- **Port 3000 is occupied:** stop the existing Next process or set a different Next port, then use the matching dashboard origin when an API server exists.
- **Desktop shortcut unavailable:** this machine reported the shortcut conflict on launch. The tray icon/window fallback remains; do not work around it with a keyboard hook.
- **Desktop Save disabled:** expected; vault creation/unlock and the service wiring are absent.
- **Chrome refuses the extension folder:** expected; there is no generated `manifest.json`.
- **MCP reports permission denied:** grant the corresponding scope for the exact `--client-id`; prompts do not grant permissions.
- **MCP starts in an unexpected directory:** pass `--data-dir` and `--grant-file` explicitly. Both paths should be protected by the current Windows user account.
- **npm install reports audit advisories:** the verified `npm ci` reported 8 advisories in `web` (7 moderate, 1 high) and 6 moderate in `extension`. No automatic dependency upgrades were applied.

## Shutdown

- Dashboard: **Ctrl+C** in its PowerShell terminal.
- Desktop: choose **Exit** from the tray menu. Closing the capture window only hides it.
- MCP: disconnect/stop the stdio server from the MCP client, or press **Ctrl+C** when started manually.
- There is currently no API server or background reminder process to stop.
