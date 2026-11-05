# Dashboard workspace

The dashboard workspace adds Inbox, All notes, Collections, Tags, Pinned, Archived, Reminders, and Search results views. Notes render as reusable cards with type, title, content excerpt, collection, tag names, creation date, pinned marker, reminder marker, and attachment count. Card actions support edit, pin, archive/restore, duplicate, delete, move between collections, and add/remove tags. Delete asks for confirmation.

The note list requests 50 records at a time using the API's `limit` and `offset` filters. The user can load more pages; it does not render the full library into memory at once. Search also uses server-side full-text search and paging. Collections and tags use dedicated directory views. `N` opens capture, `/` focuses search, `J`/`K` moves focus between note cards, and Escape closes the editor. The interface uses responsive CSS and native controls without an added UI library.

## API boundary

The current list API cannot filter notes whose `collection_id` is empty. Inbox therefore shows 20 active notes across the workspace in API order (pinned notes first, then recently updated); it cannot yet guarantee that the notes are unfiled. Add a dedicated `unassigned=true` filter to the Go API/application/repository path before changing Inbox to strictly represent uncategorized notes. The dashboard intentionally does not fetch every note and filter locally because that would become inaccurate with pagination and inefficient for large libraries.

The documented API does not expose attachment metadata lookup by ID or a delete-note confirmation/undo capability. Cards can show attachment counts from note references, but existing attachment names/previews are not presented in this workspace card. The current backend's collection/tag/reminder capabilities may also return unavailable errors until real application and persistence wiring is complete.
