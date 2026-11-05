package reminders

import "errors"

var ErrInvalidScheduler = errors.New("reminder scheduler requires source, vault gate, clock and notifier")
