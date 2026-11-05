# Part 1 release-readiness audit

**Audit date:** 2026-11-05
**Scope:** current repository only, Part 1. No Part 2 watchlist implementation. This is a source/build/limited runtime audit, not a professional security audit or certification.

## Release decision

**NO-GO for a personal-data release or real note use.** The repository is a collection of independently testable pieces, not an integrated usable personal capture application. The highest-impact blocker is that note storage is plaintext and there is no vault lifecycle. The dashboard has no runnable API backend, desktop saving is disabled, the extension cannot be loaded, and backup/restore is not implemented.

**Part 2: NOT READY.** Watchlist can remain a separate later module, but Part 1 does not yet provide safe shared storage, a production composition root, a usable end-to-end service, or recovery guarantees on which another module should depend.

## Component status

| Area | Status | Evidence and limitation |
| --- | --- | --- |
| Domain and app services | Partial | Note/collection/tag/reminder behavior and tests exist. The service is not composed into a runnable API or desktop save flow. |
| SQLite and FTS5 | Partial | Version 2 transactional migrations, parameterized SQL, FTS5, temp-DB integration tests. Stores plaintext. List/search fetch IDs then issue repeated `Get` queries (N+1). No representative large-vault benchmark. |
| Encryption and vault | Missing / release blocker | `internal/vault/doc.go` is a placeholder. No creation/unlock/lock, encryption keys, encrypted attachments, rotation, or recovery. |
| Local HTTP API | Partial | Loopback server library, auth/origin/host checks, permissions, size limits and handler tests exist. No `cmd/api`, production composition root, token setup process, or real service implementation wiring. |
| Dashboard | Partial | Next app builds and its server-side proxy keeps the token out of browser code. Dashboard returned HTTP 200 when previously run; notes proxy returned 503 because configuration/backend is absent. No end-to-end integration against a real API. |
| Desktop | Partial | Windows tray/quick-capture shell opens; Save is disabled while locked because `cmd/desktop` passes empty config. Shortcut reported unavailable in prior run; a pre-existing desktop process may have held it. Tray menu lifecycle was not fully exercised. |
| Browser extension | Missing | TypeScript source manifest object only. No generated `manifest.json`, popup/service worker, Native Messaging host, capture flow, or Chrome load test. |
| MCP | Partial | SDK stdio server/tools/resources/grants/revocation/audit exist and unit tests pass. Command directly opens plaintext SQLite; `--client-id` is a label, not strong identity. Same-account process can impersonate/change grants. No attachment-byte or permanent-delete tool. |
| Reminders | Partial | Domain, SQLite and timer scheduling logic/test coverage exist, but no running app composition, vault gate or Windows notification process. |
| Backup/export/restore | Missing | Design proposal exists only. No JSON/Markdown/ZIP implementation, encrypted backup, checksum validation, path-safe restore, or recovery flow. |
| Tests | Partial | Go unit/integration/security tests and dashboard API/theme/companion tests. No Playwright/browser suite, packaged-extension tests, real Windows end-to-end capture, or large-data performance suite. Race detector could not build with installed 32-bit MinGW. |
| Docs | Improved | Added privacy, security, user guide, known limitations, release checklist and this audit. Prior architecture document remains a proposal, not evidence that proposed features exist. |

## Security review and remediation

### Fixed in this audit

