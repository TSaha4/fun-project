# Reminder lifecycle and scheduling

## Lifecycle and storage

Each note can have one reminder. Its stable ID is the note ID, preserving the existing one-to-one schema and HTTP route shape. Reminder operations live in `app.ReminderService`; editing note fields does not reset reminder snoozes, completion, or history.

Migration 2 is additive. It extends `reminders` with `snoozed_until`, `completed_at`, creation/update timestamps, and an indexed `effective_due_at`. Existing `due_at` values are preserved and copied to the new due/index columns. `reminder_events` stores created, updated, snoozed, completed, and notification-claim events. `reminder_notification_claims` has a unique `(reminder_id, scheduled_for)` key so a due instant is only claimed once, including after process restart.

All time instants are normalized to UTC for persistence. `two_hours` uses an elapsed two-hour duration. `tomorrow` and `next_monday` use the selected IANA time zone and local calendar arithmetic, preserving the local clock time across daylight-saving transitions where that local time exists. A custom time must be a future absolute instant. Existing reminders remain at their chosen instant if the computer's time zone later changes; changing the displayed zone does not move them.

## Scheduler and notifications

`internal/reminders.Scheduler` asks the repository for the next effective due instant, waits on one timer, then queries and claims due reminders. It does not periodically poll. Reminder create/update/snooze/complete callers should wire `ReminderService.SetScheduleWake(scheduler.Wake)`, and vault lock/unlock transitions should also call `Wake`. While locked, the scheduler waits for a wake signal without querying due reminder rows.

The Windows desktop adapter sends a generic tray notification: **Reminder — A reminder is due.** It does not fetch note content or titles. This uses the native tray information balloon through Walk, not the newer Windows App SDK toast API; Windows notification settings and shell behavior can affect whether it is displayed.

Notification claims are recorded before calling the operating-system notification API. This provides at-most-once delivery and prevents duplicates after restart. A process or shell failure after a claim but before display can cause one reminder notification to be missed. The OS tray notification API does not provide a transaction that can make database recording and display exactly-once.

## Security and current integration boundary

Reminder metadata, lifecycle events, and notification claims are currently stored in the existing SQLite database, which is not encrypted. Treat due times and note relationships as sensitive. The vault design is still awaiting approval, so the desktop composition root does not open the reminder scheduler or enable saving. The scheduler, persistence, application service, and notification adapter are available for integration after the approved vault gate is wired. No note text is placed in system notifications.

The HTTP API exposes reminder create/edit/list/due/history/snooze/complete use cases with existing reminder permissions. The current repository does not yet provide a complete production API composition root; do not expose real data through an unconfigured local server.
