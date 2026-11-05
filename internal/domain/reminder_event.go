package domain

import "time"

type ReminderEventKind string

const (
	ReminderEventCreated             ReminderEventKind = "created"
	ReminderEventUpdated             ReminderEventKind = "updated"
	ReminderEventSnoozed             ReminderEventKind = "snoozed"
	ReminderEventCompleted           ReminderEventKind = "completed"
	ReminderEventNotificationClaimed ReminderEventKind = "notification_claimed"
)

// ReminderEvent stores lifecycle metadata only; it never stores note content.
type ReminderEvent struct {
	ID         string            `json:"id"`
	ReminderID string            `json:"reminder_id"`
	Kind       ReminderEventKind `json:"kind"`
	OccurredAt time.Time         `json:"occurred_at"`
	DueAt      time.Time         `json:"due_at"`
}
