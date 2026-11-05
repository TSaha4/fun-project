package app

import (
	"testing"
	"time"
)

func TestSnoozePresetsUseLocalCalendarAndStableInstants(t *testing.T) {
	zone, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("time-zone database unavailable: %v", err)
	}
	now := time.Date(2026, time.March, 7, 9, 30, 0, 0, zone)
	tomorrow, err := CalculateSnoozeUntil(now, zone, SnoozeTomorrow, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	localTomorrow := tomorrow.In(zone)
	if localTomorrow.Year() != 2026 || localTomorrow.Month() != time.March || localTomorrow.Day() != 8 || localTomorrow.Hour() != 9 || localTomorrow.Minute() != 30 {
		t.Fatalf("tomorrow did not preserve wall-clock time across DST: %s", localTomorrow)
	}
	if got := tomorrow.Sub(now); got != 23*time.Hour {
		t.Fatalf("DST transition duration=%s want 23h", got)
	}
	if _, offset := tomorrow.Zone(); offset != 0 {
		t.Fatalf("stored time not normalized to UTC: %s", tomorrow)
	}

	monday := time.Date(2026, time.July, 6, 10, 0, 0, 0, zone)
	nextMonday, err := CalculateSnoozeUntil(monday, zone, SnoozeNextMonday, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if got := nextMonday.In(zone); got.Weekday() != time.Monday || got.Day() != 13 || got.Hour() != 10 {
		t.Fatalf("next Monday = %s", got)
	}
	twoHours, err := CalculateSnoozeUntil(now, zone, SnoozeTwoHours, time.Time{})
	if err != nil || twoHours.Sub(now) != 2*time.Hour {
		t.Fatalf("two-hour preset: %s %v", twoHours, err)
	}
}

func TestCustomSnoozeKeepsChosenAbsoluteTimeAcrossZoneChanges(t *testing.T) {
	now := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Skipf("time-zone database unavailable: %v", err)
	}
	custom := time.Date(2026, time.January, 2, 9, 15, 0, 0, tokyo)
	want := custom.UTC()
	got, err := CalculateSnoozeUntil(now, time.UTC, SnoozeCustom, custom)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(want) {
		t.Fatalf("custom instant %s want %s", got, want)
	}
	if !got.In(time.FixedZone("changed-local-zone", -7*3600)).Equal(want) {
		t.Fatal("changing display timezone shifted the persisted instant")
	}
}
