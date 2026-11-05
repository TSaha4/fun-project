package domain

import (
	"net/url"
	"strings"
	"time"
)

type NoteType string

const (
	NoteTypeThought   NoteType = "thought"
	NoteTypeIdea      NoteType = "idea"
	NoteTypeTask      NoteType = "task"
	NoteTypeLink      NoteType = "link"
	NoteTypeImage     NoteType = "image"
	NoteTypeQuote     NoteType = "quote"
	NoteTypeCode      NoteType = "code_snippet"
	NoteTypeReference NoteType = "reference"
)

func (t NoteType) Valid() bool {
	switch t {
	case NoteTypeThought, NoteTypeIdea, NoteTypeTask, NoteTypeLink, NoteTypeImage, NoteTypeQuote, NoteTypeCode, NoteTypeReference:
		return true
	default:
		return false
	}
}

type Reminder struct {
	ID           string     `json:"id,omitempty"`
	NoteID       string     `json:"note_id,omitempty"`
	DueAt        time.Time  `json:"due_at"`
	SnoozedUntil *time.Time `json:"snoozed_until,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

func (r Reminder) EffectiveDueAt() time.Time {
	if r.SnoozedUntil != nil && r.SnoozedUntil.After(r.DueAt) {
		return *r.SnoozedUntil
	}
	return r.DueAt
}

func (r Reminder) IsCompleted() bool { return r.CompletedAt != nil }

func NewReminder(dueAt time.Time) (Reminder, error) {
	if dueAt.IsZero() {
		return Reminder{}, ErrInvalidReminder
	}
	return Reminder{DueAt: dueAt}, nil
}

type NoteMetadata struct {
	SourceURL string `json:"source_url,omitempty"`
}

type Note struct {
	ID            string       `json:"id"`
	Title         string       `json:"title"`
	Content       string       `json:"content"`
	Type          NoteType     `json:"type"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`
	CollectionID  string       `json:"collection_id,omitempty"`
	TagIDs        []string     `json:"tag_ids"`
	Pinned        bool         `json:"pinned"`
	Archived      bool         `json:"archived"`
	Reminder      *Reminder    `json:"reminder,omitempty"`
	Metadata      NoteMetadata `json:"metadata"`
	AttachmentIDs []string     `json:"attachment_ids"`
}

func NewNote(id, title, content string, noteType NoteType, now time.Time) (Note, error) {
	n := Note{ID: id, Title: strings.TrimSpace(title), Content: content, Type: noteType, CreatedAt: now, UpdatedAt: now}
	if err := n.Validate(); err != nil {
		return Note{}, err
	}
	return n, nil
}

func (n Note) Validate() error {
	if strings.TrimSpace(n.ID) == "" {
		return ErrInvalidID
	}
	if strings.TrimSpace(n.Title) == "" {
		return ErrInvalidTitle
	}
	if !n.Type.Valid() {
		return ErrInvalidType
	}
	if n.CreatedAt.IsZero() || n.UpdatedAt.IsZero() || n.UpdatedAt.Before(n.CreatedAt) {
		return ErrInvalidTimestamp
	}
	if n.Metadata.SourceURL != "" {
		u, err := url.ParseRequestURI(n.Metadata.SourceURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return ErrInvalidURL
		}
	}
	return nil
}

func (n *Note) Update(title, content string, noteType NoteType, sourceURL string, reminder *Reminder, attachmentIDs []string, now time.Time) error {
	if n.Archived {
		return ErrArchived
	}
	candidate := *n
	candidate.Title, candidate.Content, candidate.Type = strings.TrimSpace(title), content, noteType
	candidate.Metadata.SourceURL, candidate.Reminder = sourceURL, cloneReminder(reminder)
	candidate.AttachmentIDs = append([]string(nil), attachmentIDs...)
	candidate.UpdatedAt = now
	if err := candidate.Validate(); err != nil {
		return err
	}
	*n = candidate
	return nil
}

func (n *Note) Archive(now time.Time) {
	if !n.Archived {
		n.Archived, n.UpdatedAt = true, now
	}
}
func (n *Note) Restore(now time.Time) {
	if n.Archived {
		n.Archived, n.UpdatedAt = false, now
	}
}

func (n *Note) SetPinned(pinned bool, now time.Time) error {
	if n.Archived {
		return ErrArchived
	}
	n.Pinned, n.UpdatedAt = pinned, now
	return nil
}

func (n *Note) SetCollection(collectionID string, now time.Time) error {
	if n.Archived {
		return ErrArchived
	}
	n.CollectionID, n.UpdatedAt = collectionID, now
	return nil
}

func (n *Note) SetTags(tagIDs []string, now time.Time) error {
	if n.Archived {
		return ErrArchived
	}
	n.TagIDs, n.UpdatedAt = uniqueIDs(tagIDs), now
	return nil
}

func cloneReminder(r *Reminder) *Reminder {
	if r == nil {
		return nil
	}
	c := *r
	return &c
}

func uniqueIDs(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
