package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"example.com/local-capture/internal/app"
	"example.com/local-capture/internal/domain"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	maxResponseBytes = 512 * 1024
	maxPageSize      = 50
)

type Application interface {
	CreateNote(context.Context, app.CreateNoteInput) (domain.Note, error)
	GetNote(context.Context, string) (domain.Note, error)
	UpdateNote(context.Context, string, app.UpdateNoteInput) (domain.Note, error)
	ArchiveNote(context.Context, string) (domain.Note, error)
	ListNotes(context.Context, app.ListNotesQuery) ([]domain.Note, error)
	SearchNotes(context.Context, app.SearchQuery) ([]domain.Note, error)
	ListCollections(context.Context) ([]domain.Collection, error)
	CreateCollection(context.Context, string) (domain.Collection, error)
	NoteStats(context.Context) (app.NoteStats, error)
}

type ReminderApplication interface {
	ListDueReminders(context.Context) ([]domain.Reminder, error)
}

// AuditEvent contains only client, operation, scope and outcome metadata.
// It intentionally has no note IDs, arguments, titles, URLs, or content.
type AuditEvent struct {
	ClientID string    `json:"client_id"`
	Action   string    `json:"action"`
	Scope    Scope     `json:"scope"`
	Outcome  string    `json:"outcome"`
	At       time.Time `json:"at"`
}

// MutationReceipt confirms a write without returning the note's private fields.
// Callers need notes:read as well as a write scope to receive note content.
type MutationReceipt struct {
	ID        string    `json:"id"`
	UpdatedAt time.Time `json:"updated_at"`
}
type Auditor interface {
	Record(context.Context, AuditEvent)
}
type AuditFunc func(context.Context, AuditEvent)

func (f AuditFunc) Record(ctx context.Context, event AuditEvent) { f(ctx, event) }

type ServerConfig struct {
	ClientID         string
	Grants           *GrantStore
	Auditor          Auditor
	MaxResponseBytes int
}

type Server struct {
	mcp             *sdk.Server
	app             Application
	reminders       ReminderApplication
	config          ServerConfig
	grantGeneration string
}

func NewServer(application Application, reminders ReminderApplication, config ServerConfig) (*Server, error) {
	if application == nil {
		return nil, errors.New("application services are required")
	}
	if config.Grants == nil {
		return nil, errors.New("MCP grant store is required")
	}
	if err := validateClientID(config.ClientID); err != nil {
		return nil, err
	}
	if config.MaxResponseBytes <= 0 {
		config.MaxResponseBytes = maxResponseBytes
	}
	s := &Server{app: application, reminders: reminders, config: config, grantGeneration: config.Grants.Generation(config.ClientID)}
	s.mcp = sdk.NewServer(&sdk.Implementation{Name: "local-capture-mcp", Version: "0.1.0", Description: "Local personal capture application adapter"}, &sdk.ServerOptions{Instructions: "Access is controlled by locally configured per-client permission scopes. Prompts do not grant permissions."})
	s.addTools()
	s.addResources()
	s.addPrompts()
	return s, nil
}

func (s *Server) Run(ctx context.Context, transport sdk.Transport) error {
	return s.mcp.Run(ctx, transport)
}

func (s *Server) authorize(ctx context.Context, action string, scope Scope) error {
	if !s.config.Grants.HasGeneration(s.config.ClientID, s.grantGeneration, scope, true) {
		s.audit(ctx, action, scope, "denied")
		return errors.New("permission denied: client lacks required scope " + string(scope))
	}
	return nil
}
func (s *Server) audit(ctx context.Context, action string, scope Scope, outcome string) {
	if s.config.Auditor != nil {
		s.config.Auditor.Record(ctx, AuditEvent{ClientID: s.config.ClientID, Action: action, Scope: scope, Outcome: outcome, At: time.Now().UTC()})
	}
}
func (s *Server) result(ctx context.Context, action string, scope Scope, value any, err error) (any, error) {
	if err != nil {
		s.audit(ctx, action, scope, "error")
		return nil, safeError(err)
	}
	value = s.filterAttachmentReferences(value)
	data, err := json.Marshal(value)
	if err != nil {
		s.audit(ctx, action, scope, "error")
		return nil, errors.New("could not encode result")
	}
	if len(data) > s.config.MaxResponseBytes {
		s.audit(ctx, action, scope, "error")
		return nil, fmt.Errorf("response exceeds %d byte limit; narrow the query", s.config.MaxResponseBytes)
	}
	s.audit(ctx, action, scope, "success")
	return value, nil
}

