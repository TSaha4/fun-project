package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.com/local-capture/internal/app"
	"example.com/local-capture/internal/domain"
	_ "modernc.org/sqlite"
)

const currentSchemaVersion = 2

type Store struct{ db *sql.DB }

var (
	_ app.NoteRepository       = (*Store)(nil)
	_ app.CollectionRepository = (*Store)(nil)
	_ app.TagRepository        = (*Store)(nil)
	_ app.Searcher             = (*Store)(nil)
)

func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	// A single pooled connection keeps connection-scoped foreign-key enforcement
	// consistent; SQLite still serializes writers across separate Store instances.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA busy_timeout = 5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set busy timeout: %w", err)
	}
	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) CreateNote(ctx context.Context, note domain.Note) error {
	if err := note.Validate(); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO notes(id,title,content,type,created_at,updated_at,collection_id,pinned,archived,source_url) VALUES(?,?,?,?,?,?,?,?,?,?)`, note.ID, note.Title, note.Content, string(note.Type), formatTime(note.CreatedAt), formatTime(note.UpdatedAt), nullableString(note.CollectionID), boolInt(note.Pinned), boolInt(note.Archived), nullableString(note.Metadata.SourceURL))
	if err != nil {
		return fmt.Errorf("insert note: %w", err)
	}
	if err = insertRelations(ctx, tx, note); err != nil {
		return err
	}
	if note.Reminder != nil {
		r := *note.Reminder
		r.ID, r.NoteID = note.ID, note.ID
		if err := insertReminder(ctx, tx, r, note.CreatedAt, note.UpdatedAt); err != nil {
			return fmt.Errorf("insert reminder: %w", err)
		}
		if err := insertReminderEvent(ctx, tx, domain.ReminderEvent{ID: "created:" + note.ID, ReminderID: note.ID, Kind: domain.ReminderEventCreated, OccurredAt: note.CreatedAt, DueAt: r.EffectiveDueAt()}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Get(ctx context.Context, id string) (domain.Note, error) {
	var n domain.Note
	var kind, created, updated string
	var collection, source sql.NullString
	var pinned, archived int
	err := s.db.QueryRowContext(ctx, `SELECT id,title,content,type,created_at,updated_at,collection_id,pinned,archived,source_url FROM notes WHERE id=?`, id).Scan(&n.ID, &n.Title, &n.Content, &kind, &created, &updated, &collection, &pinned, &archived, &source)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Note{}, app.ErrNotFound
	}
	if err != nil {
		return domain.Note{}, err
	}
	n.Type = domain.NoteType(kind)
	n.CreatedAt, err = parseTime(created)
	if err != nil {
		return domain.Note{}, err
	}
	n.UpdatedAt, err = parseTime(updated)
	if err != nil {
		return domain.Note{}, err
	}
	n.CollectionID = collection.String
	n.Pinned = pinned != 0
	n.Archived = archived != 0
	n.Metadata.SourceURL = source.String
	rows, err := s.db.QueryContext(ctx, `SELECT tag_id FROM note_tags WHERE note_id=? ORDER BY tag_id`, id)
	if err != nil {
		return domain.Note{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var tagID string
		if err := rows.Scan(&tagID); err != nil {
			return domain.Note{}, err
		}
		n.TagIDs = append(n.TagIDs, tagID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return domain.Note{}, err
	}
	if err := rows.Close(); err != nil {
		return domain.Note{}, err
	}
	rows, err = s.db.QueryContext(ctx, `SELECT attachment_id FROM note_attachments WHERE note_id=? ORDER BY attachment_id`, id)
	if err != nil {
		return domain.Note{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var attachmentID string
		if err := rows.Scan(&attachmentID); err != nil {
			return domain.Note{}, err
		}
		n.AttachmentIDs = append(n.AttachmentIDs, attachmentID)
	}
	if err := rows.Err(); err != nil {
		return domain.Note{}, err
	}
	if err := rows.Close(); err != nil {
		return domain.Note{}, err
	}
	var due sql.NullString
	err = s.db.QueryRowContext(ctx, `SELECT due_at FROM reminders WHERE note_id=?`, id).Scan(&due)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.Note{}, err
	}
	if due.Valid {
		t, e := parseTime(due.String)
		if e != nil {
			return domain.Note{}, e
		}
		var snoozed, completed sql.NullString
		if err := s.db.QueryRowContext(ctx, `SELECT snoozed_until,completed_at FROM reminders WHERE note_id=?`, id).Scan(&snoozed, &completed); err != nil {
			return domain.Note{}, err
		}
		r := &domain.Reminder{ID: id, NoteID: id, DueAt: t}
		if snoozed.Valid {
			v, e := parseTime(snoozed.String)
			if e != nil {
				return domain.Note{}, e
			}
			r.SnoozedUntil = &v
		}
		if completed.Valid {
			v, e := parseTime(completed.String)
			if e != nil {
				return domain.Note{}, e
			}
			r.CompletedAt = &v
		}
		n.Reminder = r
	}
	return n, nil
}

func (s *Store) Update(ctx context.Context, note domain.Note) error {
	if err := note.Validate(); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE notes SET title=?,content=?,type=?,updated_at=?,collection_id=?,pinned=?,archived=?,source_url=? WHERE id=?`, note.Title, note.Content, string(note.Type), formatTime(note.UpdatedAt), nullableString(note.CollectionID), boolInt(note.Pinned), boolInt(note.Archived), nullableString(note.Metadata.SourceURL), note.ID)
	if err != nil {
		return fmt.Errorf("update note: %w", err)
	}
	count, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return app.ErrNotFound
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM note_tags WHERE note_id=?`, note.ID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM note_attachments WHERE note_id=?`, note.ID); err != nil {
		return err
	}
	if err = insertRelations(ctx, tx, note); err != nil {
		return err
	}
	// Reminder lifecycle is managed separately; note edits cannot reset snooze/history.
	return tx.Commit()
}

