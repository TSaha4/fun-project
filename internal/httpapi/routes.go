package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"example.com/local-capture/internal/app"
	"example.com/local-capture/internal/domain"
)

const apiPrefix = "/api/v1"

type createNoteRequest struct {
	Title         string           `json:"title"`
	Content       string           `json:"content"`
	Type          domain.NoteType  `json:"type"`
	CollectionID  string           `json:"collection_id"`
	TagIDs        []string         `json:"tag_ids"`
	Pinned        bool             `json:"pinned"`
	Reminder      *domain.Reminder `json:"reminder"`
	SourceURL     string           `json:"source_url"`
	AttachmentIDs []string         `json:"attachment_ids"`
}

type updateNoteRequest struct {
	Title         string           `json:"title"`
	Content       string           `json:"content"`
	Type          domain.NoteType  `json:"type"`
	CollectionID  *string          `json:"collection_id"`
	TagIDs        *[]string        `json:"tag_ids"`
	Pinned        *bool            `json:"pinned"`
	Reminder      *domain.Reminder `json:"reminder"`
	SourceURL     string           `json:"source_url"`
	AttachmentIDs []string         `json:"attachment_ids"`
}

type searchRequest struct {
	Text          string          `json:"text"`
	CollectionID  string          `json:"collection_id"`
	TagIDs        []string        `json:"tag_ids"`
	Type          domain.NoteType `json:"type"`
	Pinned        *bool           `json:"pinned"`
	Archived      *bool           `json:"archived"`
	CreatedAfter  *time.Time      `json:"created_after"`
	CreatedBefore *time.Time      `json:"created_before"`
	Limit         int             `json:"limit"`
	Offset        int             `json:"offset"`
}

// Application is implemented by the application-service layer. HTTP handlers
// depend on use cases only and have no persistence or SQL dependency.
type Application interface {
	CreateNote(context.Context, app.CreateNoteInput) (domain.Note, error)
	GetNote(context.Context, string) (domain.Note, error)
	UpdateNote(context.Context, string, app.UpdateNoteInput) (domain.Note, error)
	DeleteNote(context.Context, string) error
	ArchiveNote(context.Context, string) (domain.Note, error)
	RestoreNote(context.Context, string) (domain.Note, error)
	ListNotes(context.Context, app.ListNotesQuery) ([]domain.Note, error)
	SearchNotes(context.Context, app.SearchQuery) ([]domain.Note, error)
	CreateCollection(context.Context, string) (domain.Collection, error)
	ListCollections(context.Context) ([]domain.Collection, error)
	UpdateCollection(context.Context, string, string) (domain.Collection, error)
	DeleteCollection(context.Context, string) error
	CreateTag(context.Context, string) (domain.Tag, error)
	ListTags(context.Context) ([]domain.Tag, error)
	AssignTags(context.Context, string, []string) (domain.Note, error)
	RemoveTag(context.Context, string, string) (domain.Note, error)
	UploadAttachment(context.Context, string, string, string, io.Reader) (domain.Attachment, error)
	DownloadAttachment(context.Context, string) (domain.Attachment, io.ReadCloser, error)
	DeleteAttachment(context.Context, string) error
	CreateReminder(context.Context, string, time.Time) (domain.Reminder, error)
	UpdateReminder(context.Context, string, time.Time) (domain.Reminder, error)
	ListReminders(context.Context) ([]domain.Reminder, error)
	ListDueReminders(context.Context) ([]domain.Reminder, error)
	ListReminderHistory(context.Context, string) ([]domain.ReminderEvent, error)
	SnoozeReminder(context.Context, string, time.Time) (domain.Reminder, error)
	CompleteReminder(context.Context, string) (domain.Reminder, error)
}

