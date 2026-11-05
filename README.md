# Local Capture Application

This repository is the implementation scaffold for a local-first personal capture application. The product name is undecided. See [the architecture proposal](docs/architecture.md) and `docs/decisions/` before adding capabilities.

## Prerequisites

- Go 1.25.0 or newer (required by `go.mod`)
- Node.js 22 and npm
- Windows is the initial desktop target; the dashboard and extension are independently buildable

## Development setup

Copy `.env.example` to `.env` for local configuration if needed. The example contains no secrets. Do not commit real `.env` files or user data.

### Go backend / Windows desktop companion

From the repository root:

```powershell
go test ./...
go vet ./...
go run ./cmd/desktop
```

On Windows, this starts a native tray companion and quick capture window. Press **Ctrl+Shift+Space** to show the window. Closing the window hides it to the tray; use the tray menu to open the dashboard, lock the vault, or exit. It does not register itself to start with Windows.

The vault is not implemented, so the current command starts locked and disables Save. It does not connect the plaintext SQLite adapter. See [desktop companion notes](docs/desktop.md) and [release readiness](docs/release-readiness-audit.md).

### MCP adapter scaffold

The MCP adapter runs as a local stdio server. Configure one instance per MCP
client, grant that client only the scopes it needs, and point `--data-dir` at
the application's data directory. See [docs/mcp-server.md](docs/mcp-server.md)
for setup, scopes, grant/revoke commands, and current limitations.

### React dashboard (Next.js)

```powershell
cd web
npm ci
npm run dev
```

Build and checks:

```powershell
npm run lint
npm run typecheck
npm run build
```

### Browser extension (TypeScript)

```powershell
cd extension
npm ci
npm run build
npm run lint
```

The extension currently contains only a typed Manifest V3 source object. Checks type-check the scaffold; no installable extension package is produced.

## Checks

Run `go test ./...` and `go vet ./...` at the root, then run lint/typecheck/build in `web` and lint/typecheck in `extension`. GitHub Actions runs these checks on pushes and pull requests.

## Privacy

The dashboard's `npm run dev`, `build`, and `start` commands disable Next.js telemetry for those invocations. This does not change a global Next.js telemetry setting for commands run outside those npm scripts. SQLite and MCP currently store note content in plaintext; do not store real personal content. See [privacy](docs/privacy.md), [security](docs/security.md), and [known limitations](docs/known-limitations.md).
