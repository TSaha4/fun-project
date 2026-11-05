# Local MCP server

The MCP adapter uses the official, maintained Go SDK and runs over MCP stdio. Each configured assistant launches its own subprocess; JSON-RPC uses stdin/stdout, so the server writes only protocol traffic to stdout. It binds no network port. The `braindump://` resource scheme is provisional.

The adapter only calls application services. SQLite repositories are composed in `cmd/mcp` and are not referenced by `internal/mcp`.

## Local trust and permissions

Every subprocess must have a configured `--client-id`. Access starts denied. Grant permissions explicitly using the CLI before connecting a client:

```powershell
go run ./cmd/mcp grant --client assistant-one --scopes notes:read,collections:read,metadata:read
go run ./cmd/mcp list-grants --client assistant-one
go run ./cmd/mcp revoke --client assistant-one
```

The grants file is `${APP_DATA_DIR:-./data}/mcp-grants.json`, written with restrictive file permissions where supported and atomically replaced. Keep the directory private to the current OS user. Use `--grant-file` on grant, revoke, and server commands if you choose a custom path. Every authorization reloads grants, so revocation is effective on a running server. The grant file is local configuration, not cryptographic identity: a process running as the same OS user can invoke the server with another configured ID. It does not protect against malware or compromise of that account.

| Scope | Access |
| --- | --- |
| `metadata:read` | Aggregate stats and due reminder metadata |
| `notes:read` | Note search, retrieval, and recent notes/resources |
| `attachments:read` | Reserved; no MCP attachment operation is exposed |
| `notes:create` | `create_note`, `save_link` |
| `notes:edit` | `update_note` |
| `notes:archive` | `archive_note` |
| `notes:delete` | Reserved for a future explicit permanent-delete operation |
| `collections:read` | List collections/resource |
| `collections:write` | Create collections |

Prompts are instructions only; they never grant scopes. Audit events contain client ID, operation, required scope, outcome, and timestamp. They contain no note IDs, request arguments, titles, URLs, or content.

## Connect an MCP client

Build the executable and grant read access:

```powershell
go build -o .\bin\capture-mcp.exe .\cmd\mcp
.\bin\capture-mcp.exe grant --client desktop-assistant --grant-file "C:\Users\YOUR_USER\AppData\Local\LocalCapture\mcp-grants.json" --scopes notes:read,collections:read,metadata:read
```

In an MCP client that supports local stdio servers, add an entry equivalent to:

```json
{
  "mcpServers": {
    "local-capture": {
      "command": "C:\\Users\\YOUR_USER\\path\\fun_project\\bin\\capture-mcp.exe",
      "args": ["--client-id", "desktop-assistant", "--data-dir", "C:\\Users\\YOUR_USER\\AppData\\Local\\LocalCapture", "--grant-file", "C:\\Users\\YOUR_USER\\AppData\\Local\\LocalCapture\\mcp-grants.json"]
    }
  }
}
```

The exact settings location depends on the MCP client. Use one unique client ID per configured client. The data directory should point to the same local database used by the application. With no matching grant, tools and resources return permission errors. Add write scopes only when needed. Revoke by running the `revoke` command for that ID; it replaces the grant list with no access.

See [MCP workflow evaluation](mcp-workflows.md) for the implemented read-only prompt workflows, scope requirements, external-provider notes, and privacy boundaries.

Available tools: `create_note`, `search_notes`, `get_note`, `update_note`, `archive_note`, `list_collections`, `save_link`, `get_recent_notes`, `get_due_items`, `create_collection`.

Available resources: `braindump://notes/recent`, `braindump://collections`, `braindump://stats`, `braindump://tasks/due`.

Available prompts: Organise my inbox, Find related ideas, Summarise my notes this week, Find forgotten tasks, and Turn my notes into a project outline.

## Current limitations

- Vault encryption and lock/unlock are not implemented in the current repository. SQLite data remains plaintext; MCP does not improve that status.
- The repository does not yet provide a shared application composition root. This command currently opens `capture.db` in its selected data directory.
- Statistics are basic aggregate counts. Due-item results expose reminder IDs (currently note IDs) and timestamps, but not note text.
- There is no MCP tool for attachments or permanent deletion. The corresponding scopes exist but do not activate any capability.
- The command grants/revokes scopes locally; there is no interactive consent UI. Protect the OS account and app data directory.
- Note mutation calls return a minimal ID/time receipt unless that client also has `notes:read`; edit/archive grants do not implicitly allow reading note text.
