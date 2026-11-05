package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewNoteValidatesRequiredFieldsAndType(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, test := range []struct {
		name, id, title string
		kind            NoteType
		want            error
	}{
		{"missing id", "", "title", NoteTypeThought, ErrInvalidID},
		{"missing title", "id", "  ", NoteTypeThought, ErrInvalidTitle},
		{"invalid type", "id", "title", "other", ErrInvalidType},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewNote(test.id, test.title, "body", test.kind, now); !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}
		})
	}
	if _, err := NewNote("id", " title ", "body", NoteTypeIdea, now); err != nil {
		t.Fatal(err)
	}
}

func TestNoteUpdateArchiveRestoreAndRelationships(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	note, err := NewNote("id", "title", "body", NoteTypeThought, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := note.Update("updated", "new body", NoteTypeCode, "https://example.test/path", nil, []string{"a"}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if note.Title != "updated" || note.Type != NoteTypeCode || note.UpdatedAt != now.Add(time.Minute) {
		t.Fatalf("update not applied: %+v", note)
	}
	if err := note.SetCollection("collection", now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := note.SetTags([]string{"tag", "tag", ""}, now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if note.CollectionID != "collection" || len(note.TagIDs) != 1 {
		t.Fatalf("relationships not applied: %+v", note)
	}
	note.Archive(now.Add(4 * time.Minute))
	if !note.Archived {
		t.Fatal("expected archived")
	}
	if err := note.Update("blocked", "", NoteTypeIdea, "", nil, nil, now.Add(5*time.Minute)); !errors.Is(err, ErrArchived) {
		t.Fatalf("got %v", err)
	}
	note.Restore(now.Add(6 * time.Minute))
	if note.Archived {
		t.Fatal("expected restored")
	}
	if err := note.SetPinned(true, now.Add(7*time.Minute)); err != nil || !note.Pinned {
		t.Fatalf("pin failed: %v", err)
	}
}

func TestInvalidSourceURL(t *testing.T) {
	now := time.Now().UTC()
	note, _ := NewNote("id", "title", "body", NoteTypeLink, now)
	note.Metadata.SourceURL = "file:///private/path"
	if !errors.Is(note.Validate(), ErrInvalidURL) {
		t.Fatal("expected invalid URL")
	}
}
