# Release checklist

This checklist is intentionally a gate. Current overall decision: **not ready for release**; see [release-readiness audit](release-readiness-audit.md).

## Data safety and privacy

- [ ] Vault creation/unlock/lock and key management implemented with maintained cryptography and independent review.
- [ ] SQLite, FTS indexes, attachments, temporary files, logs, and backup data follow the approved encryption boundary.
- [ ] No real notes are sent to external services without explicit, scoped user action.
- [ ] Privacy, security, retention, export, recovery, and known-limitations docs match shipped behavior.
- [ ] MCP and dashboard use the same application services and locked state.

## Product flows

- [ ] Create/read/update/archive/restore/delete/search work end to end through the real service.
- [ ] Dashboard API, desktop, attachments, tags, collections, reminders, and authorization integrate with the backend.
- [ ] Windows tray, shortcut, capture, vault lock/unlock, notification behavior, and graceful shutdown verified on a clean Windows account.
- [ ] Extension is packaged, least-privileged, paired securely, and tested in Chrome.
- [ ] MCP client identity, scopes, revocation, audit redaction, and response limits reviewed.
- [ ] Export/backup/restore validated, including encrypted attachments, corruption, and path traversal.

## Engineering and security

- [ ] CI: gofmt, go vet, Go tests, race tests where supported, frontend tests/lint/typecheck/build, extension packaging/tests.
- [ ] Dependency advisories reviewed and resolved or explicitly accepted with evidence.
- [ ] Large-vault and large-attachment performance measured.
- [ ] Migration rollback/data preservation tested on representative existing databases.
- [ ] Threat model and professional independent security review completed before public release.
- [ ] Release package has no development database, tokens, private keys, logs, or user data.

## Part 2 readiness

- [ ] Part 1 is usable and release-safe first: encrypted common storage, application composition, versioned migration extension points, backups, and tests are complete.
- [ ] Watchlist remains a separate domain module with explicit shared-service and permission boundaries.
