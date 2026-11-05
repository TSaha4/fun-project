# MCP security review

**Review date:** 2026-11-05
**Scope:** MCP tools, resources, grants, revocation, audit events, error handling, and attachment/deletion boundaries.
**Method:** Source review plus Go regression tests over the official SDK's in-memory transport. This is not a professional security audit, penetration test, or assurance that the whole application is secure.

## Findings

| ID | Severity | Finding | Status |
| --- | --- | --- | --- |
| MCP-01 | Medium, residual | `--client-id` is an identity label, not an authenticated credential. A same-OS-user process can launch another server with a granted ID. | Documented trust boundary |
| MCP-02 | High, residual | The MCP command opens the current plaintext SQLite database and has no vault lock/unlock integration. Note contents are returned to any client with `notes:read`. | Requires vault integration before relying on encrypted/locked-vault guarantees |
| MCP-03 | Medium, fixed | Note reads returned attachment IDs without requiring `attachments:read`. | Attachment references are now filtered unless that scope is granted |
| MCP-04 | Medium, fixed | An exported `SDKServer()` accessor allowed in-process code to add an unguarded alternate tool/resource handler. | Removed the accessor |
| MCP-05 | Medium, fixed | A running process denied access after revocation, but could regain access if the same client ID was granted again. | Each grant has a generation; a server process remains bound to the generation it saw at startup |
| MCP-06 | Low, fixed | The production command configured no auditor, so it emitted no access audit events. | Added fixed-field structured audit events to stderr |
| MCP-07 | Low, residual | SDK-level schema errors are rejected before the application audit hook and therefore do not produce an MCP audit event. | No content is logged; malformed-call audit coverage remains incomplete |
| MCP-08 | High, fixed in release-readiness audit | `update_note` and `archive_note` returned full note records to clients with edit/archive permission but without `notes:read`. | Mutations now return only ID and update time unless read permission is also granted |

## Evidence and behavior checked

- Each of the ten registered tools checks one scope before calling the application service. `notes:create`, `notes:edit`, `notes:archive`, and `collections:write` remain distinct. There is no delete or attachment tool.
- Mutation results are checked separately from mutation authorization. A write scope alone authorizes the operation but not reading the resulting note body; response returns a minimal receipt unless `notes:read` is also granted.
- The four resources enforce `notes:read`, `collections:read`, or `metadata:read` individually. Prompts return static instructions and cannot change grants.
- `attachments:read` alone does not authorize note reads. Without it, note results and the recent-notes resource redact `AttachmentIDs`; the adapter exposes no attachment bytes or metadata endpoint.
- Permanent deletion cannot be performed through MCP: `delete_note` is not registered and the MCP application interface does not expose deletion. `notes:delete` is recognized in the grant registry but currently activates no operation.
- Revocation is checked against the on-disk grant file for every request. New grant generations prevent an old running MCP process from regaining access after revoke-and-regrant; the client must restart to use a later grant.
- `AuditEvent` only contains client ID, action, scope, outcome, and timestamp. `SlogAuditor` logs only those fixed fields. Tool results can contain private note text by design when the caller has `notes:read`; errors returned from application services are mapped to generic messages.
- Regression tests exercise unauthorized calls, read-only restrictions, separate write scopes, resource scope checks, prompt non-authorization, attachment redaction, absence of delete/attachment operations, cross-process revocation, revoke-and-regrant, malformed arguments, response limits, and audit/error canaries.

## Remaining risks and recommendations

1. **Client isolation is limited by the OS account.** MCP stdio authenticates the peer as the process connected to the pipes; the `--client-id` string itself is not proof of client identity. A process running as the same OS user can invoke the server with another client's ID or modify its own MCP configuration. This is consistent with the current same-user trust boundary, but it is not strong per-client isolation. If client-to-client isolation is required, add a trusted consent/broker mechanism or OS-enforced process identity before granting scopes.
2. **Vault protection is absent.** `internal/vault` is not wired into MCP or the SQLite store, and the database is plaintext. MCP has no lock-state check. Treat local note content as readable by any process/user that can access the data directory and by an MCP client granted `notes:read`.
3. **Grant-file permissions depend on the parent directory's ACL on Windows.** Go's `Chmod(0600)` is not a substitute for setting and verifying a Windows DACL. Keep custom data/grant directories restricted to the intended user. The current implementation does not inspect or repair inherited ACLs.
4. **In-flight operations may complete during revocation.** Revocation prevents subsequent authorization checks, but does not cancel a tool call that passed its check before the grant changed.
5. **Audit coverage is incomplete for malformed MCP arguments.** SDK schema validation can reject input before the handler and audit event are reached. A future audit design should record safe protocol-level failure metadata without storing argument values.
6. **The stats and due resources are metadata, but metadata can still be sensitive.** They remain gated by `metadata:read`; due reminders include reminder/note IDs and timestamps.

## Fixes made during review

- Removed the exported raw SDK server accessor.
- Redacted attachment references without `attachments:read` from note tool results and the recent-notes resource.
- Added a random grant generation and bound each server process to the generation loaded at startup, preventing stale process access after revoke-and-regrant.
- Wired content-free structured audit logging into the production MCP command.
- Added regression tests for the above changes and the permission boundaries listed above.
- Added HTTP/MCP regression coverage for write-only mutation results and API rejection of reminder fields embedded in note-write payloads.

No test here proves resistance to a compromised OS account, malicious same-user process, malware, or runtime memory inspection. Obtain an independent security review before public release.