func mutationReceipt(note domain.Note) MutationReceipt {
	return MutationReceipt{ID: note.ID, UpdatedAt: note.UpdatedAt}
}

// Attachment IDs are references to attachment objects and require their own
// scope. Note readers can still read note content without learning attachment
// identifiers unless the client has attachments:read.
func (s *Server) filterAttachmentReferences(value any) any {
	if s.config.Grants.Has(s.config.ClientID, ScopeAttachmentsRead) {
		return value
	}
	switch notes := value.(type) {
	case domain.Note:
		notes.AttachmentIDs = nil
		return notes
	case []domain.Note:
		filtered := make([]domain.Note, len(notes))
		for i, note := range notes {
			note.AttachmentIDs = nil
			filtered[i] = note
		}
		return filtered
	default:
		return value
	}
}
func safeError(err error) error {
	switch {
	case errors.Is(err, app.ErrNotFound):
		return errors.New("resource not found")
	case errors.Is(err, domain.ErrInvalidID), errors.Is(err, domain.ErrInvalidTitle), errors.Is(err, domain.ErrInvalidType), errors.Is(err, domain.ErrInvalidURL), errors.Is(err, domain.ErrInvalidTimestamp), errors.Is(err, app.ErrInvalidRelationship):
		return errors.New("invalid or incomplete arguments")
	default:
		return errors.New("application operation failed")
	}
}

type CreateNoteArgs struct {
	Title        string   `json:"title" jsonschema:"optional title for the note"`
	Content      string   `json:"content"`
	Type         string   `json:"type" jsonschema:"one of thought, idea, task, link, image, quote, code_snippet, reference"`
	CollectionID string   `json:"collection_id,omitempty"`
	TagIDs       []string `json:"tag_ids,omitempty"`
	SourceURL    string   `json:"source_url,omitempty"`
}
type SearchArgs struct {
	Query         string   `json:"query"`
	CollectionID  string   `json:"collection_id,omitempty"`
	Type          string   `json:"type,omitempty"`
	Limit         int      `json:"limit,omitempty"`
	CreatedAfter  string   `json:"created_after,omitempty" jsonschema:"RFC3339 lower bound for note creation time"`
	CreatedBefore string   `json:"created_before,omitempty" jsonschema:"RFC3339 upper bound for note creation time"`
	TagIDs        []string `json:"tag_ids,omitempty"`
	Pinned        *bool    `json:"pinned,omitempty"`
	Archived      *bool    `json:"archived,omitempty"`
}
type GetNoteArgs struct {
	ID string `json:"id"`
}
type UpdateNoteArgs struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Type      string `json:"type"`
	SourceURL string `json:"source_url,omitempty"`
}
type ArchiveArgs struct {
	ID string `json:"id"`
}
type SaveLinkArgs struct {
	URL          string   `json:"url"`
	Title        string   `json:"title,omitempty"`
	SelectedText string   `json:"selected_text,omitempty"`
	Note         string   `json:"note,omitempty"`
	CollectionID string   `json:"collection_id,omitempty"`
	TagIDs       []string `json:"tag_ids,omitempty"`
}
type RecentArgs struct {
	Limit int `json:"limit,omitempty"`
}
type CreateCollectionArgs struct {
	Name string `json:"name"`
}
type EmptyArgs struct{}

