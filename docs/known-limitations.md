# Known limitations

- **Plaintext data:** the SQLite database and MCP data are not encrypted. No real personal notes should be stored.
- **No vault:** `internal/vault` is only a package placeholder; no create, unlock, lock, key rotation, or recovery flow exists.
- **No runnable HTTP backend:** HTTP API handlers and tests exist, but there is no `cmd/api` or shared production composition root. The dashboard cannot perform real note operations.
- **Desktop is shell-only:** tray and quick capture UI exist, but no application service, database, or vault is connected; Save stays disabled. The global shortcut may be unavailable if another process registered it.
- **Extension is not packaged:** only TypeScript source scaffold exists; no manifest output, popup, capture behavior, Native Messaging host, or Chrome installation path.
- **MCP is a development adapter:** it directly composes SQLite in its command, stores plaintext, and grants are not strong per-client identity. It has no attachment bytes or permanent delete operation.
- **Reminders are not integrated:** reminder domain/store/scheduler logic has tests, but there is no running scheduler/notification composition.
- **No backup/restore:** a proposal exists, but no JSON/Markdown/ZIP export, encrypted archive, restore validation, or recovery workflow exists.
- **No end-to-end browser tests:** no Playwright/browser harness is installed; the extension cannot be exercised in Chrome.
- **Performance evidence is incomplete:** list/search loads note IDs then calls `Get` per result, creating N+1 queries. There are no representative large-vault benchmarks or attachment-size tests.
- **Race detector unavailable on this machine:** installed MinGW compiler is 32-bit and fails Go's 64-bit cgo race build.
- **Native desktop verification incomplete:** a prior UI run confirmed window startup but Save disabled; shortcut reported unavailable. Tray interactions and shutdown from the visible tray were not fully exercised in this audit.
