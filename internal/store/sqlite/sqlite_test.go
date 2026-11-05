package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"example.com/local-capture/internal/app"
	"example.com/local-capture/internal/domain"
)

func openTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "vault.sqlite")
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	return store, path
}

func sampleNote(id string) domain.Note {
	now := time.Date(2026, 5, 6, 7, 8, 9, 123000000, time.UTC)
	note, err := domain.NewNote(id, "Title "+id, "body content", domain.NoteTypeThought, now)
	if err != nil {
		panic(err)
	}
	return note
}

func TestSQLiteNoteCRUDAndRelationships(t *testing.T) {
	ctx := context.Background()
	store, _ := openTestStore(t)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c, _ := domain.NewCollection("collection", "Research", now)
	if err := store.CreateCollection(ctx, c); err != nil {
		t.Fatal(err)
	}
	tag, _ := domain.NewTag("tag", "Reading", now)
	if err := store.CreateTag(ctx, tag); err != nil {
		t.Fatal(err)
	}
	attachment, _ := domain.NewAttachment("attachment", "capture.png", "image/png", 12, "checksum", now)
	if err := store.CreateAttachment(ctx, attachment); err != nil {
		t.Fatal(err)
	}
	note := sampleNote("note-1")
	note.CollectionID = c.ID
	note.TagIDs = []string{tag.ID}
	note.AttachmentIDs = []string{attachment.ID}
	note.Reminder = &domain.Reminder{DueAt: now.Add(time.Hour)}
	note.Metadata.SourceURL = "https://example.test/a?x=1&y=2"
	note.Pinned = true
	if err := store.CreateNote(ctx, note); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ctx, note.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CollectionID != c.ID || len(got.TagIDs) != 1 || got.TagIDs[0] != tag.ID || len(got.AttachmentIDs) != 1 || got.Reminder == nil || !got.Pinned || got.Metadata.SourceURL != note.Metadata.SourceURL {
		t.Fatalf("relationships not loaded: %+v", got)
	}
	got.Title = "updated"
	got.Content = "new body"
	got.TagIDs = nil
	got.AttachmentIDs = nil
	got.Reminder = nil
	got.UpdatedAt = got.UpdatedAt.Add(time.Minute)
	if err := store.Update(ctx, got); err != nil {
		t.Fatal(err)
	}
	got, err = store.Get(ctx, note.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "updated" || len(got.TagIDs) != 0 || len(got.AttachmentIDs) != 0 || got.Reminder == nil {
		t.Fatalf("update did not replace relations: %+v", got)
	}
	if err := store.Delete(ctx, note.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, note.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestSQLiteSearchAndFilters(t *testing.T) {
	ctx := context.Background()
	store, _ := openTestStore(t)
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	c, _ := domain.NewCollection("c1", "Collection", now)
	_ = store.CreateCollection(ctx, c)
	tag, _ := domain.NewTag("t1", "nebula", now)
	_ = store.CreateTag(ctx, tag)
	n1 := sampleNote("search-1")
	n1.CreatedAt = now
	n1.UpdatedAt = now
	n1.Title = "Special [Title]"
	n1.Content = "galaxy phrase"
	n1.Type = domain.NoteTypeIdea
	n1.CollectionID = c.ID
	n1.TagIDs = []string{tag.ID}
	n1.Pinned = true
	n2 := sampleNote("search-2")
	n2.Title = "Other"
	n2.Content = "unrelated"
	n2.Type = domain.NoteTypeTask
	n2.CreatedAt = now.Add(24 * time.Hour)
	n2.UpdatedAt = n2.CreatedAt
	if err := store.CreateNote(ctx, n1); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateNote(ctx, n2); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		term string
		want string
	}{{"galaxy", "search-1"}, {"nebula", "search-1"}, {`[Title]`, "search-1"}} {
		found, err := store.Search(ctx, app.SearchQuery{Text: tc.term})
		if err != nil {
			t.Fatalf("search %q: %v", tc.term, err)
		}
		if len(found) != 1 || found[0].ID != tc.want {
			t.Fatalf("search %q got %+v", tc.term, found)
		}
	}
	pinned := true
	kind := domain.NoteTypeIdea
	after := now.Add(12 * time.Hour)
	found, err := store.Search(ctx, app.SearchQuery{CollectionID: c.ID, Type: kind, Pinned: &pinned, CreatedAfter: &after})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Fatalf("date filter returned old note: %+v", found)
	}
	after = now.Add(-time.Hour)
	found, err = store.Search(ctx, app.SearchQuery{CollectionID: c.ID, Type: kind, Pinned: &pinned, CreatedAfter: &after})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].ID != n1.ID {
		t.Fatalf("filtered search: %+v", found)
	}
	n1.Archive(now.Add(2 * time.Hour))
	if err := store.Update(ctx, n1); err != nil {
		t.Fatal(err)
	}
	found, err = store.Search(ctx, app.SearchQuery{Text: "galaxy"})
	if err != nil || len(found) != 0 {
		t.Fatalf("default search should omit archived: %+v, %v", found, err)
	}
	archived := true
	found, err = store.Search(ctx, app.SearchQuery{Text: "galaxy", Archived: &archived})
	if err != nil || len(found) != 1 {
		t.Fatalf("archived filter: %+v, %v", found, err)
	}
}

