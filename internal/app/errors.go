package app

import "errors"

var (
	ErrNotFound            = errors.New("not found")
	ErrInvalidRelationship = errors.New("relationship target does not exist")
	ErrReminderExists      = errors.New("note already has a reminder")
)
