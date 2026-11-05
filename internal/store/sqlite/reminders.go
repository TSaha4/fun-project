package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"example.com/local-capture/internal/app"
	"example.com/local-capture/internal/domain"
)

var _ app.ReminderRepository = (*Store)(nil)

func (s *Store) CreateReminder(ctx context.Context, r domain.Reminder, event domain.ReminderEvent) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM reminders WHERE note_id=?`, r.NoteID).Scan(&exists); err != nil {
		return err
	}
	if exists != 0 {
		return app.ErrReminderExists
	}
	if err := insertReminder(ctx, tx, r, event.OccurredAt, event.OccurredAt); err != nil {
		return fmt.Errorf("insert reminder: %w", err)
	}
	if err := insertReminderEvent(ctx, tx, event); err != nil {
		return fmt.Errorf("record reminder creation: %w", err)
	}
	return tx.Commit()
}

func (s *Store) GetReminder(ctx context.Context, id string) (domain.Reminder, error) {
	var r domain.Reminder
	var due string
	var snoozed, completed sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT note_id,due_at,snoozed_until,completed_at FROM reminders WHERE note_id=?`, id).Scan(&r.NoteID, &due, &snoozed, &completed)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Reminder{}, app.ErrNotFound
	}
	if err != nil {
		return domain.Reminder{}, err
	}
	r.ID = r.NoteID
	r.DueAt, err = parseTime(due)
	if err != nil {
		return domain.Reminder{}, err
	}
	if snoozed.Valid {
		v, e := parseTime(snoozed.String)
		if e != nil {
			return domain.Reminder{}, e
		}
		r.SnoozedUntil = &v
	}
	if completed.Valid {
		v, e := parseTime(completed.String)
		if e != nil {
			return domain.Reminder{}, e
		}
		r.CompletedAt = &v
	}
	return r, nil
}

func (s *Store) UpdateReminder(ctx context.Context, r domain.Reminder, event domain.ReminderEvent) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var snoozed, completed any
	if r.SnoozedUntil != nil {
		snoozed = formatTime(*r.SnoozedUntil)
	}
	if r.CompletedAt != nil {
		completed = formatTime(*r.CompletedAt)
	}
	res, err := tx.ExecContext(ctx, `UPDATE reminders SET due_at=?,snoozed_until=?,completed_at=?,updated_at=?,effective_due_at=? WHERE note_id=?`, formatTime(r.DueAt), snoozed, completed, formatTime(event.OccurredAt), formatTime(r.EffectiveDueAt()), r.NoteID)
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
	if err := insertReminderEvent(ctx, tx, event); err != nil {
		return fmt.Errorf("record reminder update: %w", err)
	}
	return tx.Commit()
}

func (s *Store) ListReminders(ctx context.Context, includeCompleted bool) ([]domain.Reminder, error) {
	query := `SELECT r.note_id,r.due_at,r.snoozed_until,r.completed_at FROM reminders r WHERE 1=1`
	if !includeCompleted {
		query += ` AND r.completed_at IS NULL`
	}
	query += ` ORDER BY r.effective_due_at,r.note_id`
	return s.queryReminders(ctx, query)
}

func (s *Store) ListDueReminders(ctx context.Context, now time.Time) ([]domain.Reminder, error) {
	query := `SELECT r.note_id,r.due_at,r.snoozed_until,r.completed_at FROM reminders r WHERE r.completed_at IS NULL AND r.effective_due_at<=? ORDER BY r.effective_due_at,r.note_id`
	return s.queryReminders(ctx, query, formatTime(now))
}

func (s *Store) NextReminderDue(ctx context.Context) (time.Time, bool, error) {
	query := `SELECT MIN(r.effective_due_at) FROM reminders r WHERE r.completed_at IS NULL AND NOT EXISTS (SELECT 1 FROM reminder_notification_claims c WHERE c.reminder_id=r.note_id AND c.scheduled_for=r.effective_due_at)`
	var value sql.NullString
	if err := s.db.QueryRowContext(ctx, query).Scan(&value); err != nil {
		return time.Time{}, false, err
	}
	if !value.Valid {
		return time.Time{}, false, nil
	}
	t, err := parseTime(value.String)
	return t, err == nil, err
}

func (s *Store) ClaimNotification(ctx context.Context, reminderID string, dueAt, claimedAt time.Time) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	scheduled := formatTime(dueAt)
	res, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO reminder_notification_claims(reminder_id,scheduled_for,claimed_at) VALUES(?,?,?)`, reminderID, scheduled, formatTime(claimedAt))
	if err != nil {
		return false, err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if count == 0 {
		return false, tx.Commit()
	}
	eventID := "notification:" + reminderID + ":" + scheduled
	event := domain.ReminderEvent{ID: eventID, ReminderID: reminderID, Kind: domain.ReminderEventNotificationClaimed, OccurredAt: claimedAt.UTC(), DueAt: dueAt.UTC()}
	if err := insertReminderEvent(ctx, tx, event); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) ListReminderHistory(ctx context.Context, reminderID string) ([]domain.ReminderEvent, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,reminder_id,kind,occurred_at,due_at FROM reminder_events WHERE reminder_id=? ORDER BY rowid`, reminderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.ReminderEvent{}
	for rows.Next() {
		var e domain.ReminderEvent
		var kind, occurred, due string
		if err := rows.Scan(&e.ID, &e.ReminderID, &kind, &occurred, &due); err != nil {
			return nil, err
		}
		e.Kind = domain.ReminderEventKind(kind)
		e.OccurredAt, err = parseTime(occurred)
		if err != nil {
			return nil, err
		}
		e.DueAt, err = parseTime(due)
		if err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

func (s *Store) queryReminders(ctx context.Context, query string, args ...any) ([]domain.Reminder, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.Reminder{}
	for rows.Next() {
		var r domain.Reminder
		var due string
		var snoozed, completed sql.NullString
		if err := rows.Scan(&r.NoteID, &due, &snoozed, &completed); err != nil {
			return nil, err
		}
		r.ID = r.NoteID
		r.DueAt, err = parseTime(due)
		if err != nil {
			return nil, err
		}
		if snoozed.Valid {
			v, e := parseTime(snoozed.String)
			if e != nil {
				return nil, e
			}
			r.SnoozedUntil = &v
		}
		if completed.Valid {
			v, e := parseTime(completed.String)
			if e != nil {
				return nil, e
			}
			r.CompletedAt = &v
		}
		result = append(result, r)
	}
	return result, rows.Err()
}
