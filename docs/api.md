# Local HTTP API v1

The Go API listens only on a literal loopback address. It requires a random bearer token, accepts only configured exact browser origins, validates the Host port against the bound listener, limits request bodies, and emits structured request metadata without request paths, query text, bodies, or backend error text. Handlers depend on the `httpapi.Application` use-case interface; they do not open SQLite.

Create a 32-byte random token with `httpapi.GenerateToken`, then persist/reload it with `httpapi.LoadOrCreateToken` in the user's private application configuration directory. On Windows that directory must inherit an ACL limited to the current user. Never print the token. Extension/dashboard delivery requires a deliberate pairing mechanism; an extension must not read arbitrary local files to discover it.

The server defaults to `127.0.0.1:0` and must be configured with exact origins, for example `http://localhost:3000` and the packaged extension's exact `chrome-extension://<id>` origin. It rejects wildcard origins, `null`, non-loopback hosts, wrong Host ports, and credentials in URLs. Requests without an Origin are supported for native clients but still require the bearer token. CORS is not authentication.

## Routes and scopes

All routes start with `/api/v1`. Request and response JSON uses snake_case. The API scopes are configured separately: `notes.read`, `notes.write`, `notes.archive`, `notes.delete`, `collections.read`, `collections.write`, `collections.delete`, `tags.read`, `tags.write`, `attachments.read`, `attachments.write`, `attachments.delete`, `reminders.read`, and `reminders.write`.

| Method | Path | Scope | Purpose |
|---|---|---|---|
| POST | `/notes` | `notes.write` | Create note |
| GET | `/notes/{id}` | `notes.read` | Read one note |
| PUT | `/notes/{id}` | `notes.write` | Replace editable note fields |
| DELETE | `/notes/{id}` | `notes.delete` | Delete note |
| POST | `/notes/{id}/archive`, `/restore` | `notes.archive` | Archive/restore |
| GET | `/notes` | `notes.read` | List with type, collection, tag, pinned, archived, dates and paging filters |
| POST | `/notes/search` | `notes.read` | Full-text search with the same filters |
| POST/GET | `/collections` | `collections.write` / `collections.read` | Create/list collections |
| PUT/DELETE | `/collections/{id}` | `collections.write` / `collections.delete` | Rename/delete collection |
| POST/GET | `/tags` | `tags.write` / `tags.read` | Create/list tags |
| POST | `/tags/assign` | `tags.write` | Assign a tag set to a note |
| DELETE | `/notes/{id}/tags/{tag_id}` | `tags.write` | Remove a tag from a note |
| POST | `/attachments?note_id={id}` | `attachments.write` | Upload raw bytes with `X-Filename` and media type |
| GET/DELETE | `/attachments/{id}` | `attachments.read` / `attachments.delete` | Download/delete attachment |
| POST/GET | `/reminders` | `reminders.write` / `reminders.read` | Create/list reminders |
| PUT | `/reminders/{id}` | `reminders.write` | Change due time |
| GET | `/reminders/due`, `/reminders/{id}/history` | `reminders.read` | View due reminders and lifecycle history |
| POST | `/reminders/{id}/snooze`, `/complete` | `reminders.write` | Snooze/complete |

Snooze accepts either an absolute `until` timestamp or a preset: `{"preset":"two_hours"}`, `{"preset":"tomorrow","timezone":"America/New_York"}`, `{"preset":"next_monday","timezone":"Europe/London"}`, or `{"preset":"custom","custom_at":"2026-12-24T09:00:00-05:00"}`. Presets use local calendar arithmetic, then persist an absolute UTC instant. The `GET /reminders/{id}/history` response includes reminder lifecycle timestamps and due instants, but never note content.

The configured body limit defaults to 1 MiB and applies to JSON and uploads. Configure a larger value only if attachment policy requires it. Upload is a raw request body, not multipart, and filename paths/control characters are rejected. Downloads set `Content-Disposition` and `X-Content-Type-Options: nosniff`.

## Example: create and search

```http
POST /api/v1/notes HTTP/1.1
Host: 127.0.0.1:43127
Authorization: Bearer <local-token>
Origin: http://localhost:3000
Content-Type: application/json

{"title":"Read later","content":"A short note","type":"idea","collection_id":"reading","tag_ids":["personal"],"pinned":false,"source_url":"https://example.test/article"}
```

```json
{
  "id": "generated-id",
  "title": "Read later",
  "content": "A short note",
  "type": "idea",
  "created_at": "2026-11-05T10:00:00Z",
  "updated_at": "2026-11-05T10:00:00Z",
  "collection_id": "reading",
  "tag_ids": ["personal"],
  "pinned": false,
  "archived": false,
  "metadata": {"source_url":"https://example.test/article"},
  "attachment_ids": []
}
```

```http
POST /api/v1/notes/search HTTP/1.1
Authorization: Bearer <local-token>
Content-Type: application/json

{"text":"short note","type":"idea","pinned":false,"archived":false,"limit":20,"offset":0}
```

## Errors

Errors use `{ "error": { "code": "...", "message": "...", "request_id": "..." } }`. Common statuses are 400 invalid input, 401 invalid/missing token, 403 denied origin/permission, 404 missing route/resource, 413 request too large, 415 unsupported media type, and 500 generic internal failure. Backend error strings are never returned to clients.

## Security notes and integration boundary

The API is not yet wired into a production desktop composition root. In particular, upload/download must connect to the approved encrypted-vault design; this HTTP package does not implement or imply encrypted attachment storage. Reminder scheduling is documented in [reminders.md](reminders.md). The vault design previously proposed is still awaiting approval, so do not expose real personal data through this API yet.
