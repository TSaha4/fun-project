package app

import (
	"context"
	"fmt"
	"time"

	"example.com/local-capture/internal/domain"
)

// ReminderService owns reminder lifecycle behavior separately from note editing.
// A note has at most one reminder; its stable reminder ID is the note ID.
type ReminderService struct {
	reminders ReminderRepository
	notes     NoteRepository
	ids       IDGenerator
	clock     Clock
	wake      func()
}

func (s *ReminderService) SetScheduleWake(wake func()) { s.wake = wake }

func (s *ReminderService) scheduleChanged() {
	if s.wake != nil {
		s.wake()
	}
}

func NewReminderService(reminders ReminderRepository, notes NoteRepository, ids IDGenerator, clock Clock) *ReminderService {
	return &ReminderService{reminders: reminders, notes: notes, ids: ids, clock: clock}
}

func (s *ReminderService) CreateReminder(ctx context.Context, noteID string, dueAt time.Time) (domain.Reminder, error) {
	if noteID == "" || dueAt.IsZero() {
		return domain.Reminder{}, domain.ErrInvalidReminder
	}
	note, err := s.notes.Get(ctx, noteID)
	if err != nil {
		return domain.Reminder{}, err
	}
	if note.Archived {
		return domain.Reminder{}, domain.ErrArchived
	}
	now := s.clock.Now().UTC()
	dueAt = dueAt.UTC()
	r := domain.Reminder{ID: noteID, NoteID: noteID, DueAt: dueAt}
	event, err := s.event(noteID, domain.ReminderEventCreated, now, dueAt)
	if err != nil {
		return domain.Reminder{}, err
	}
	if err := s.reminders.CreateReminder(ctx, r, event); err != nil {
		return domain.Reminder{}, err
	}
	s.scheduleChanged()
	return r, nil
}

func (s *ReminderService) GetReminder(ctx context.Context, id string) (domain.Reminder, error) {
	return s.reminders.GetReminder(ctx, id)
}

func (s *ReminderService) UpdateReminder(ctx context.Context, id string, dueAt time.Time) (domain.Reminder, error) {
	if dueAt.IsZero() {
		return domain.Reminder{}, domain.ErrInvalidReminder
	}
	r, err := s.reminders.GetReminder(ctx, id)
	if err != nil {
		return domain.Reminder{}, err
	}
	if r.IsCompleted() {
		return domain.Reminder{}, domain.ErrReminderCompleted
	}
	r.DueAt = dueAt.UTC()
	r.SnoozedUntil = nil
	event, err := s.event(id, domain.ReminderEventUpdated, s.clock.Now(), r.DueAt)
	if err != nil {
		return domain.Reminder{}, err
	}
	if err := s.reminders.UpdateReminder(ctx, r, event); err != nil {
		return domain.Reminder{}, err
	}
	s.scheduleChanged()
	return r, nil
}

func (s *ReminderService) SnoozeReminder(ctx context.Context, id string, until time.Time) (domain.Reminder, error) {
	now := s.clock.Now()
	if until.IsZero() || !until.After(now) {
		return domain.Reminder{}, domain.ErrInvalidSnooze
	}
	r, err := s.reminders.GetReminder(ctx, id)
	if err != nil {
		return domain.Reminder{}, err
	}
	if r.IsCompleted() {
		return domain.Reminder{}, domain.ErrReminderCompleted
	}
	value := until.UTC()
	r.SnoozedUntil = &value
	event, err := s.event(id, domain.ReminderEventSnoozed, now, value)
	if err != nil {
		return domain.Reminder{}, err
	}
	if err := s.reminders.UpdateReminder(ctx, r, event); err != nil {
		return domain.Reminder{}, err
	}
	s.scheduleChanged()
	return r, nil
}

func (s *ReminderService) CompleteReminder(ctx context.Context, id string) (domain.Reminder, error) {
	r, err := s.reminders.GetReminder(ctx, id)
	if err != nil {
		return domain.Reminder{}, err
	}
	if r.IsCompleted() {
		return r, nil
	}
	now := s.clock.Now().UTC()
	r.CompletedAt = &now
	event, err := s.event(id, domain.ReminderEventCompleted, now, r.EffectiveDueAt())
	if err != nil {
		return domain.Reminder{}, err
	}
	if err := s.reminders.UpdateReminder(ctx, r, event); err != nil {
		return domain.Reminder{}, err
	}
	s.scheduleChanged()
	return r, nil
}

func (s *ReminderService) ListReminders(ctx context.Context) ([]domain.Reminder, error) {
	return s.reminders.ListReminders(ctx, false)
}

func (s *ReminderService) ListAllReminders(ctx context.Context) ([]domain.Reminder, error) {
	return s.reminders.ListReminders(ctx, true)
}

func (s *ReminderService) ListDueReminders(ctx context.Context) ([]domain.Reminder, error) {
	return s.reminders.ListDueReminders(ctx, s.clock.Now())
}

func (s *ReminderService) ListReminderHistory(ctx context.Context, id string) ([]domain.ReminderEvent, error) {
	if _, err := s.reminders.GetReminder(ctx, id); err != nil {
		return nil, err
	}
	return s.reminders.ListReminderHistory(ctx, id)
}

func (s *ReminderService) event(reminderID string, kind domain.ReminderEventKind, at, dueAt time.Time) (domain.ReminderEvent, error) {
	id, err := s.ids.NewID()
	if err != nil {
		return domain.ReminderEvent{}, fmt.Errorf("generate reminder event ID: %w", err)
	}
	return domain.ReminderEvent{ID: id, ReminderID: reminderID, Kind: kind, OccurredAt: at.UTC(), DueAt: dueAt.UTC()}, nil
}
