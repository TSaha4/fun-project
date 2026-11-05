package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"example.com/local-capture/internal/domain"
)

type Service struct {
	notes       NoteRepository
	collections CollectionRepository
	tags        TagRepository
	searcher    Searcher
	ids         IDGenerator
	clock       Clock
}

func NewService(notes NoteRepository, collections CollectionRepository, tags TagRepository, searcher Searcher, ids IDGenerator, clock Clock) *Service {
	return &Service{notes: notes, collections: collections, tags: tags, searcher: searcher, ids: ids, clock: clock}
}

type CreateNoteInput struct {
	Title         string
	Content       string
	Type          domain.NoteType
	CollectionID  string
	TagIDs        []string
	Pinned        bool
	Reminder      *domain.Reminder
	SourceURL     string
	AttachmentIDs []string
}

func (s *Service) CreateNote(ctx context.Context, in CreateNoteInput) (domain.Note, error) {
	id, err := s.ids.NewID()
	if err != nil {
		return domain.Note{}, fmt.Errorf("generate note ID: %w", err)
	}
	note, err := domain.NewNote(id, in.Title, in.Content, in.Type, s.clock.Now())
	if err != nil {
		return domain.Note{}, err
	}
	note.Metadata.SourceURL = strings.TrimSpace(in.SourceURL)
	note.Reminder = cloneReminder(in.Reminder)
	note.AttachmentIDs = append([]string(nil), in.AttachmentIDs...)
	note.Pinned = in.Pinned
	if in.CollectionID != "" {
		if _, err := s.collections.GetCollection(ctx, in.CollectionID); err != nil {
			return domain.Note{}, relationshipError(err)
		}
		note.CollectionID = in.CollectionID
	}
	if err := s.assignTags(ctx, &note, in.TagIDs); err != nil {
		return domain.Note{}, err
	}
	if err := note.Validate(); err != nil {
		return domain.Note{}, err
	}
	if err := s.notes.CreateNote(ctx, note); err != nil {
		return domain.Note{}, err
	}
	return note, nil
}

func (s *Service) GetNote(ctx context.Context, id string) (domain.Note, error) {
	return s.notes.Get(ctx, id)
}

type UpdateNoteInput struct {
	Title         string
	Content       string
	Type          domain.NoteType
	CollectionID  *string
	TagIDs        *[]string
	Pinned        *bool
	Reminder      *domain.Reminder
	SourceURL     string
	AttachmentIDs []string
}

func (s *Service) UpdateNote(ctx context.Context, id string, in UpdateNoteInput) (domain.Note, error) {
	note, err := s.notes.Get(ctx, id)
	if err != nil {
		return domain.Note{}, err
	}
	reminder := in.Reminder
	if reminder == nil {
		reminder = note.Reminder
	}
	if err := note.Update(in.Title, in.Content, in.Type, strings.TrimSpace(in.SourceURL), reminder, in.AttachmentIDs, s.clock.Now()); err != nil {
		return domain.Note{}, err
	}
	if in.CollectionID != nil {
		if *in.CollectionID != "" {
			if _, err := s.collections.GetCollection(ctx, *in.CollectionID); err != nil {
				return domain.Note{}, relationshipError(err)
			}
		}
		if err := note.SetCollection(*in.CollectionID, s.clock.Now()); err != nil {
			return domain.Note{}, err
		}
	}
	if in.TagIDs != nil {
		if err := s.assignTags(ctx, &note, *in.TagIDs); err != nil {
			return domain.Note{}, err
		}
	}
	if in.Pinned != nil {
		if err := note.SetPinned(*in.Pinned, s.clock.Now()); err != nil {
			return domain.Note{}, err
		}
	}
	if err := s.notes.Update(ctx, note); err != nil {
		return domain.Note{}, err
	}
	return note, nil
}

func (s *Service) DeleteNote(ctx context.Context, id string) error { return s.notes.Delete(ctx, id) }

func (s *Service) ArchiveNote(ctx context.Context, id string) (domain.Note, error) {
	note, err := s.notes.Get(ctx, id)
	if err != nil {
		return domain.Note{}, err
	}
	note.Archive(s.clock.Now())
	if err := s.notes.Update(ctx, note); err != nil {
		return domain.Note{}, err
	}
	return note, nil
}

func (s *Service) RestoreNote(ctx context.Context, id string) (domain.Note, error) {
	note, err := s.notes.Get(ctx, id)
	if err != nil {
		return domain.Note{}, err
	}
	note.Restore(s.clock.Now())
	if err := s.notes.Update(ctx, note); err != nil {
		return domain.Note{}, err
	}
	return note, nil
}

func (s *Service) ListNotes(ctx context.Context, query ListNotesQuery) ([]domain.Note, error) {
	return s.notes.List(ctx, query)
}
func (s *Service) SearchNotes(ctx context.Context, query SearchQuery) ([]domain.Note, error) {
	return s.searcher.Search(ctx, query)
}

func (s *Service) CreateCollection(ctx context.Context, name string) (domain.Collection, error) {
	id, err := s.ids.NewID()
	if err != nil {
		return domain.Collection{}, fmt.Errorf("generate collection ID: %w", err)
	}
	collection, err := domain.NewCollection(id, name, s.clock.Now())
	if err != nil {
		return domain.Collection{}, err
	}
	if err := s.collections.CreateCollection(ctx, collection); err != nil {
		return domain.Collection{}, err
	}
	return collection, nil
}

func (s *Service) AssignTags(ctx context.Context, noteID string, tagIDs []string) (domain.Note, error) {
	note, err := s.notes.Get(ctx, noteID)
	if err != nil {
		return domain.Note{}, err
	}
	if err := s.assignTags(ctx, &note, tagIDs); err != nil {
		return domain.Note{}, err
	}
	if err := note.SetTags(note.TagIDs, s.clock.Now()); err != nil {
		return domain.Note{}, err
	}
	if err := s.notes.Update(ctx, note); err != nil {
		return domain.Note{}, err
	}
	return note, nil
}

func (s *Service) assignTags(ctx context.Context, note *domain.Note, tagIDs []string) error {
	ids := uniqueStrings(tagIDs)
	if len(ids) > 0 {
		found, err := s.tags.GetMany(ctx, ids)
		if err != nil {
			return err
		}
		if len(found) != len(ids) {
			return ErrInvalidRelationship
		}
	}
	return note.SetTags(ids, s.clock.Now())
}

func relationshipError(err error) error {
	if errors.Is(err, ErrNotFound) {
		return ErrInvalidRelationship
	}
	return err
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func cloneReminder(r *domain.Reminder) *domain.Reminder {
	if r == nil {
		return nil
	}
	c := *r
	return &c
}