func (s *Store) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM notes WHERE id=?`, id)
	if err != nil {
		return err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return app.ErrNotFound
	}
	return nil
}

func (s *Store) List(ctx context.Context, q app.ListNotesQuery) ([]domain.Note, error) {
	return s.queryNotes(ctx, `SELECT n.id FROM notes n WHERE 1=1`, nil, q.CollectionID, q.TagID, q.Type, q.Pinned, q.Archived, q.CreatedAfter, q.CreatedBefore, q.Limit, q.Offset)
}

func (s *Store) Search(ctx context.Context, q app.SearchQuery) ([]domain.Note, error) {
	base := `SELECT n.id FROM notes n JOIN notes_fts f ON f.note_id=n.id WHERE 1=1`
	args := []any{}
	if strings.TrimSpace(q.Text) != "" {
		base += ` AND notes_fts MATCH ?`
		args = append(args, ftsQuery(q.Text))
	}
	if len(q.TagIDs) > 0 {
		base += ` AND EXISTS(SELECT 1 FROM note_tags nt WHERE nt.note_id=n.id AND nt.tag_id IN (` + placeholders(len(q.TagIDs)) + `))`
		for _, id := range q.TagIDs {
			args = append(args, id)
		}
	}
	return s.queryNotes(ctx, base, args, q.CollectionID, "", q.Type, q.Pinned, q.Archived, q.CreatedAfter, q.CreatedBefore, q.Limit, q.Offset)
}

func (s *Store) queryNotes(ctx context.Context, base string, args []any, collectionID, tagID string, noteType domain.NoteType, pinned, archived *bool, after, before *time.Time, limit, offset int) ([]domain.Note, error) {
	if collectionID != "" {
		base += ` AND n.collection_id=?`
		args = append(args, collectionID)
	}
	if tagID != "" {
		base += ` AND EXISTS(SELECT 1 FROM note_tags nt WHERE nt.note_id=n.id AND nt.tag_id=?)`
		args = append(args, tagID)
	}
	if noteType != "" {
		base += ` AND n.type=?`
		args = append(args, string(noteType))
	}
	if pinned != nil {
		base += ` AND n.pinned=?`
		args = append(args, boolInt(*pinned))
	}
	if archived != nil {
		base += ` AND n.archived=?`
	} else {
		base += ` AND n.archived=0`
	}
	if archived != nil {
		args = append(args, boolInt(*archived))
	}
	if after != nil {
		base += ` AND n.created_at>=?`
		args = append(args, formatTime(*after))
	}
	if before != nil {
		base += ` AND n.created_at<?`
		args = append(args, formatTime(*before))
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	base += ` ORDER BY n.pinned DESC,n.updated_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, base, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	result := make([]domain.Note, 0, len(ids))
	for _, id := range ids {
		note, err := s.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		result = append(result, note)
	}
	return result, nil
}

func insertRelations(ctx context.Context, tx *sql.Tx, note domain.Note) error {
	for _, id := range unique(note.TagIDs) {
		if _, err := tx.ExecContext(ctx, `INSERT INTO note_tags(note_id,tag_id) VALUES(?,?)`, note.ID, id); err != nil {
			return fmt.Errorf("insert note tag: %w", err)
		}
	}
	for _, id := range unique(note.AttachmentIDs) {
		if _, err := tx.ExecContext(ctx, `INSERT INTO note_attachments(note_id,attachment_id) VALUES(?,?)`, note.ID, id); err != nil {
			return fmt.Errorf("insert attachment reference: %w", err)
		}
	}
	return nil
}

func insertReminder(ctx context.Context, tx *sql.Tx, reminder domain.Reminder, createdAt, updatedAt time.Time) error {
	var snoozed, completed any
	if reminder.SnoozedUntil != nil {
		snoozed = formatTime(*reminder.SnoozedUntil)
	}
	if reminder.CompletedAt != nil {
		completed = formatTime(*reminder.CompletedAt)
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO reminders(note_id,due_at,snoozed_until,completed_at,created_at,updated_at,effective_due_at) VALUES(?,?,?,?,?,?,?)`, reminder.NoteID, formatTime(reminder.DueAt), snoozed, completed, formatTime(createdAt), formatTime(updatedAt), formatTime(reminder.EffectiveDueAt()))
	return err
}

func insertReminderEvent(ctx context.Context, tx *sql.Tx, event domain.ReminderEvent) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO reminder_events(id,reminder_id,kind,occurred_at,due_at) VALUES(?,?,?,?,?)`, event.ID, event.ReminderID, string(event.Kind), formatTime(event.OccurredAt), formatTime(event.DueAt))
	return err
}

func unique(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range values {
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
func placeholders(n int) string {
	if n < 1 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}
func ftsQuery(raw string) string {
	parts := strings.Fields(raw)
	for i, p := range parts {
		parts[i] = `"` + strings.ReplaceAll(p, `"`, `""`) + `"`
	}
	return strings.Join(parts, " ")
}
func nullableString(v string) any {
	if v == "" {
		return nil
	}
	return v
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
func formatTime(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000000000Z") }
func parseTime(v string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse persisted timestamp: %w", err)
	}
	return t, nil
}
