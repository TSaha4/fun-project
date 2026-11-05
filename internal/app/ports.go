package app

import (
	"context"
	"time"

	"example.com/local-capture/internal/domain"
)

type NoteRepository interface {
	CreateNote(context.Context, domain.Note) error
	Get(context.Context, string) (domain.Note, error)
	Update(context.Context, domain.Note) error
	Delete(context.Context, string) error
	List(context.Context, ListNotesQuery) ([]domain.Note, error)
}

type CollectionRepository interface {
	CreateCollection(context.Context, domain.Collection) error
	GetCollection(context.Context, string) (domain.Collection, error)
	ListCollections(context.Context) ([]domain.Collection, error)
}

// NoteStats is aggregate metadata and never includes note content.
type NoteStats struct {
	Total     int `json:"total"`
	Archived  int `json:"archived"`
	Pinned    int `json:"pinned"`
	WithTasks int `json:"with_tasks"`
}

type NoteStatistics interface {
	NoteStats(context.Context) (NoteStats, error)
}

type TagRepository interface {
	GetMany(context.Context, []string) ([]domain.Tag, error)
	CreateTag(context.Context, domain.Tag) error
}

type Searcher interface {
	Search(context.Context, SearchQuery) ([]domain.Note, error)
}

// ReminderRepository persists reminder state, lifecycle history and notification claims.
// ClaimNotification must be atomic and idempotent for a reminder/due-instant pair.
type ReminderRepository interface {
	CreateReminder(context.Context, domain.Reminder, domain.ReminderEvent) error
	GetReminder(context.Context, string) (domain.Reminder, error)
	UpdateReminder(context.Context, domain.Reminder, domain.ReminderEvent) error
	ListReminders(context.Context, bool) ([]domain.Reminder, error)
	ListDueReminders(context.Context, time.Time) ([]domain.Reminder, error)
	NextReminderDue(context.Context) (time.Time, bool, error)
	ClaimNotification(context.Context, string, time.Time, time.Time) (bool, error)
	ListReminderHistory(context.Context, string) ([]domain.ReminderEvent, error)
}

type IDGenerator interface{ NewID() (string, error) }
type Clock interface{ Now() time.Time }

type ListNotesQuery struct {
	CollectionID  string
	TagID         string
	InboxOnly     bool
	Type          domain.NoteType
	Pinned        *bool
	Archived      *bool
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	Limit         int
	Offset        int
}
type SearchQuery struct {
	Text          string
	CollectionID  string
	TagIDs        []string
	Type          domain.NoteType
	Pinned        *bool
	Archived      *bool
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	Limit         int
	Offset        int
}
