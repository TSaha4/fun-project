package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/local-capture/internal/domain"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type fixedIDs struct{ next string }

func (g *fixedIDs) NewID() (string, error) { id := g.next; g.next += "x"; return id, nil }

type memoryNotes struct{ values map[string]domain.Note }

func (m *memoryNotes) CreateNote(_ context.Context, n domain.Note) error {
	if m.values == nil {
		m.values = map[string]domain.Note{}
	}
	m.values[n.ID] = cloneNote(n)
	return nil
}
func (m *memoryNotes) Get(_ context.Context, id string) (domain.Note, error) {
	n, ok := m.values[id]
	if !ok {
		return domain.Note{}, ErrNotFound
	}
	return cloneNote(n), nil
}
func (m *memoryNotes) Update(_ context.Context, n domain.Note) error {
	if _, ok := m.values[n.ID]; !ok {
		return ErrNotFound
	}
	m.values[n.ID] = cloneNote(n)
	return nil
}
func (m *memoryNotes) Delete(_ context.Context, id string) error {
	if _, ok := m.values[id]; !ok {
		return ErrNotFound
	}
	delete(m.values, id)
	return nil
}
func (m *memoryNotes) List(_ context.Context, _ ListNotesQuery) ([]domain.Note, error) {
	out := []domain.Note{}
	for _, n := range m.values {
		out = append(out, cloneNote(n))
	}
	return out, nil
}

type memoryCollections struct{ values map[string]domain.Collection }

func (m *memoryCollections) CreateCollection(_ context.Context, c domain.Collection) error {
	if m.values == nil {
		m.values = map[string]domain.Collection{}
	}
	m.values[c.ID] = c
	return nil
}
func (m *memoryCollections) GetCollection(_ context.Context, id string) (domain.Collection, error) {
	c, ok := m.values[id]
	if !ok {
		return domain.Collection{}, ErrNotFound
	}
	return c, nil
}
func (m *memoryCollections) ListCollections(_ context.Context) ([]domain.Collection, error) {
	out := []domain.Collection{}
	for _, c := range m.values {
		out = append(out, c)
	}
	return out, nil
}

type memoryTags struct{ values map[string]domain.Tag }

func (m *memoryTags) CreateTag(_ context.Context, tag domain.Tag) error {
	if m.values == nil {
		m.values = map[string]domain.Tag{}
	}
	m.values[tag.ID] = tag
	return nil
}
func (m *memoryTags) GetMany(_ context.Context, ids []string) ([]domain.Tag, error) {
	found := []domain.Tag{}
	for _, id := range ids {
		if tag, ok := m.values[id]; ok {
			found = append(found, tag)
		}
	}
	return found, nil
}

type noSearch struct{}

func (noSearch) Search(context.Context, SearchQuery) ([]domain.Note, error) { return nil, nil }

func cloneNote(n domain.Note) domain.Note {
	n.TagIDs = append([]string(nil), n.TagIDs...)
	n.AttachmentIDs = append([]string(nil), n.AttachmentIDs...)
	if n.Reminder != nil {
		r := *n.Reminder
		n.Reminder = &r
	}
	return n
}

func TestServiceCreateUpdateArchiveRestoreAndCollectionRelationships(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	notes := &memoryNotes{}
	collections := &memoryCollections{}
	tags := &memoryTags{}
	service := NewService(notes, collections, tags, noSearch{}, &fixedIDs{next: "id"}, fixedClock{now})
	collection, err := service.CreateCollection(ctx, "  Research ")
	if err != nil || collection.Name != "Research" {
		t.Fatalf("collection: %+v, %v", collection, err)
	}
	tag, _ := domain.NewTag("tag", "reading", now)
	_ = tags.CreateTag(ctx, tag)
	note, err := service.CreateNote(ctx, CreateNoteInput{Title: "First", Content: "body", Type: domain.NoteTypeIdea, CollectionID: collection.ID, TagIDs: []string{tag.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if note.CollectionID != collection.ID || len(note.TagIDs) != 1 {
		t.Fatalf("relationships not assigned: %+v", note)
	}
	updated, err := service.UpdateNote(ctx, note.ID, UpdateNoteInput{Title: "Updated", Content: "revised", Type: domain.NoteTypeThought})
	if err != nil || updated.Title != "Updated" {
		t.Fatalf("update: %+v %v", updated, err)
	}
	archived, err := service.ArchiveNote(ctx, note.ID)
	if err != nil || !archived.Archived {
		t.Fatalf("archive: %+v %v", archived, err)
	}
	restored, err := service.RestoreNote(ctx, note.ID)
	if err != nil || restored.Archived {
		t.Fatalf("restore: %+v %v", restored, err)
	}
	if err := service.DeleteNote(ctx, note.ID); err != nil {
		t.Fatal(err)
	}
}

func TestServiceRejectsMissingCollectionAndTag(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	service := NewService(&memoryNotes{}, &memoryCollections{}, &memoryTags{}, noSearch{}, &fixedIDs{next: "id"}, fixedClock{now})
	_, err := service.CreateNote(ctx, CreateNoteInput{Title: "Note", Type: domain.NoteTypeThought, CollectionID: "missing"})
	if !errors.Is(err, ErrInvalidRelationship) {
		t.Fatalf("expected relationship error, got %v", err)
	}
	_, err = service.CreateNote(ctx, CreateNoteInput{Title: "Note", Type: domain.NoteTypeThought, TagIDs: []string{"missing"}})
	if !errors.Is(err, ErrInvalidRelationship) {
		t.Fatalf("expected tag relationship error, got %v", err)
	}
}
