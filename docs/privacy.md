# Privacy

**Current status:** the repository is local-only by design, but it is not yet safe for real private notes because SQLite and the MCP command store note content in plaintext.

## Data and destinations

- SQLite note bodies, titles, URLs, relationships, and reminder metadata are stored in the selected local database file without encryption.
- Attachments have metadata support in the API/store scaffold, but there is no encrypted attachment store.
- The MCP server runs as a local stdio subprocess and reads/writes its own SQLite database. It does not send content to a hosted AI provider. An MCP client may itself use an external model; user-selected note text can then leave the machine through that client. Review the client's data policy and scope grants.
- The dashboard persists only theme and companion display preferences in browser `localStorage`; note content is held in page memory. Its local API token is read server-side and not sent to page JavaScript.
- No browser extension capture package is currently built.
- No telemetry is intentionally collected by application code. Next.js anonymous developer telemetry is disabled for dashboard `npm run dev`, `npm run build`, and `npm run start` invocations. A global Next.js setting can remain enabled for commands that bypass these scripts; see [Next.js telemetry](https://nextjs.org/telemetry).

## Privacy guidance

Do not enter personal or confidential information in the current database or MCP server. Use synthetic development data. The code does not claim protection from malware, another process running as the same Windows account, administrators, memory inspection, screenshots, or accidental plaintext export. Backups, export, restore, and vault recovery are not implemented.

## Intended product boundary

Before release, implement and review encryption/key management for both the database and attachments; ship a usable lock/unlock lifecycle; document local storage and export behavior; and keep logs free of note content, credentials, and keys. Any optional external AI transfer must require explicit user action and show which content is sent.
