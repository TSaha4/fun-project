# Local Capture Application

This repository is the implementation scaffold for a local-first personal capture application. The product name is undecided. See [the architecture proposal](docs/architecture.md) and `docs/decisions/` before adding capabilities.

## Prerequisites

- Go 1.23 or newer
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

The vault and encryption design are still pending approval, so the current command intentionally starts locked and disables Save. It does not connect the plaintext SQLite adapter. See [desktop companion notes](docs/desktop.md).

### MCP adapter scaffold

```powershell
go run ./cmd/mcp
```

This is only an entry-point placeholder and does not start an MCP server.

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

The extension currently contains only a typed Manifest V3 source object. A packaging step and browser behavior will be added with the first extension feature.

## Checks

Run `go test ./...` and `go vet ./...` at the root, then run lint/typecheck/build in `web` and lint/typecheck in `extension`. GitHub Actions runs these checks on pushes and pull requests.

## Privacy

No cloud service, telemetry, external AI service, database, or encryption behavior is configured by this scaffold. Do not store real personal content until the encryption and key-management decisions in the architecture proposal are resolved.