1. **High — write-only MCP/HTTP mutation response exposed note content.** `update_note` and `archive_note` with only their write/archive scopes returned full notes. HTTP mutation routes similarly returned note bodies to write-only callers. Added ID/time mutation receipts for clients without `notes.read`; added regression tests.
2. **Medium — attachment references bypassed attachment scope.** Note reads could reveal attachment IDs without `attachments.read`. HTTP and MCP responses now redact those IDs; tests added.
3. **Medium — note-write payload created embedded reminders.** Create/update DTOs accepted a reminder under note-write permission, bypassing the dedicated reminders permission. Removed reminder fields from note DTOs; API rejects them and tests verify no service call.
4. **High dependency advisory — dashboard PostCSS.** `next@15.5.29` resolved `postcss@8.4.31`, which is affected by a source-map disclosure advisory; patched upstream version is 8.5.23. Added npm override to resolve 8.5.32 without a major Next upgrade. `npm ls` confirms the patched resolution and `npm audit --omit=dev` reports zero production dependencies vulnerable. Full npm audit still reports six moderate development dependency findings; review before release. The advisory and fixed version are documented by [PostCSS](https://github.com/postcss/postcss/security/advisories/GHSA-fxqj-rqcc-2cmp).
5. **Telemetry defaults.** Next anonymous developer telemetry was enabled globally on this machine. Dashboard `dev`, `build`, and `start` scripts now set `NEXT_TELEMETRY_DISABLED=1`. This is local to those scripts and does not alter the user's global configuration or bypass commands. Next documents telemetry behavior at [Next.js telemetry](https://nextjs.org/telemetry).

### Residual security risks

- All SQLite note content is plaintext. Local-only is not equivalent to encrypted or safe against another local process.
- MCP's same-user process identity weakness and Windows grant-file ACL assumptions remain.
- Revocation cannot cancel an operation already past its authorization check; SDK schema failures may not create audit events.
- No secure export/restore or encrypted attachment behavior exists.
- No full end-to-end test proves locks/scopes across each client.
- Independent security review is required before public release.

## Privacy and external transfer

Application source does not show an external AI call or telemetry collector. The dashboard stores theme/companion preferences only in `localStorage`; note content is held in page memory. Its token stays server-side in the proxy. Next's build/runtime telemetry was disabled for project npm scripts; the machine-level Next setting remains as-is, and a direct global Next invocation may follow that setting. MCP itself is local stdio but a connected assistant may send data to its own model provider if granted note content. SQLite/MCP note data remains plaintext on disk.

## Performance and reliability

- SQLite FTS5 and indexes provide a sound initial search mechanism, but `queryNotes` performs one ID query followed by `Get` per note. Each `Get` loads tags, attachments, and reminder metadata, making a 500-note page thousands of SQL statements. Optimize via joined/batched hydration before claiming large-vault responsiveness.
- No measured latency, memory, or DB-size evidence for hundreds/thousands of notes or large attachments.
- Migrations are applied automatically in per-version transactions and reject newer schema versions. No downgrade path; backup before future schema changes remains important.
- API and MCP commands are not composed around one shared running application service/store.

## Verification performed

| Check | Result |
| --- | --- |
| `go build ./...` | Passed |
| `go vet ./...` | Passed |
| `go test -count=1 ./...` | Passed; all Go packages with tests passed |
| `go test -count=1 ./internal/store/sqlite ./internal/mcp ./internal/httpapi` | Passed |
| gofmt on current Go source | Passed; `git ls-files` check cannot be used as-is because it includes a deleted tracked `internal/mcp/doc.go` in this working tree |
| `govulncheck ./...` via `go run ...@latest` | No reachable vulnerabilities reported; one imported dependency advisory not shown as called by this code |
| golangci-lint v2 | Failed: 48 findings, primarily unchecked errors (43), plus 2 staticcheck and 3 unused findings. Linter was run ad hoc; no repo lint configuration required this specific linter. |
| `go test -race ./...` | Not run successfully: installed MinGW `gcc.exe` is 32-bit and reports `64-bit mode not compiled in` for Go's cgo race runtime. |
| Dashboard `npm test` | Passed: 18 tests |
| Dashboard lint/typecheck/build | Passed after telemetry wrapper lint/module corrections |
| Extension build/lint/typecheck | Passed; these are TypeScript checks only and do not package an extension |
| `npm ls postcss --all` (web) | Next 15.5.29 resolves PostCSS 8.5.32 |
| `npm audit --omit=dev` (web) | 0 vulnerabilities |
| Full npm audit | Web: six moderate advisories remain, all in development lint tooling (`@typescript-eslint`/`minimatch`). Extension: six moderate advisories in development lint tooling (`@typescript-eslint`/`minimatch`). No automatic broad upgrades applied. |
| Windows UI runtime | Earlier audit run launched quick-capture window; Save was disabled/locked and shortcut unavailable. No real note capture can be verified without vault/service wiring. Existing tray process was left untouched. |
| Browser/API end-to-end | Not available: no API executable and no extension package. |

## Release gates

1. Implement and independently review the approved encrypted vault before entering real data.
2. Add an API/desktop production composition root that binds store, services, permissions, vault state, token management, and graceful shutdown.
3. Make dashboard and MCP use the same service/data directory; wire reminders and notifications with locked-state behavior.
4. Complete extension packaging and secure, user-approved Native Messaging installation/host integration.
5. Implement portable export/backup/restore with integrity, encrypted backup compatibility, path traversal defenses, and recoverability documentation.
6. Optimize note hydration and record large-vault performance results.
7. Review remaining npm advisories, resolve unchecked Go errors or document an intentional linter baseline, and run Windows race-enabled CI with a supported 64-bit compiler.
8. Complete clean-machine Windows end-to-end tests and an independent security review.

## Recommended Part 2 order

Do not begin watchlist implementation yet. First finish the Part 1 release gates: encrypted shared storage and recovery, application composition, backup/restore, and end-to-end tests. Then add watchlist as a separate domain/use-case module with incremental migrations and its own API/MCP scopes, reusing the reviewed vault, permission, and backup boundaries.