func TestSQLiteInboxOnlyListsUncollectedActiveNotes(t *testing.T) {
	ctx := context.Background()
	store, _ := openTestStore(t)
	now := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	collection, err := domain.NewCollection("work", "Work", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateCollection(ctx, collection); err != nil {
		t.Fatal(err)
	}
	inbox := sampleNote("inbox")
	inbox.CreatedAt = now
	inbox.UpdatedAt = now
	collected := sampleNote("collected")
	collected.CreatedAt = now
	collected.UpdatedAt = now
	collected.CollectionID = collection.ID
	archived := sampleNote("archived")
	archived.CreatedAt = now
	archived.UpdatedAt = now
	archived.Archived = true
	for _, note := range []domain.Note{inbox, collected, archived} {
		if err := store.CreateNote(ctx, note); err != nil {
			t.Fatal(err)
		}
	}
	active := false
	got, err := store.List(ctx, app.ListNotesQuery{InboxOnly: true, Archived: &active, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "inbox" {
		t.Fatalf("expected only the active uncollected note, got %+v", got)
	}
}

func TestSQLiteTransactionsDuplicatesEmptyAndSpecialCharacters(t *testing.T) {
	ctx := context.Background()
	store, _ := openTestStore(t)
	now := time.Now().UTC()
	tag, _ := domain.NewTag("valid-tag", "valid", now)
	if err := store.CreateTag(ctx, tag); err != nil {
		t.Fatal(err)
	}
	note := sampleNote("transactional")
	note.TagIDs = []string{"missing-tag"}
	if err := store.CreateNote(ctx, note); err == nil {
		t.Fatal("expected foreign-key failure")
	}
	if _, err := store.Get(ctx, note.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("failed transaction left note behind: %v", err)
	}
	note = sampleNote("duplicate")
	note.Title = "Quotes ' OR 1=1; -- and café"
	note.Content = ""
	if err := store.CreateNote(ctx, note); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateNote(ctx, note); err == nil {
		t.Fatal("expected duplicate ID rejection")
	}
	if _, err := domain.NewNote("empty-title", "  ", "", domain.NoteTypeThought, now); !errors.Is(err, domain.ErrInvalidTitle) {
		t.Fatalf("expected blank title rejection, got %v", err)
	}
	found, err := store.Search(ctx, app.SearchQuery{Text: "café"})
	if err != nil || len(found) != 1 || found[0].ID != note.ID {
		t.Fatalf("special character search: %+v, %v", found, err)
	}
	collection, _ := domain.NewCollection("duplicate-name", "Dupe", now)
	if err := store.CreateCollection(ctx, collection); err != nil {
		t.Fatal(err)
	}
	other, _ := domain.NewCollection("duplicate-name-2", "dupe", now)
	if err := store.CreateCollection(ctx, other); err == nil {
		t.Fatal("expected duplicate collection name rejection")
	}
	if _, err := store.Get(ctx, "absent"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("empty lookup: %v", err)
	}
}

func TestUpdateTransactionRollsBackRelationshipFailure(t *testing.T) {
	ctx := context.Background()
	store, _ := openTestStore(t)
	note := sampleNote("atomic-update")
	if err := store.CreateNote(ctx, note); err != nil {
		t.Fatal(err)
	}
	note.Title = "should roll back"
	note.TagIDs = []string{"nonexistent"}
	note.UpdatedAt = note.UpdatedAt.Add(time.Minute)
	if err := store.Update(ctx, note); err == nil {
		t.Fatal("expected invalid tag foreign-key failure")
	}
	got, err := store.Get(ctx, note.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Title atomic-update" || got.UpdatedAt != sampleNote("atomic-update").UpdatedAt {
		t.Fatalf("partial update escaped transaction: %+v", got)
	}
}

func TestMigrationsAreVersionedIdempotentAndPreserveRows(t *testing.T) {
	ctx := context.Background()
	store, path := openTestStore(t)
	note := sampleNote("kept")
	if err := store.CreateNote(ctx, note); err != nil {
		t.Fatal(err)
	}
	var version int
	if err := store.db.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil || version != 2 {
		t.Fatalf("migration version=%d err=%v", version, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.Get(ctx, note.ID)
	if err != nil || got.Title != note.Title {
		t.Fatalf("reopen lost data: %+v %v", got, err)
	}
	if err := reopened.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&version); err != nil || version != 2 {
		t.Fatalf("migration reran: count=%d err=%v", version, err)
	}
}

func TestConcurrentWrites(t *testing.T) {
	ctx := context.Background()
	store, _ := openTestStore(t)
	const writers = 20
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			n := sampleNote(fmt.Sprintf("parallel-%d", i))
			errs <- store.CreateNote(ctx, n)
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	items, err := store.List(ctx, app.ListNotesQuery{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != writers {
		t.Fatalf("got %d rows, want %d", len(items), writers)
	}
}

func TestNewerUnknownSchemaVersionIsRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "future.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY, applied_at TEXT); INSERT INTO schema_migrations VALUES(999,'future');`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	if _, err := Open(context.Background(), path); err == nil {
		t.Fatal("expected newer schema version rejection")
	}
}
