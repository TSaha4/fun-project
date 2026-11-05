# Security

## Current security posture

This is a development scaffold, not a release-ready personal vault. SQLite and MCP currently persist note data in plaintext; vault creation, encryption, lock/unlock, encrypted attachments, secure backup, and restore are absent. Do not store sensitive user data.

Existing mitigations include parameterized SQLite queries, versioned transactional migrations, loopback-only HTTP server construction, bearer-token validation, host/origin checks, bounded request bodies, API permission middleware, content-free API logs, server-side dashboard token forwarding, scoped MCP grants, revocation checks, bounded MCP result sizes, and content-free MCP audit events. These mitigations do not compensate for plaintext storage or same-user compromise.

## Trust boundaries and threats

- **Local API/browser:** malicious web origins may probe loopback services. The HTTP library enforces a loopback listener, host/origin validation, token auth, request limits, and permissions, but no production composition root starts it today.
- **MCP clients:** local stdio scopes provide operation authorization, not strong process identity. A process running as the same OS account can invoke a granted client ID or inspect/change local grant configuration.
- **OS account:** a compromised account, malware, administrator, or process with equivalent privileges can read plaintext DB data and process memory.
- **Dependencies/build:** npm audits, Go vulnerability scans, CI, and independent review are needed continuously. Audit findings are recorded in [the release audit](release-readiness-audit.md).
- **Backups/export:** no implemented recovery/export format currently exists. Plaintext files can be copied and deleted without guaranteed secure erasure.

## MCP findings

The source review in [MCP security review](mcp-security-review.md) identified and fixed mutation responses that exposed full note content without the separate notes-read scope, attachment references without attachments-read, and reminder creation embedded inside a note-write payload. Regression tests now cover those routes/tools. MCP has no permanent delete or attachment-byte operation. This source review is not a professional security audit.

## Release requirements

1. Implement and independently review vault encryption/key lifecycle before real-user data.
2. Ensure the dashboard, desktop, MCP, attachment store, reminders, and backups all honor locked state.
3. Complete composition-root and permission integration tests on Windows.
4. Review same-user MCP identity limits, grant-file Windows ACLs, export confidentiality, and dependency advisories.
5. Obtain independent security review before a public release; do not describe the product as audited or production-secure without evidence.
