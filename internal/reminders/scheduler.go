package reminders

import (
	"context"
	"time"

	"example.com/local-capture/internal/domain"
)

// Source is the scheduling-only view of the reminder application/storage layer.
// ClaimNotification must durably suppress a second attempt for the same due instant.
type Source interface {
	ListDueReminders(context.Context, time.Time) ([]domain.Reminder, error)
	NextReminderDue(context.Context) (time.Time, bool, error)
	ClaimNotification(context.Context, string, time.Time, time.Time) (bool, error)
}

type VaultGate interface {
	IsUnlocked() bool
}

type Notice struct {
	ReminderID string
	Title      string
	Message    string
}

type Notifier interface {
	Notify(context.Context, Notice) error
}

type Clock interface{ Now() time.Time }

type Scheduler struct {
	source      Source
	vault       VaultGate
	clock       Clock
	retryPeriod time.Duration
	wake        chan struct{}
}

func NewScheduler(source Source, vault VaultGate, clock Clock) *Scheduler {
	return &Scheduler{source: source, vault: vault, clock: clock, retryPeriod: time.Minute, wake: make(chan struct{}, 1)}
}

// Wake asks the scheduler to reload due times after a reminder mutation or vault
// state transition. Calls are coalesced so bursts do not queue redundant work.
func (s *Scheduler) Wake() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Scheduler) Run(ctx context.Context, notifier Notifier) error {
	if s.source == nil || s.vault == nil || s.clock == nil || notifier == nil {
		return ErrInvalidScheduler
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !s.vault.IsUnlocked() {
			if err := waitForWake(ctx, s.wake); err != nil {
				return err
			}
			continue
		}
		now := s.clock.Now()
		due, err := s.source.ListDueReminders(ctx, now)
		if err != nil {
			if err := waitForWakeOrTimer(ctx, s.wake, s.retryPeriod); err != nil {
				return err
			}
			continue
		}
		claimFailed := false
		for _, reminder := range due {
			if err := ctx.Err(); err != nil {
				return err
			}
			if !s.vault.IsUnlocked() {
				break
			}
			dueAt := reminder.EffectiveDueAt()
			claimed, err := s.source.ClaimNotification(ctx, reminder.ID, dueAt, s.clock.Now())
			if err != nil {
				claimFailed = true
				continue
			}
			if !claimed {
				continue
			}
			// Notification content is intentionally generic. Note text never crosses
			// this adapter unless a future explicit user preference adds it.
			_ = notifier.Notify(ctx, Notice{ReminderID: reminder.ID, Title: "Reminder", Message: "A reminder is due."})
		}
		if claimFailed {
			if err := waitForWakeOrTimer(ctx, s.wake, s.retryPeriod); err != nil {
				return err
			}
			continue
		}
		if !s.vault.IsUnlocked() {
			continue
		}
		next, found, err := s.source.NextReminderDue(ctx)
		if err != nil {
			if err := waitForWakeOrTimer(ctx, s.wake, s.retryPeriod); err != nil {
				return err
			}
			continue
		}
		if !found {
			if err := waitForWake(ctx, s.wake); err != nil {
				return err
			}
			continue
		}
		delay := next.Sub(s.clock.Now())
		if delay < 0 {
			delay = 0
		}
		if err := waitForWakeOrTimer(ctx, s.wake, delay); err != nil {
			return err
		}
	}
}

func waitForWake(ctx context.Context, wake <-chan struct{}) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-wake:
		return nil
	}
}

func waitForWakeOrTimer(ctx context.Context, wake <-chan struct{}, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-wake:
		return nil
	case <-timer.C:
		return nil
	}
}

func wait(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
