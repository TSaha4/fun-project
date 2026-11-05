# SQLite storage and migration strategy

**Status:** Implemented initial persistence adapter  
**Scope:** Local SQLite storage for the approved capture domain. This decision does not select encryption-at-rest; see `docs/architecture.md` section 6.

## Driver and boundary

Persistence uses `database/sql` with the pure-Go `modernc.org/sqlite` driver. The driver is isolated in `internal/store/sqlite`. That package implements the application repository/search interfaces; application services remain independent of SQL and HTTP/UI code.

The store currently caps its connection pool at one connection per `Store` instance and enables foreign keys and a five-second busy timeout on that connection. This favors predictable connection-scoped foreign-key enforcement and modest local workloads. SQLite still serializes concurrent writers across independent connections; larger workloads may justify measured pool changes.

## Schema (migration 1)

| Table | Purpose and constraints |
|---|---|
| `schema_migrations` | Applied migration version and UTC application timestamp. Version is the primary key. |
| `notes` | Note ID, title, content, type, created/updated timestamps, nullable collection foreign key (`ON DELETE SET NULL`), pinned/archived booleans, and source URL. Title cannot be blank. |
| `collections` | ID, case-insensitive unique name, created/updated timestamps. |
| `tags` | ID, case-insensitive unique name, created timestamp. |
| `note_tags` | Many-to-many link with composite primary key; note/tag deletion cascades. |
| `attachments` | Attachment metadata (ID, file name, media type, size, checksum, created timestamp). Binary files are outside SQLite. |
| `note_attachments` | Many-to-many note-to-attachment references with composite key and cascading cleanup. |
| `reminders` | Optional one-to-one note reminder with due, snooze, completion, created/updated, and indexed effective due timestamps. |
| `reminder_events` | Lifecycle history containing IDs, event kinds, timestamps, and due instants; no note text. |
| `reminder_notification_claims` | Unique reminder/effective-due claims used to suppress duplicate notifications across restarts. |
| `notes_fts` | FTS5 virtual index over title, content, and denormalized tag text; `note_id` is unindexed metadata for joining. |

Migration 2 adds reminder lifecycle fields and tables without rewriting prior reminder records. Existing due instants populate `effective_due_at`. Indexes cover collection/type/update ordering, pinned/archive/update ordering, created timestamps, reverse tag lookup, attachment lookup, base reminder due times, and effective reminder due times. FTS triggers maintain the text index when notes, note-tag links, or tag names change. FTS storage contains searchable copies of note content and tags and must be included in any future encryption-at-rest decision. Reminder times and relationship IDs are also currently plaintext and may be sensitive.

All user-controlled values are SQL parameters. The only generated SQL fragments are placeholder counts for a parameterized `IN` list and static predicates/order clauses. Search terms are converted into quoted FTS phrases so FTS operators and punctuation do not become executable query syntax.

## Migration policy

1. Migrations are numbered, ordered, and append-only in `internal/store/sqlite/migrations.go`.
2. The migration ledger is created idempotently. Each unapplied migration's DDL and ledger insert execute in one SQL transaction. Failed DDL or ledger writes roll back together.
3. The store refuses to open a database whose recorded version exceeds the highest version supported by the binary.
4. Schema changes must preserve existing user data by default. Prefer additive tables/columns/indexes and explicit backfills. Destructive rewrites require a separate reviewed migration, backup/restore validation, and approval.
5. Never edit an already released migration. Add a new version and test upgrade from representative prior-version databases.
6. Migration tests use temporary files, verify applied versions, verify reopening is idempotent, verify existing rows remain readable, and reject unknown future versions.

SQLite DDL is transactional here, but migrations must still be tested against real prior schema snapshots before releases. Backups and encryption are not yet implemented by this increment.

## Current query behavior

`List` and `Search` filter by type, collection, pinned state, archived state, and created-time bounds. Archived records are excluded by default; a non-nil `Archived` filter selects either state. Search covers title/content/tags. `SearchQuery.TagIDs` requires a match for at least one listed tag. Results are ordered pinned-first, then most recently updated, with a maximum page size of 500.
