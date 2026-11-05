package domain

import "errors"

var (
	ErrInvalidID         = errors.New("invalid ID")
	ErrInvalidTitle      = errors.New("title is required")
	ErrInvalidType       = errors.New("invalid note type")
	ErrInvalidURL        = errors.New("source URL must be an absolute HTTP or HTTPS URL")
	ErrInvalidName       = errors.New("name is required")
	ErrInvalidReminder   = errors.New("reminder time is required")
	ErrInvalidAttachment = errors.New("attachment metadata is invalid")
	ErrArchived          = errors.New("archived note cannot be changed; restore it first")
	ErrInvalidTimestamp  = errors.New("created/updated timestamp is invalid")
	ErrReminderCompleted = errors.New("reminder is already complete")
	ErrInvalidSnooze     = errors.New("snooze time must be in the future")
)