func (s *Server) routes(a Application) http.Handler {
	mux := http.NewServeMux()
	register := func(pattern string, p Permission, h http.HandlerFunc) { mux.Handle(pattern, s.require(p, h)) }
	register("POST "+apiPrefix+"/notes", PermissionNotesWrite, func(w http.ResponseWriter, r *http.Request) {
		var input createNoteRequest
		if !decodeJSON(w, r, &input) {
			return
		}
		if !validNoteType(input.Type) {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_type", "Note type is invalid")
			return
		}
		if strings.TrimSpace(input.Title) == "" || len(input.Title) > 500 || len(input.Content) > 1<<20 || len(input.TagIDs) > 100 || len(input.AttachmentIDs) > 100 {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_request", "Note fields exceed allowed limits or are missing")
			return
		}
		in := app.CreateNoteInput{Title: input.Title, Content: input.Content, Type: input.Type, CollectionID: input.CollectionID, TagIDs: input.TagIDs, Pinned: input.Pinned, Reminder: input.Reminder, SourceURL: input.SourceURL, AttachmentIDs: input.AttachmentIDs}
		n, err := a.CreateNote(r.Context(), in)
		respond(w, r, n, err, http.StatusCreated)
	})
	register("GET "+apiPrefix+"/notes/{id}", PermissionNotesRead, func(w http.ResponseWriter, r *http.Request) {
		n, err := a.GetNote(r.Context(), resourceID(r, "id"))
		respond(w, r, n, err, http.StatusOK)
	})
	register("PUT "+apiPrefix+"/notes/{id}", PermissionNotesWrite, func(w http.ResponseWriter, r *http.Request) {
		var input updateNoteRequest
		if !decodeJSON(w, r, &input) {
			return
		}
		if !validNoteType(input.Type) {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_type", "Note type is invalid")
			return
		}
		if strings.TrimSpace(input.Title) == "" || len(input.Title) > 500 || len(input.Content) > 1<<20 || (input.TagIDs != nil && len(*input.TagIDs) > 100) || len(input.AttachmentIDs) > 100 {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_request", "Note fields exceed allowed limits or are missing")
			return
		}
		in := app.UpdateNoteInput{Title: input.Title, Content: input.Content, Type: input.Type, CollectionID: input.CollectionID, TagIDs: input.TagIDs, Pinned: input.Pinned, Reminder: input.Reminder, SourceURL: input.SourceURL, AttachmentIDs: input.AttachmentIDs}
		n, err := a.UpdateNote(r.Context(), resourceID(r, "id"), in)
		respond(w, r, n, err, http.StatusOK)
	})
	register("POST "+apiPrefix+"/notes/{id}/archive", PermissionNotesArchive, func(w http.ResponseWriter, r *http.Request) {
		n, err := a.ArchiveNote(r.Context(), resourceID(r, "id"))
		respond(w, r, n, err, http.StatusOK)
	})
	register("POST "+apiPrefix+"/notes/{id}/restore", PermissionNotesArchive, func(w http.ResponseWriter, r *http.Request) {
		n, err := a.RestoreNote(r.Context(), resourceID(r, "id"))
		respond(w, r, n, err, http.StatusOK)
	})
	register("DELETE "+apiPrefix+"/notes/{id}", PermissionNotesDelete, func(w http.ResponseWriter, r *http.Request) {
		err := a.DeleteNote(r.Context(), resourceID(r, "id"))
		respondDeleted(w, r, err)
	})
	register("GET "+apiPrefix+"/notes", PermissionNotesRead, func(w http.ResponseWriter, r *http.Request) {
		q, err := noteListQuery(r)
		if err != nil {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_query", "Query parameters are invalid")
			return
		}
		items, err := a.ListNotes(r.Context(), q)
		respond(w, r, items, err, http.StatusOK)
	})
	register("POST "+apiPrefix+"/notes/search", PermissionNotesRead, func(w http.ResponseWriter, r *http.Request) {
		var input searchRequest
		if !decodeJSON(w, r, &input) {
			return
		}
		if len(input.Text) > 4096 {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_query", "Search query is too long")
			return
		}
		q := app.SearchQuery{Text: input.Text, CollectionID: input.CollectionID, TagIDs: input.TagIDs, Type: input.Type, Pinned: input.Pinned, Archived: input.Archived, CreatedAfter: input.CreatedAfter, CreatedBefore: input.CreatedBefore, Limit: input.Limit, Offset: input.Offset}
		if q.Type != "" && !q.Type.Valid() {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_type", "Note type is invalid")
			return
		}
		items, err := a.SearchNotes(r.Context(), q)
		respond(w, r, items, err, http.StatusOK)
	})

	register("POST "+apiPrefix+"/collections", PermissionCollectionsWrite, func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Name string `json:"name"`
		}
		if !decodeJSON(w, r, &in) {
			return
		}
		if strings.TrimSpace(in.Name) == "" || len(in.Name) > 200 {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_name", "Collection name is invalid")
			return
		}
		v, err := a.CreateCollection(r.Context(), in.Name)
		respond(w, r, v, err, http.StatusCreated)
	})
	register("GET "+apiPrefix+"/collections", PermissionCollectionsRead, func(w http.ResponseWriter, r *http.Request) {
		v, err := a.ListCollections(r.Context())
		respond(w, r, v, err, http.StatusOK)
	})
	register("PUT "+apiPrefix+"/collections/{id}", PermissionCollectionsWrite, func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Name string `json:"name"`
		}
		if !decodeJSON(w, r, &in) {
			return
		}
		if strings.TrimSpace(in.Name) == "" || len(in.Name) > 200 {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_name", "Collection name is invalid")
			return
		}
		v, err := a.UpdateCollection(r.Context(), resourceID(r, "id"), in.Name)
		respond(w, r, v, err, http.StatusOK)
	})
	register("DELETE "+apiPrefix+"/collections/{id}", PermissionCollectionsDelete, func(w http.ResponseWriter, r *http.Request) {
		respondDeleted(w, r, a.DeleteCollection(r.Context(), resourceID(r, "id")))
	})

	register("POST "+apiPrefix+"/tags", PermissionTagsWrite, func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Name string `json:"name"`
		}
		if !decodeJSON(w, r, &in) {
			return
		}
		if strings.TrimSpace(in.Name) == "" || len(in.Name) > 100 {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_name", "Tag name is invalid")
			return
		}
		v, err := a.CreateTag(r.Context(), in.Name)
		respond(w, r, v, err, http.StatusCreated)
	})
	register("GET "+apiPrefix+"/tags", PermissionTagsRead, func(w http.ResponseWriter, r *http.Request) {
		v, err := a.ListTags(r.Context())
		respond(w, r, v, err, http.StatusOK)
	})
	register("POST "+apiPrefix+"/tags/assign", PermissionTagsWrite, func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			NoteID string   `json:"note_id"`
			TagIDs []string `json:"tag_ids"`
		}
		if !decodeJSON(w, r, &in) {
			return
		}
		if in.NoteID == "" || len(in.TagIDs) > 100 {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_request", "Tag assignment is invalid")
			return
		}
		n, err := a.AssignTags(r.Context(), in.NoteID, in.TagIDs)
		respond(w, r, n, err, http.StatusOK)
	})
	register("DELETE "+apiPrefix+"/notes/{id}/tags/{tag_id}", PermissionTagsWrite, func(w http.ResponseWriter, r *http.Request) {
		n, err := a.RemoveTag(r.Context(), resourceID(r, "id"), resourceID(r, "tag_id"))
		respond(w, r, n, err, http.StatusOK)
	})

	register("POST "+apiPrefix+"/attachments", PermissionAttachmentsWrite, func(w http.ResponseWriter, r *http.Request) {
		name := r.Header.Get("X-Filename")
		if name == "" || len(name) > 255 || strings.ContainsAny(name, "/\\\x00") {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_filename", "Attachment filename is invalid")
			return
		}
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType == "" {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_media_type", "Attachment media type is invalid")
			return
		}
		v, err := a.UploadAttachment(r.Context(), r.URL.Query().Get("note_id"), name, mediaType, r.Body)
		respond(w, r, v, err, http.StatusCreated)
	})
	register("GET "+apiPrefix+"/attachments/{id}", PermissionAttachmentsRead, func(w http.ResponseWriter, r *http.Request) {
		v, body, err := a.DownloadAttachment(r.Context(), resourceID(r, "id"))
		if err != nil {
			writeMappedError(w, r, err)
			return
		}
		defer body.Close()
		w.Header().Set("Content-Type", v.MediaType)
		w.Header().Set("Content-Disposition", `attachment; filename="`+safeFilename(v.FileName)+`"`)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Length", strconv.FormatInt(v.Size, 10))
		_, _ = io.Copy(w, body)
	})
	register("DELETE "+apiPrefix+"/attachments/{id}", PermissionAttachmentsDelete, func(w http.ResponseWriter, r *http.Request) {
		respondDeleted(w, r, a.DeleteAttachment(r.Context(), resourceID(r, "id")))
	})

	register("POST "+apiPrefix+"/reminders", PermissionRemindersWrite, func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			NoteID string    `json:"note_id"`
			DueAt  time.Time `json:"due_at"`
		}
		if !decodeJSON(w, r, &in) {
			return
		}
		if in.NoteID == "" || in.DueAt.IsZero() {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_reminder", "Reminder request is invalid")
			return
		}
		v, err := a.CreateReminder(r.Context(), in.NoteID, in.DueAt)
		respond(w, r, v, err, http.StatusCreated)
	})
	register("PUT "+apiPrefix+"/reminders/{id}", PermissionRemindersWrite, func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			DueAt time.Time `json:"due_at"`
		}
		if !decodeJSON(w, r, &in) {
			return
		}
		if in.DueAt.IsZero() {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_reminder", "Reminder request is invalid")
			return
		}
		v, err := a.UpdateReminder(r.Context(), resourceID(r, "id"), in.DueAt)
		respond(w, r, v, err, http.StatusOK)
	})
	register("GET "+apiPrefix+"/reminders", PermissionRemindersRead, func(w http.ResponseWriter, r *http.Request) {
		v, err := a.ListReminders(r.Context())
		respond(w, r, v, err, http.StatusOK)
	})
	register("GET "+apiPrefix+"/reminders/due", PermissionRemindersRead, func(w http.ResponseWriter, r *http.Request) {
		v, err := a.ListDueReminders(r.Context())
		respond(w, r, v, err, http.StatusOK)
	})
	register("GET "+apiPrefix+"/reminders/{id}/history", PermissionRemindersRead, func(w http.ResponseWriter, r *http.Request) {
		v, err := a.ListReminderHistory(r.Context(), resourceID(r, "id"))
		respond(w, r, v, err, http.StatusOK)
	})
	register("POST "+apiPrefix+"/reminders/{id}/snooze", PermissionRemindersWrite, func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Until    time.Time        `json:"until"`
			Preset   app.SnoozePreset `json:"preset"`
			Timezone string           `json:"timezone"`
			CustomAt time.Time        `json:"custom_at"`
		}
		if !decodeJSON(w, r, &in) {
			return
		}
		until := in.Until
		if in.Preset != "" {
			zone := time.Local
			if in.Timezone != "" {
				var err error
				zone, err = time.LoadLocation(in.Timezone)
				if err != nil {
					writeAPIError(w, r, http.StatusBadRequest, "invalid_reminder", "Reminder time zone is invalid")
					return
				}
			}
			var err error
			until, err = app.CalculateSnoozeUntil(time.Now(), zone, in.Preset, in.CustomAt)
			if err != nil {
				writeAPIError(w, r, http.StatusBadRequest, "invalid_reminder", "Snooze preset or time is invalid")
				return
			}
		}
		if until.IsZero() {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_reminder", "Snooze time is invalid")
			return
		}
		v, err := a.SnoozeReminder(r.Context(), resourceID(r, "id"), until)
		respond(w, r, v, err, http.StatusOK)
	})
	register("POST "+apiPrefix+"/reminders/{id}/complete", PermissionRemindersWrite, func(w http.ResponseWriter, r *http.Request) {
		v, err := a.CompleteReminder(r.Context(), resourceID(r, "id"))
		respond(w, r, v, err, http.StatusOK)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeAPIError(w, r, http.StatusNotFound, "not_found", "Route does not exist")
	})
	return mux
}