func (s *Server) addTools() {
	sdk.AddTool(s.mcp, &sdk.Tool{Name: "create_note", Description: "Create a note in the local vault."}, func(ctx context.Context, _ *sdk.CallToolRequest, in CreateNoteArgs) (*sdk.CallToolResult, any, error) {
		const scope = ScopeNotesCreate
		if err := s.authorize(ctx, "create_note", scope); err != nil {
			return nil, nil, err
		}
		if len(in.Content)+len(in.Title)+len(in.SourceURL) > 256*1024 || len(in.Title) > 1000 {
			return nil, nil, errors.New("note fields exceed limits")
		}
		if !domain.NoteType(in.Type).Valid() {
			return nil, nil, errors.New("invalid note type")
		}
		out, err := s.app.CreateNote(ctx, app.CreateNoteInput{Title: in.Title, Content: in.Content, Type: domain.NoteType(in.Type), CollectionID: in.CollectionID, TagIDs: in.TagIDs, SourceURL: in.SourceURL})
		value, e := s.result(ctx, "create_note", scope, out, err)
		return nil, value, e
	})
	sdk.AddTool(s.mcp, &sdk.Tool{Name: "search_notes", Description: "Search local notes and return matching note content."}, func(ctx context.Context, _ *sdk.CallToolRequest, in SearchArgs) (*sdk.CallToolResult, any, error) {
		const scope = ScopeNotesRead
		if err := s.authorize(ctx, "search_notes", scope); err != nil {
			return nil, nil, err
		}
		limit, err := boundedLimit(in.Limit)
		if err != nil {
			return nil, nil, err
		}
		var noteType domain.NoteType
		if in.Type != "" {
			noteType = domain.NoteType(in.Type)
			if !noteType.Valid() {
				return nil, nil, errors.New("invalid note type")
			}
		}
		if len(in.Query) > 4096 || len(in.CollectionID) > 256 || len(in.TagIDs) > 50 {
			return nil, nil, errors.New("search filters exceed limits")
		}
		for _, id := range in.TagIDs {
			if len(id) > 256 {
				return nil, nil, errors.New("search filters exceed limits")
			}
		}
		after, err := parseOptionalTime(in.CreatedAfter)
		if err != nil {
			return nil, nil, errors.New("created_after must be an RFC3339 timestamp")
		}
		before, err := parseOptionalTime(in.CreatedBefore)
		if err != nil {
			return nil, nil, errors.New("created_before must be an RFC3339 timestamp")
		}
		if after != nil && before != nil && !after.Before(*before) {
			return nil, nil, errors.New("created_after must be earlier than created_before")
		}
		out, err := s.app.SearchNotes(ctx, app.SearchQuery{Text: in.Query, CollectionID: in.CollectionID, Type: noteType, CreatedAfter: after, CreatedBefore: before, TagIDs: in.TagIDs, Pinned: in.Pinned, Archived: in.Archived, Limit: limit})
		value, e := s.result(ctx, "search_notes", scope, out, err)
		return nil, value, e
	})
	sdk.AddTool(s.mcp, &sdk.Tool{Name: "get_inbox_notes", Description: "List unarchived notes with no collection, bounded to 50 results."}, func(ctx context.Context, _ *sdk.CallToolRequest, in RecentArgs) (*sdk.CallToolResult, any, error) {
		const scope = ScopeNotesRead
		if err := s.authorize(ctx, "get_inbox_notes", scope); err != nil {
			return nil, nil, err
		}
		limit, err := boundedLimit(in.Limit)
		if err != nil {
			return nil, nil, err
		}
		archived := false
		out, err := s.app.ListNotes(ctx, app.ListNotesQuery{InboxOnly: true, Archived: &archived, Limit: limit})
		value, e := s.result(ctx, "get_inbox_notes", scope, out, err)
		return nil, value, e
	})
	sdk.AddTool(s.mcp, &sdk.Tool{Name: "get_note", Description: "Read one local note by ID."}, func(ctx context.Context, _ *sdk.CallToolRequest, in GetNoteArgs) (*sdk.CallToolResult, any, error) {
		const scope = ScopeNotesRead
		if err := s.authorize(ctx, "get_note", scope); err != nil {
			return nil, nil, err
		}
		if strings.TrimSpace(in.ID) == "" {
			return nil, nil, errors.New("id is required")
		}
		out, err := s.app.GetNote(ctx, in.ID)
		value, e := s.result(ctx, "get_note", scope, out, err)
		return nil, value, e
	})
	sdk.AddTool(s.mcp, &sdk.Tool{Name: "update_note", Description: "Edit a local note. Requires notes:edit separately from notes:create."}, func(ctx context.Context, _ *sdk.CallToolRequest, in UpdateNoteArgs) (*sdk.CallToolResult, any, error) {
		const scope = ScopeNotesEdit
		if err := s.authorize(ctx, "update_note", scope); err != nil {
			return nil, nil, err
		}
		if strings.TrimSpace(in.ID) == "" || !domain.NoteType(in.Type).Valid() {
			return nil, nil, errors.New("id and valid type are required")
		}
		current, err := s.app.GetNote(ctx, in.ID)
		if err != nil {
			value, e := s.result(ctx, "update_note", scope, nil, err)
			return nil, value, e
		}
		sourceURL := in.SourceURL
		if sourceURL == "" {
			sourceURL = current.Metadata.SourceURL
		}
		out, err := s.app.UpdateNote(ctx, in.ID, app.UpdateNoteInput{Title: in.Title, Content: in.Content, Type: domain.NoteType(in.Type), SourceURL: sourceURL, AttachmentIDs: current.AttachmentIDs})
		value, e := s.result(ctx, "update_note", scope, mutationReceipt(out), err)
		return nil, value, e
	})
	sdk.AddTool(s.mcp, &sdk.Tool{Name: "archive_note", Description: "Archive a local note. Requires notes:archive."}, func(ctx context.Context, _ *sdk.CallToolRequest, in ArchiveArgs) (*sdk.CallToolResult, any, error) {
		const scope = ScopeNotesArchive
		if err := s.authorize(ctx, "archive_note", scope); err != nil {
			return nil, nil, err
		}
		if in.ID == "" {
			return nil, nil, errors.New("id is required")
		}
		out, err := s.app.ArchiveNote(ctx, in.ID)
		value, e := s.result(ctx, "archive_note", scope, mutationReceipt(out), err)
		return nil, value, e
	})
	sdk.AddTool(s.mcp, &sdk.Tool{Name: "list_collections", Description: "List collection names and IDs."}, func(ctx context.Context, _ *sdk.CallToolRequest, _ EmptyArgs) (*sdk.CallToolResult, any, error) {
		const scope = ScopeCollectionsRead
		if err := s.authorize(ctx, "list_collections", scope); err != nil {
			return nil, nil, err
		}
		out, err := s.app.ListCollections(ctx)
		value, e := s.result(ctx, "list_collections", scope, out, err)
		return nil, value, e
	})
	sdk.AddTool(s.mcp, &sdk.Tool{Name: "save_link", Description: "Save a webpage URL with optional selected text and personal note."}, func(ctx context.Context, _ *sdk.CallToolRequest, in SaveLinkArgs) (*sdk.CallToolResult, any, error) {
		const scope = ScopeNotesCreate
		if err := s.authorize(ctx, "save_link", scope); err != nil {
			return nil, nil, err
		}
		u, err := url.ParseRequestURI(in.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, nil, errors.New("URL must be an absolute HTTP or HTTPS URL")
		}
		if len(in.URL)+len(in.SelectedText)+len(in.Note)+len(in.Title) > 256*1024 {
			return nil, nil, errors.New("link fields exceed limits")
		}
		content := in.Note
		if in.SelectedText != "" {
			if content != "" {
				content += "\n\n"
			}
			content += in.SelectedText
		}
		title := in.Title
		if strings.TrimSpace(title) == "" {
			title = u.Host
		}
		out, err := s.app.CreateNote(ctx, app.CreateNoteInput{Title: title, Content: content, Type: domain.NoteTypeLink, SourceURL: in.URL, CollectionID: in.CollectionID, TagIDs: in.TagIDs})
		value, e := s.result(ctx, "save_link", scope, out, err)
		return nil, value, e
	})
	sdk.AddTool(s.mcp, &sdk.Tool{Name: "get_recent_notes", Description: "Return a bounded list of recent local notes."}, func(ctx context.Context, _ *sdk.CallToolRequest, in RecentArgs) (*sdk.CallToolResult, any, error) {
		const scope = ScopeNotesRead
		if err := s.authorize(ctx, "get_recent_notes", scope); err != nil {
			return nil, nil, err
		}
		limit, err := boundedLimit(in.Limit)
		if err != nil {
			return nil, nil, err
		}
		archived := false
		out, err := s.app.ListNotes(ctx, app.ListNotesQuery{Limit: limit, Archived: &archived})
		value, e := s.result(ctx, "get_recent_notes", scope, out, err)
		return nil, value, e
	})
	sdk.AddTool(s.mcp, &sdk.Tool{Name: "get_due_items", Description: "Return due reminder metadata and note IDs; does not include note content."}, func(ctx context.Context, _ *sdk.CallToolRequest, _ EmptyArgs) (*sdk.CallToolResult, any, error) {
		const scope = ScopeMetadataRead
		if err := s.authorize(ctx, "get_due_items", scope); err != nil {
			return nil, nil, err
		}
		if s.reminders == nil {
			return nil, nil, errors.New("reminder service is unavailable")
		}
		out, err := s.reminders.ListDueReminders(ctx)
		value, e := s.result(ctx, "get_due_items", scope, out, err)
		return nil, value, e
	})
	sdk.AddTool(s.mcp, &sdk.Tool{Name: "create_collection", Description: "Create a collection in the local vault."}, func(ctx context.Context, _ *sdk.CallToolRequest, in CreateCollectionArgs) (*sdk.CallToolResult, any, error) {
		const scope = ScopeCollectionsWrite
		if err := s.authorize(ctx, "create_collection", scope); err != nil {
			return nil, nil, err
		}
		if strings.TrimSpace(in.Name) == "" || len(in.Name) > 200 {
			return nil, nil, errors.New("name must be 1 to 200 characters")
		}
		out, err := s.app.CreateCollection(ctx, in.Name)
		value, e := s.result(ctx, "create_collection", scope, out, err)
		return nil, value, e
	})
}

