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

type reminderClock struct{ now time.Time }

func (c *reminderClock) Now() time.Time { return c.now }

type reminderIDs struct{ next int }

func (g *reminderIDs) NewID() (string, error) { g.next++; return fmt.Sprintf("event-%d", g.next), nil }

func TestReminderLifecycleDueSnoozeCompletionAndHistory(t *testing.T) {
	ctx := context.Background()
	store, _ := openTestStore(t)
	note := sampleNote("reminder-lifecycle")
	if err := store.CreateNote(ctx, note); err != nil {
		t.Fatal(err)
	}
	clock := &reminderClock{now: time.Date(2026, 8, 4, 10, 0, 0, 0, time.UTC)}
	service := app.NewReminderService(store, store, &reminderIDs{}, clock)
	r, err := service.CreateReminder(ctx, note.ID, clock.now.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != note.ID || r.NoteID != note.ID {
		t.Fatalf("stable note/reminder ID mismatch: %+v", r)
	}
	r, err = service.UpdateReminder(ctx, r.ID, clock.now.Add(-time.Minute))
	if err != nil || !r.DueAt.Equal(clock.now.Add(-time.Minute)) {
		t.Fatalf("reminder edit=%+v err=%v", r, err)
	}
	if _, err := service.CreateReminder(ctx, note.ID, clock.now.Add(time.Hour)); !errors.Is(err, app.ErrReminderExists) {
		t.Fatalf("duplicate create error=%v", err)
	}
	due, err := service.ListDueReminders(ctx)
	if err != nil || len(due) != 1 {
		t.Fatalf("due reminders=%v err=%v", due, err)
	}

	claimed, err := store.ClaimNotification(ctx, r.ID, r.EffectiveDueAt(), clock.now)
	if err != nil || !claimed {
		t.Fatalf("claim=%v err=%v", claimed, err)
	}
	claimed, err = store.ClaimNotification(ctx, r.ID, r.EffectiveDueAt(), clock.now.Add(time.Second))
	if err != nil || claimed {
		t.Fatalf("duplicate claim=%v err=%v", claimed, err)
	}

	until := clock.now.Add(2 * time.Hour)
	r, err = service.SnoozeReminder(ctx, r.ID, until)
	if err != nil {
		t.Fatal(err)
	}
	if !r.EffectiveDueAt().Equal(until) {
		t.Fatalf("effective snooze due=%s want=%s", r.EffectiveDueAt(), until)
	}
	due, err = service.ListDueReminders(ctx)
	if err != nil || len(due) != 0 {
		t.Fatalf("snoozed reminder due early: %v %v", due, err)
	}
	due, err = store.ListDueReminders(ctx, until)
	if err != nil || len(due) != 1 {
		t.Fatalf("snoozed reminder missing at deadline: %v %v", due, err)
	}

	loadedNote, err := store.Get(ctx, note.ID)
	if err != nil {
		t.Fatal(err)
	}
	loadedNote.Title = "Edited independently"
	loadedNote.UpdatedAt = clock.now.Add(time.Minute)
	if err := store.Update(ctx, loadedNote); err != nil {
		t.Fatal(err)
	}
	r, err = service.GetReminder(ctx, r.ID)
	if err != nil || r.SnoozedUntil == nil || !r.SnoozedUntil.Equal(until) {
		t.Fatalf("note edit reset snooze: %+v %v", r, err)
	}

	clock.now = until
	r, err = service.CompleteReminder(ctx, r.ID)
	if err != nil || !r.IsCompleted() {
		t.Fatalf("completion=%+v err=%v", r, err)
	}
	due, err = service.ListDueReminders(ctx)
	if err != nil || len(due) != 0 {
		t.Fatalf("completed reminder still due: %v %v", due, err)
	}
	active, err := service.ListReminders(ctx)
	if err != nil || len(active) != 0 {
		t.Fatalf("active reminders=%v err=%v", active, err)
	}
	all, err := service.ListAllReminders(ctx)
	if err != nil || len(all) != 1 || !all[0].IsCompleted() {
		t.Fatalf("reminder history list=%v err=%v", all, err)
	}
	history, err := service.ListReminderHistory(ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.ReminderEventKind{domain.ReminderEventCreated, domain.ReminderEventUpdated, domain.ReminderEventNotificationClaimed, domain.ReminderEventSnoozed, domain.ReminderEventCompleted}
	if len(history) != len(want) {
		t.Fatalf("history length=%d events=%+v", len(history), history)
	}
	for i := range want {
		if history[i].Kind != want[i] {
			t.Fatalf("history[%d]=%s want=%s", i, history[i].Kind, want[i])
		}
	}
	if _, err := service.SnoozeReminder(ctx, r.ID, clock.now.Add(time.Hour)); !errors.Is(err, domain.ErrReminderCompleted) {
		t.Fatalf("completed reminder snooze error=%v", err)
	}
	repeated, err := service.CompleteReminder(ctx, r.ID)
	if err != nil || len(mustHistory(t, service, ctx, r.ID)) != len(want) || !repeated.CompletedAt.Equal(*r.CompletedAt) {
		t.Fatalf("repeat completion changed history or timestamp: %+v %v", repeated, err)
	}
}

func mustHistory(t *testing.T, service *app.ReminderService, ctx context.Context, id string) []domain.ReminderEvent {
	t.Helper()
	history, err := service.ListReminderHistory(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return history
}

func TestNotificationClaimSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "restart.sqlite")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	note := sampleNote("restart-reminder")
	if err := store.CreateNote(ctx, note); err != nil {
		t.Fatal(err)
	}
	due := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	clock := &reminderClock{now: due.Add(time.Hour)}
	service := app.NewReminderService(store, store, &reminderIDs{}, clock)
	r, err := service.CreateReminder(ctx, note.ID, due)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimNotification(ctx, r.ID, due, clock.now)
	if err != nil || !claimed {
		t.Fatalf("initial claim=%v err=%v", claimed, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	claimed, err = reopened.ClaimNotification(ctx, r.ID, due, clock.now.Add(time.Minute))
	if err != nil || claimed {
		t.Fatalf("claim repeated after restart: %v %v", claimed, err)
	}
	if next, found, err := reopened.NextReminderDue(ctx); err != nil || found {
		t.Fatalf("claimed due time remained scheduled: %s %v %v", next, found, err)
	}
}

func TestConcurrentNotificationClaimsOnlyWinOnce(t *testing.T) {
	ctx := context.Background()
	store, _ := openTestStore(t)
	note := sampleNote("concurrent-claim")
	if err := store.CreateNote(ctx, note); err != nil {
		t.Fatal(err)
	}
	due := time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC)
	service := app.NewReminderService(store, store, &reminderIDs{}, &reminderClock{now: due.Add(time.Hour)})
	r, err := service.CreateReminder(ctx, note.ID, due)
	if err != nil {
		t.Fatal(err)
	}
	const workers = 16
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			claimed, err := store.ClaimNotification(ctx, r.ID, due, due.Add(time.Duration(i)*time.Second))
			if err != nil {
				errs <- err
				return
			}
			if claimed {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if wins != 1 {
		t.Fatalf("successful claim count=%d want=1", wins)
	}
}

func TestMigrationPreservesExistingReminderInstant(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "version-one.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	legacyDue := "2026-10-12T09:30:00.000000000Z"
	_, err = db.Exec(`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY NOT NULL,applied_at TEXT NOT NULL);
INSERT INTO schema_migrations VALUES(1,'2026-01-01T00:00:00Z');
CREATE TABLE notes(id TEXT PRIMARY KEY);
CREATE TABLE reminders(note_id TEXT PRIMARY KEY NOT NULL REFERENCES notes(id) ON DELETE CASCADE,due_at TEXT NOT NULL);
INSERT INTO notes VALUES('legacy-note');
INSERT INTO reminders VALUES('legacy-note','` + legacyDue + `');`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	r, err := store.GetReminder(ctx, "legacy-note")
	wantDue, _ := time.Parse(time.RFC3339Nano, legacyDue)
	if err != nil || r.ID != "legacy-note" || !r.DueAt.Equal(wantDue) {
		t.Fatalf("legacy reminder changed: %+v %v", r, err)
	}
	if r.SnoozedUntil != nil || r.CompletedAt != nil {
		t.Fatalf("legacy reminder got lifecycle state: %+v", r)
	}
	if events, err := store.ListReminderHistory(ctx, "legacy-note"); err != nil || len(events) != 0 {
		t.Fatalf("legacy history=%v err=%v", events, err)
	}
}