func (s *Server) require(p Permission, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.config.Permissions[p] {
			writeAPIError(w, r, http.StatusForbidden, "permission_denied", "The token lacks permission for this operation")
			return
		}
		next(w, r)
	}
}

func noteListQuery(r *http.Request) (app.ListNotesQuery, error) {
	v := r.URL.Query()
	q := app.ListNotesQuery{CollectionID: v.Get("collection_id"), TagID: v.Get("tag_id"), Type: domain.NoteType(v.Get("type")), Limit: 100}
	if v.Get("type") != "" && !q.Type.Valid() {
		return q, fmt.Errorf("invalid type")
	}
	if s := v.Get("limit"); s != "" {
		n, e := strconv.Atoi(s)
		if e != nil || n < 1 || n > 500 {
			return q, fmt.Errorf("invalid limit")
		}
		q.Limit = n
	}
	if s := v.Get("offset"); s != "" {
		n, e := strconv.Atoi(s)
		if e != nil || n < 0 {
			return q, fmt.Errorf("invalid offset")
		}
		q.Offset = n
	}
	var err error
	q.Pinned, err = parseOptionalBool(v.Get("pinned"))
	if err != nil {
		return q, err
	}
	q.Archived, err = parseOptionalBool(v.Get("archived"))
	if err != nil {
		return q, err
	}
	q.CreatedAfter, err = parseOptionalTime(v.Get("created_after"))
	if err != nil {
		return q, err
	}
	q.CreatedBefore, err = parseOptionalTime(v.Get("created_before"))
	return q, err
}
func parseOptionalBool(v string) (*bool, error) {
	if v == "" {
		return nil, nil
	}
	b, e := strconv.ParseBool(v)
	return &b, e
}
func parseOptionalTime(v string) (*time.Time, error) {
	if v == "" {
		return nil, nil
	}
	t, e := time.Parse(time.RFC3339, v)
	return &t, e
}
func resourceID(r *http.Request, key string) string { return strings.TrimSpace(r.PathValue(key)) }
func validNoteType(t domain.NoteType) bool          { return t.Valid() }

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	mediaType, _, mediaErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaErr != nil || !strings.EqualFold(mediaType, "application/json") {
		writeAPIError(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type", "Expected application/json")
		return false
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeAPIError(w, r, http.StatusRequestEntityTooLarge, "request_too_large", "Request body exceeds the configured limit")
		} else {
			writeAPIError(w, r, http.StatusBadRequest, "malformed_request", "Request body is invalid")
		}
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeAPIError(w, r, http.StatusBadRequest, "malformed_request", "Request body must contain one JSON value")
		return false
	}
	return true
}

