package reminders

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"example.com/local-capture/internal/domain"
)

type testClock struct{ now time.Time }

func (c testClock) Now() time.Time { return c.now }

type testGate struct{ unlocked atomic.Bool }

func (g *testGate) IsUnlocked() bool { return g.unlocked.Load() }

type testSource struct {
	mu        sync.Mutex
	reminders []domain.Reminder
	claims    map[string]bool
	listCalls atomic.Int32
	nextCalls atomic.Int32
}

func (s *testSource) ListDueReminders(_ context.Context, now time.Time) ([]domain.Reminder, error) {
	s.listCalls.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.Reminder
	for _, r := range s.reminders {
		if !r.IsCompleted() && !r.EffectiveDueAt().After(now) {
			out = append(out, r)
		}
	}
	return out, nil
}
func (s *testSource) NextReminderDue(context.Context) (time.Time, bool, error) {
	s.nextCalls.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	var next time.Time
	for _, r := range s.reminders {
		key := r.ID + "/" + r.EffectiveDueAt().UTC().Format(time.RFC3339Nano)
		if r.IsCompleted() || s.claims[key] {
			continue
		}
		if next.IsZero() || r.EffectiveDueAt().Before(next) {
			next = r.EffectiveDueAt()
		}
	}
	return next, !next.IsZero(), nil
}
func (s *testSource) ClaimNotification(_ context.Context, id string, due, _ time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := id + "/" + due.UTC().Format(time.RFC3339Nano)
	if s.claims[key] {
		return false, nil
	}
	s.claims[key] = true
	return true, nil
}

type testNotifier struct{ notices chan Notice }

func (n testNotifier) Notify(_ context.Context, notice Notice) error { n.notices <- notice; return nil }

func newTestSource(reminders ...domain.Reminder) *testSource {
	return &testSource{reminders: reminders, claims: map[string]bool{}}
}

func TestSchedulerClaimsAndSendsGenericNoticeOnce(t *testing.T) {
	now := time.Now().UTC()
	source := newTestSource(domain.Reminder{ID: "note-secret", NoteID: "note-secret", DueAt: now.Add(-time.Second)})
	gate := &testGate{}
	gate.unlocked.Store(true)
	notifier := testNotifier{notices: make(chan Notice, 2)}
	scheduler := NewScheduler(source, gate, testClock{now})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- scheduler.Run(ctx, notifier) }()
	select {
	case notice := <-notifier.notices:
		if notice.Title != "Reminder" || notice.Message != "A reminder is due." {
			t.Fatalf("notification included non-generic content: %+v", notice)
		}
	case <-time.After(time.Second):
		t.Fatal("due reminder was not notified")
	}
	time.Sleep(20 * time.Millisecond)
	if got := len(notifier.notices); got != 0 {
		t.Fatalf("duplicate notice count=%d", got)
	}
	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatalf("Run error=%v", err)
	}
}

func TestSchedulerWaitsForVaultUnlockAndExplicitWake(t *testing.T) {
	now := time.Now().UTC()
	source := newTestSource()
	gate := &testGate{}
	notifier := testNotifier{notices: make(chan Notice, 1)}
	scheduler := NewScheduler(source, gate, testClock{now})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- scheduler.Run(ctx, notifier) }()
	time.Sleep(20 * time.Millisecond)
	source.mu.Lock()
	source.reminders = append(source.reminders, domain.Reminder{ID: "id", NoteID: "id", DueAt: now.Add(-time.Second)})
	source.mu.Unlock()
	scheduler.Wake()
	time.Sleep(20 * time.Millisecond)
	if len(notifier.notices) != 0 {
		t.Fatal("locked vault received reminder notification")
	}
	gate.unlocked.Store(true)
	scheduler.Wake()
	select {
	case <-notifier.notices:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not resume after unlock")
	}
	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatalf("Run error=%v", err)
	}
}

func TestSchedulerUsesDeadlineTimerInsteadOfPolling(t *testing.T) {
	now := time.Now().UTC()
	source := newTestSource(domain.Reminder{ID: "future", NoteID: "future", DueAt: now.Add(time.Hour)})
	gate := &testGate{}
	gate.unlocked.Store(true)
	scheduler := NewScheduler(source, gate, testClock{now})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- scheduler.Run(ctx, testNotifier{notices: make(chan Notice, 1)}) }()
	time.Sleep(40 * time.Millisecond)
	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatalf("Run error=%v", err)
	}
	if source.listCalls.Load() != 1 || source.nextCalls.Load() != 1 {
		t.Fatalf("scheduler polled while waiting: list=%d next=%d", source.listCalls.Load(), source.nextCalls.Load())
	}
}
