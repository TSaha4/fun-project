package app

import (
	"context"
	"errors"
	"time"
	_ "time/tzdata"

	"example.com/local-capture/internal/domain"
)

type SnoozePreset string

const (
	SnoozeTwoHours   SnoozePreset = "two_hours"
	SnoozeTomorrow   SnoozePreset = "tomorrow"
	SnoozeNextMonday SnoozePreset = "next_monday"
	SnoozeCustom     SnoozePreset = "custom"
)

var ErrInvalidSnoozePreset = errors.New("invalid snooze preset")

// CalculateSnoozeUntil uses an absolute duration for two hours and local calendar
// arithmetic for day-based presets. The returned value is always a UTC instant.
func CalculateSnoozeUntil(now time.Time, zone *time.Location, preset SnoozePreset, custom time.Time) (time.Time, error) {
	if now.IsZero() {
		return time.Time{}, ErrInvalidSnoozePreset
	}
	if zone == nil {
		zone = now.Location()
	}
	localNow := now.In(zone)
	var until time.Time
	switch preset {
	case SnoozeTwoHours:
		until = now.Add(2 * time.Hour)
	case SnoozeTomorrow:
		until = localNow.AddDate(0, 0, 1)
	case SnoozeNextMonday:
		days := (int(time.Monday) - int(localNow.Weekday()) + 7) % 7
		if days == 0 {
			days = 7
		}
		until = localNow.AddDate(0, 0, days)
	case SnoozeCustom:
		if custom.IsZero() {
			return time.Time{}, ErrInvalidSnoozePreset
		}
		until = custom
	default:
		return time.Time{}, ErrInvalidSnoozePreset
	}
	if !until.After(now) {
		return time.Time{}, ErrInvalidSnoozePreset
	}
	return until.UTC(), nil
}

func (s *ReminderService) SnoozeWithPreset(ctx context.Context, id string, preset SnoozePreset, zone *time.Location, custom time.Time) (domain.Reminder, error) {
	until, err := CalculateSnoozeUntil(s.clock.Now(), zone, preset, custom)
	if err != nil {
		return domain.Reminder{}, domain.ErrInvalidSnooze
	}
	return s.SnoozeReminder(ctx, id, until)
}
