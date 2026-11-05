package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"example.com/local-capture/internal/app"
	"example.com/local-capture/internal/domain"
)

func (s *Store) CreateCollection(ctx context.Context, c domain.Collection) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO collections(id,name,created_at,updated_at) VALUES(?,?,?,?)`, c.ID, c.Name, formatTime(c.CreatedAt), formatTime(c.UpdatedAt))
	if err != nil {
		return fmt.Errorf("insert collection: %w", err)
	}
	return nil
}

func (s *Store) GetCollection(ctx context.Context, id string) (domain.Collection, error) {
	var c domain.Collection
	var created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT id,name,created_at,updated_at FROM collections WHERE id=?`, id).Scan(&c.ID, &c.Name, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Collection{}, app.ErrNotFound
	}
	if err != nil {
		return domain.Collection{}, err
	}
	c.CreatedAt, err = parseTime(created)
	if err != nil {
		return domain.Collection{}, err
	}
	c.UpdatedAt, err = parseTime(updated)
	return c, err
}

func (s *Store) CreateTag(ctx context.Context, t domain.Tag) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO tags(id,name,created_at) VALUES(?,?,?)`, t.ID, t.Name, formatTime(t.CreatedAt))
	if err != nil {
		return fmt.Errorf("insert tag: %w", err)
	}
	return nil
}

func (s *Store) GetMany(ctx context.Context, ids []string) ([]domain.Tag, error) {
	if len(ids) == 0 {
		return []domain.Tag{}, nil
	}
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,created_at FROM tags WHERE id IN (`+placeholders(len(ids))+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.Tag{}
	for rows.Next() {
		var t domain.Tag
		var created string
		if err := rows.Scan(&t.ID, &t.Name, &created); err != nil {
			return nil, err
		}
		t.CreatedAt, err = parseTime(created)
		if err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, rows.Err()
}

func (s *Store) CreateAttachment(ctx context.Context, a domain.Attachment) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO attachments(id,file_name,media_type,size,checksum,created_at) VALUES(?,?,?,?,?,?)`, a.ID, a.FileName, a.MediaType, a.Size, a.Checksum, formatTime(a.CreatedAt))
	if err != nil {
		return fmt.Errorf("insert attachment metadata: %w", err)
	}
	return nil
}

func (s *Store) GetAttachment(ctx context.Context, id string) (domain.Attachment, error) {
	var a domain.Attachment
	var created string
	err := s.db.QueryRowContext(ctx, `SELECT id,file_name,media_type,size,checksum,created_at FROM attachments WHERE id=?`, id).Scan(&a.ID, &a.FileName, &a.MediaType, &a.Size, &a.Checksum, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Attachment{}, app.ErrNotFound
	}
	if err != nil {
		return domain.Attachment{}, err
	}
	a.CreatedAt, err = parseTime(created)
	return a, err
}

func (s *Store) Attach(ctx context.Context, noteID, attachmentID string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO note_attachments(note_id,attachment_id) VALUES(?,?)`, noteID, attachmentID)
	if err != nil {
		return fmt.Errorf("associate attachment: %w", err)
	}
	return nil
}