func respond(w http.ResponseWriter, r *http.Request, value any, err error, success int) {
	if err != nil {
		writeMappedError(w, r, err)
		return
	}
	writeJSON(w, success, value)
}
func respondDeleted(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		writeMappedError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeMappedError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, app.ErrNotFound):
		writeAPIError(w, r, http.StatusNotFound, "not_found", "Resource does not exist")
	case errors.Is(err, app.ErrReminderExists):
		writeAPIError(w, r, http.StatusConflict, "reminder_exists", "This note already has a reminder")
	case errors.Is(err, app.ErrInvalidRelationship):
		writeAPIError(w, r, http.StatusBadRequest, "invalid_relationship", "A referenced resource does not exist")
	case errors.Is(err, domain.ErrInvalidID), errors.Is(err, domain.ErrInvalidTitle), errors.Is(err, domain.ErrInvalidType), errors.Is(err, domain.ErrInvalidURL), errors.Is(err, domain.ErrInvalidName), errors.Is(err, domain.ErrInvalidReminder), errors.Is(err, domain.ErrInvalidSnooze), errors.Is(err, domain.ErrInvalidAttachment), errors.Is(err, domain.ErrInvalidTimestamp):
		writeAPIError(w, r, http.StatusBadRequest, "invalid_request", "Request values are invalid")
	case errors.Is(err, domain.ErrArchived):
		writeAPIError(w, r, http.StatusConflict, "archived_note", "Archived note must be restored before editing")
	case errors.Is(err, domain.ErrReminderCompleted):
		writeAPIError(w, r, http.StatusConflict, "reminder_completed", "Completed reminder cannot be rescheduled")
	default:
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeAPIError(w, r, http.StatusRequestEntityTooLarge, "request_too_large", "Request body exceeds the configured limit")
			return
		}
		writeAPIError(w, r, http.StatusInternalServerError, "internal_error", "The request could not be completed")
	}
}

type errorEnvelope struct {
	Error apiError `json:"error"`
}
type apiError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

func writeAPIError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeJSON(w, status, errorEnvelope{Error: apiError{Code: code, Message: message, RequestID: RequestID(r.Context())}})
}
func writeError(w http.ResponseWriter, status int, code, message, requestID string) {
	writeJSON(w, status, errorEnvelope{Error: apiError{Code: code, Message: message, RequestID: requestID}})
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func safeFilename(v string) string {
	v = strings.Map(func(r rune) rune {
		if r == '"' || r == '\\' || r < ' ' || r == ';' {
			return '_'
		}
		return r
	}, v)
	if v == "" {
		return "attachment"
	}
	return v
}
