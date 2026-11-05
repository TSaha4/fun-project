package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

type migration struct {
	version int
	sql     string
}

var migrations = []migration{{version: 1, sql: `
CREATE TABLE collections (
 id TEXT PRIMARY KEY NOT NULL,
 name TEXT NOT NULL COLLATE NOCASE UNIQUE,
 created_at TEXT NOT NULL,
 updated_at TEXT NOT NULL
);
CREATE TABLE tags (
 id TEXT PRIMARY KEY NOT NULL,
 name TEXT NOT NULL COLLATE NOCASE UNIQUE,
 created_at TEXT NOT NULL
);
CREATE TABLE notes (
 id TEXT PRIMARY KEY NOT NULL,
 title TEXT NOT NULL CHECK(length(trim(title)) > 0),
 content TEXT NOT NULL DEFAULT '',
 type TEXT NOT NULL,
 created_at TEXT NOT NULL,
 updated_at TEXT NOT NULL,
 collection_id TEXT REFERENCES collections(id) ON DELETE SET NULL,
 pinned INTEGER NOT NULL DEFAULT 0 CHECK(pinned IN (0,1)),
 archived INTEGER NOT NULL DEFAULT 0 CHECK(archived IN (0,1)),
 source_url TEXT
);
CREATE TABLE note_tags (
 note_id TEXT NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
 tag_id TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
 PRIMARY KEY(note_id, tag_id)
);
CREATE TABLE attachments (
 id TEXT PRIMARY KEY NOT NULL,
 file_name TEXT NOT NULL,
 media_type TEXT NOT NULL,
 size INTEGER NOT NULL CHECK(size >= 0),
 checksum TEXT NOT NULL,
 created_at TEXT NOT NULL
);
CREATE TABLE note_attachments (
 note_id TEXT NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
 attachment_id TEXT NOT NULL REFERENCES attachments(id) ON DELETE CASCADE,
 PRIMARY KEY(note_id, attachment_id)
);
CREATE TABLE reminders (
 note_id TEXT PRIMARY KEY NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
 due_at TEXT NOT NULL
);
CREATE INDEX idx_notes_collection_updated ON notes(collection_id, updated_at DESC);
CREATE INDEX idx_notes_type_updated ON notes(type, updated_at DESC);
CREATE INDEX idx_notes_pinned_archived_updated ON notes(pinned, archived, updated_at DESC);
CREATE INDEX idx_notes_created ON notes(created_at);
CREATE INDEX idx_note_tags_tag_note ON note_tags(tag_id, note_id);
CREATE INDEX idx_note_attachments_attachment ON note_attachments(attachment_id);
CREATE INDEX idx_reminders_due ON reminders(due_at);

CREATE VIRTUAL TABLE notes_fts USING fts5(note_id UNINDEXED, title, content, tags, tokenize='unicode61');
CREATE TRIGGER notes_fts_insert AFTER INSERT ON notes BEGIN
 INSERT INTO notes_fts(note_id,title,content,tags) VALUES(new.id,new.title,new.content,COALESCE((SELECT group_concat(t.name,' ') FROM tags t JOIN note_tags nt ON nt.tag_id=t.id WHERE nt.note_id=new.id),''));
END;
CREATE TRIGGER notes_fts_delete AFTER DELETE ON notes BEGIN
 DELETE FROM notes_fts WHERE note_id=old.id;
END;
CREATE TRIGGER notes_fts_update AFTER UPDATE OF title,content ON notes BEGIN
 DELETE FROM notes_fts WHERE note_id=old.id;
 INSERT INTO notes_fts(note_id,title,content,tags) VALUES(new.id,new.title,new.content,COALESCE((SELECT group_concat(t.name,' ') FROM tags t JOIN note_tags nt ON nt.tag_id=t.id WHERE nt.note_id=new.id),''));
END;
CREATE TRIGGER note_tags_fts_insert AFTER INSERT ON note_tags BEGIN
 DELETE FROM notes_fts WHERE note_id=new.note_id;
 INSERT INTO notes_fts(note_id,title,content,tags)
 SELECT n.id,n.title,n.content,COALESCE((SELECT group_concat(t.name,' ') FROM tags t JOIN note_tags nt ON nt.tag_id=t.id WHERE nt.note_id=n.id),'') FROM notes n WHERE n.id=new.note_id;
END;
CREATE TRIGGER note_tags_fts_delete AFTER DELETE ON note_tags BEGIN
 DELETE FROM notes_fts WHERE note_id=old.note_id;
 INSERT INTO notes_fts(note_id,title,content,tags)
 SELECT n.id,n.title,n.content,COALESCE((SELECT group_concat(t.name,' ') FROM tags t JOIN note_tags nt ON nt.tag_id=t.id WHERE nt.note_id=n.id),'') FROM notes n WHERE n.id=old.note_id;
END;
CREATE TRIGGER tags_fts_update AFTER UPDATE OF name ON tags BEGIN
 DELETE FROM notes_fts WHERE note_id IN (SELECT note_id FROM note_tags WHERE tag_id=new.id);
 INSERT INTO notes_fts(note_id,title,content,tags)
 SELECT n.id,n.title,n.content,COALESCE((SELECT group_concat(t.name,' ') FROM tags t JOIN note_tags nt ON nt.tag_id=t.id WHERE nt.note_id=n.id),'')
 FROM notes n JOIN note_tags nt ON nt.note_id=n.id WHERE nt.tag_id=new.id;
END;
`}, {version: 2, sql: `
ALTER TABLE reminders ADD COLUMN snoozed_until TEXT;
ALTER TABLE reminders ADD COLUMN completed_at TEXT;
ALTER TABLE reminders ADD COLUMN created_at TEXT NOT NULL DEFAULT '';
ALTER TABLE reminders ADD COLUMN updated_at TEXT NOT NULL DEFAULT '';
ALTER TABLE reminders ADD COLUMN effective_due_at TEXT NOT NULL DEFAULT '';
UPDATE reminders SET created_at=due_at, updated_at=due_at, effective_due_at=due_at;
CREATE INDEX idx_reminders_effective_due ON reminders(effective_due_at,completed_at);
CREATE TABLE reminder_events (
 id TEXT PRIMARY KEY NOT NULL,
 reminder_id TEXT NOT NULL REFERENCES reminders(note_id) ON DELETE CASCADE,
 kind TEXT NOT NULL CHECK(kind IN ('created','updated','snoozed','completed','notification_claimed')),
 occurred_at TEXT NOT NULL,
 due_at TEXT NOT NULL
);
CREATE INDEX idx_reminder_events_history ON reminder_events(reminder_id,occurred_at DESC);
CREATE TABLE reminder_notification_claims (
 reminder_id TEXT NOT NULL REFERENCES reminders(note_id) ON DELETE CASCADE,
 scheduled_for TEXT NOT NULL,
 claimed_at TEXT NOT NULL,
 PRIMARY KEY(reminder_id,scheduled_for)
);
CREATE INDEX idx_reminder_notification_claims_claimed ON reminder_notification_claims(claimed_at);
`}}

func migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY NOT NULL, applied_at TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}
	var current int
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&current); err != nil {
		return err
	}
	if current > currentSchemaVersion {
		return fmt.Errorf("database schema version %d is newer than supported version %d", current, currentSchemaVersion)
	}
	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, m.sql); err == nil {
			_, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations(version,applied_at) VALUES(?,strftime('%Y-%m-%dT%H:%M:%fZ','now'))`, m.version)
		}
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %d: %w", m.version, err)
		}
		if err = tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.version, err)
		}
	}
	return nil
}