func boundedLimit(n int) (int, error) {
	if n == 0 {
		return 20, nil
	}
	if n < 1 || n > maxPageSize {
		return 0, fmt.Errorf("limit must be between 1 and %d", maxPageSize)
	}
	return n, nil
}

func parseOptionalTime(value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, err
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

func (s *Server) addResources() {
	add := func(uri, name, description string, scope Scope, load func(context.Context) (any, error)) {
		s.mcp.AddResource(&sdk.Resource{URI: uri, Name: name, Description: description, MIMEType: "application/json"}, func(ctx context.Context, _ *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
			if err := s.authorize(ctx, "resource:"+uri, scope); err != nil {
				return nil, err
			}
			value, err := load(ctx)
			if err != nil {
				s.audit(ctx, "resource:"+uri, scope, "error")
				return nil, safeError(err)
			}
			value = s.filterAttachmentReferences(value)
			data, err := json.Marshal(value)
			if err != nil || len(data) > s.config.MaxResponseBytes {
				s.audit(ctx, "resource:"+uri, scope, "error")
				return nil, errors.New("resource exceeds response limit")
			}
			s.audit(ctx, "resource:"+uri, scope, "success")
			return &sdk.ReadResourceResult{Contents: []*sdk.ResourceContents{{URI: uri, MIMEType: "application/json", Text: string(data)}}}, nil
		})
	}
	add("braindump://notes/recent", "Recent notes", "Recent active notes; contains private note content.", ScopeNotesRead, func(ctx context.Context) (any, error) {
		archived := false
		return s.app.ListNotes(ctx, app.ListNotesQuery{Limit: 20, Archived: &archived})
	})
	add("braindump://collections", "Collections", "Collection names and IDs.", ScopeCollectionsRead, func(ctx context.Context) (any, error) { return s.app.ListCollections(ctx) })
	add("braindump://stats", "Statistics", "Aggregate note metadata only.", ScopeMetadataRead, func(ctx context.Context) (any, error) { return s.app.NoteStats(ctx) })
	add("braindump://tasks/due", "Due tasks", "Due reminder metadata; no note content.", ScopeMetadataRead, func(ctx context.Context) (any, error) {
		if s.reminders == nil {
			return nil, errors.New("reminder service is unavailable")
		}
		return s.reminders.ListDueReminders(ctx)
	})
}

type promptDefinition struct {
	title       string
	description string
	arguments   []*sdk.PromptArgument
	build       func(map[string]string) (string, error)
}

var prompts = map[string]promptDefinition{
	"organise_my_inbox": {
		title: "Organise my inbox", description: "Suggest an organisation plan for uncategorised notes without changing them.",
		arguments: []*sdk.PromptArgument{{Name: "limit", Description: "Maximum inbox notes to inspect (1 to 50). Optional."}},
		build: func(args map[string]string) (string, error) {
			limit := args["limit"]
			if limit == "" {
				limit = "20"
			}
			parsed, err := strconv.Atoi(limit)
			if err != nil || parsed < 1 || parsed > maxPageSize {
				return "", errors.New("limit must be between 1 and 50")
			}
			return "Use get_inbox_notes with limit=" + limit + " and list_collections. Suggest a collection and tags for each note with a short reason. Do not change any data. Ask before any move or tag change. This MCP interface currently has no collection-move or tag-assignment tool, so do not invent a write workaround.", nil
		},
	},
	"find_related_ideas": {
		title: "Find related ideas", description: "Find related notes using a focused user query.", arguments: []*sdk.PromptArgument{{Name: "query", Description: "Topic or idea to explore.", Required: true}},
		build: func(args map[string]string) (string, error) {
			query, err := requiredPromptArg(args, "query", 1000)
			if err != nil {
				return "", err
			}
			return "Search local notes for this topic: " + query + ". Compare matching notes across collections, cite IDs and titles, explain the connection, and state uncertainty. Do not modify notes or fetch external pages.", nil
		},
	},
	"personal_knowledge_librarian": {
		title: "Personal knowledge librarian", description: "Find and organise a focused slice of the user's notes.", arguments: []*sdk.PromptArgument{{Name: "topic", Description: "Topic or area of knowledge to review.", Required: true}},
		build: func(args map[string]string) (string, error) {
			topic, err := requiredPromptArg(args, "topic", 1000)
			if err != nil {
				return "", err
			}
			return "Act as a librarian for this topic only: " + topic + ". Use search_notes with a bounded limit and list_collections if relevant. Identify the strongest notes, possible duplicates, missing links, and a suggested place for uncategorised notes. Cite note IDs and titles. Do not scan all notes or make changes; ask for approval before any mutation.", nil
		},
	},
	"connect_notes_across_collections": {
		title: "Connect notes across collections", description: "Find connections between selected notes in different collections.", arguments: []*sdk.PromptArgument{{Name: "topic", Description: "Question or theme to connect.", Required: true}},
		build: func(args map[string]string) (string, error) {
			topic, err := requiredPromptArg(args, "topic", 1000)
			if err != nil {
				return "", err
			}
			return "Search notes for this theme: " + topic + ". Use list_collections to identify collection names, then compare only the bounded matching notes across collections. Cite note IDs, titles, and collection IDs. Explain each proposed connection and uncertainty. Do not create links or edit notes.", nil
		},
	},
	"summarise_recent_notes": {
		title: "Summarise recent notes", description: "Create a digest from a small, explicitly bounded set of recent notes.", arguments: []*sdk.PromptArgument{{Name: "limit", Description: "Maximum recent notes to inspect (1 to 50). Optional."}},
		build: func(args map[string]string) (string, error) {
			limit := args["limit"]
			if limit == "" {
				limit = "10"
			}
			parsed, err := strconv.Atoi(limit)
			if err != nil || parsed < 1 || parsed > maxPageSize {
				return "", errors.New("limit must be between 1 and 50")
			}
			return "Call get_recent_notes with limit=" + limit + ". Summarise only the returned notes, grouped by themes and open questions. Cite note IDs/titles. Do not request more notes automatically and do not modify anything.", nil
		},
	},
	"summarise_notes_this_week": {
		title: "Summarise my notes this week", description: "Summarise only the date range selected by the user.", arguments: []*sdk.PromptArgument{{Name: "created_after", Description: "RFC3339 start time for the selected week.", Required: true}, {Name: "created_before", Description: "RFC3339 exclusive end time.", Required: true}},
		build: func(args map[string]string) (string, error) {
			after, err := requiredPromptArg(args, "created_after", 64)
			if err != nil {
				return "", err
			}
			before, err := requiredPromptArg(args, "created_before", 64)
			if err != nil {
				return "", err
			}
			a, e1 := parseOptionalTime(after)
			b, e2 := parseOptionalTime(before)
			if e1 != nil || e2 != nil || a == nil || b == nil || !a.Before(*b) {
				return "", errors.New("week boundaries must be ordered RFC3339 timestamps")
			}
			return "Summarise this selected week only. Call search_notes with created_after=" + after + " and created_before=" + before + "; use a bounded limit and request more only if the user asks. Group themes, decisions, open loops, and notable ideas. Cite note IDs/titles. Do not modify notes.", nil
		},
	},
	"find_forgotten_tasks": {
		title: "Find forgotten tasks", description: "Identify overdue and stale tasks from local task and reminder metadata.",
		build: func(map[string]string) (string, error) {
			return "Call get_due_items and search_notes with type=task and archived=false, using a bounded limit. Prioritise overdue reminders first, then older active task notes. Completion state is incomplete for notes without a reminder, so label these as candidates and ask the user whether they are still open. Report evidence and uncertainty. Do not complete, snooze, archive, or edit tasks; ask before any change.", nil
		},
	},
	"turn_notes_into_project_outline": {
		title: "Turn my notes into a project outline", description: "Build a focused project outline from notes matching a user-selected topic.", arguments: []*sdk.PromptArgument{{Name: "project", Description: "Project name or topic to search for.", Required: true}},
		build: func(args map[string]string) (string, error) {
			project, err := requiredPromptArg(args, "project", 1000)
			if err != nil {
				return "", err
			}
			return "Create a draft outline for this project using only targeted search results for: " + project + ". Use search_notes and optionally list_collections; cite note IDs/titles and flag gaps. Do not fetch all notes or modify anything. Ask before saving a note.", nil
		},
	},
	"weekly_personal_review": {
		title: "Weekly personal review", description: "Review a selected week's ideas, tasks, and open loops.", arguments: []*sdk.PromptArgument{{Name: "created_after", Description: "RFC3339 start time.", Required: true}, {Name: "created_before", Description: "RFC3339 exclusive end time.", Required: true}},
		build: func(args map[string]string) (string, error) {
			after, err := requiredPromptArg(args, "created_after", 64)
			if err != nil {
				return "", err
			}
			before, err := requiredPromptArg(args, "created_before", 64)
			if err != nil {
				return "", err
			}
			a, e1 := parseOptionalTime(after)
			b, e2 := parseOptionalTime(before)
			if e1 != nil || e2 != nil || a == nil || b == nil || !a.Before(*b) {
				return "", errors.New("review boundaries must be ordered RFC3339 timestamps")
			}
			return "For the selected period " + after + " through " + before + ", call search_notes with these creation bounds and a bounded limit, plus get_due_items for due reminders. Summarise progress, recurring themes, unfinished tasks, and a short next-week plan. Cite notes. Do not edit or complete items without explicit user approval.", nil
		},
	},
	"prioritise_tasks": {
		title: "Prioritise my tasks", description: "Rank local tasks using due dates and age; no priority field is changed.",
		build: func(map[string]string) (string, error) {
			return "Call get_due_items and search_notes with type=task, archived=false, and a bounded limit. Rank overdue items first, then upcoming reminders, then undated tasks by age. Explain the rule used and flag missing due dates. Do not alter reminders or notes.", nil
		},
	},
	"reading_queue_review": {
		title: "Review my reading queue", description: "List saved link notes as a reading queue without fetching websites.",
		build: func(map[string]string) (string, error) {
			return "Call search_notes with type=link, archived=false, and a bounded limit. Organise the saved links by note title and URL, then suggest a reading order from the user's stated interests. Do not fetch links or claim to have read their pages. Do not modify notes.", nil
		},
	},
	"meeting_action_extraction": {
		title: "Extract meeting actions", description: "Extract decisions and action items from content the user explicitly supplies.", arguments: []*sdk.PromptArgument{{Name: "meeting_notes", Description: "Meeting text imported or selected by the user (up to 64 KiB).", Required: true}},
		build: func(args map[string]string) (string, error) {
			content, err := requiredPromptArg(args, "meeting_notes", 64*1024)
			if err != nil {
				return "", err
			}
			return "Extract decisions, action items, owners, due dates, and unresolved questions from only this user-supplied meeting text. Mark missing owners or dates instead of guessing. Do not read other notes. Do not save or modify anything unless the user explicitly approves a separate create_note action.\n\nUser-supplied meeting text:\n" + content, nil
		},
	},
}

func requiredPromptArg(args map[string]string, name string, max int) (string, error) {
	value := strings.TrimSpace(args[name])
	if value == "" {
		return "", errors.New("required prompt argument is missing")
	}
	if len(value) > max {
		return "", errors.New("prompt argument exceeds its size limit")
	}
	return value, nil
}

func (s *Server) addPrompts() {
	for name, prompt := range prompts {
		n, p := name, prompt
		s.mcp.AddPrompt(&sdk.Prompt{Name: n, Title: p.title, Description: p.description, Arguments: p.arguments}, func(_ context.Context, req *sdk.GetPromptRequest) (*sdk.GetPromptResult, error) {
			body, err := p.build(req.Params.Arguments)
			if err != nil {
				return nil, err
			}
			return &sdk.GetPromptResult{Description: p.description, Messages: []*sdk.PromptMessage{{Role: sdk.Role("user"), Content: &sdk.TextContent{Text: body + " The MCP server does not call an AI provider. Prompts do not grant permission scopes; tool access is independently checked. Do not transmit more notes than needed."}}}}, nil
		})
	}
}
