package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"example.com/local-capture/internal/app"
	"example.com/local-capture/internal/domain"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type fakeApp struct {
	created           int
	note              domain.Note
	collections       []domain.Collection
	stats             app.NoteStats
	notes             []domain.Note
	err               error
	updated           app.UpdateNoteInput
	updatedCalls      int
	archivedCalls     int
	collectionCreates int
	searchQuery       app.SearchQuery
	listQuery         app.ListNotesQuery
}

func (f *fakeApp) CreateNote(_ context.Context, in app.CreateNoteInput) (domain.Note, error) {
	f.created++
	if f.err != nil {
		return domain.Note{}, f.err
	}
	n := domain.Note{ID: "secret-note-id", Title: in.Title, Content: in.Content, Type: in.Type, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	return n, nil
}
func (f *fakeApp) GetNote(context.Context, string) (domain.Note, error) {
	if f.err != nil {
		return domain.Note{}, f.err
	}
	return f.note, nil
}
func (f *fakeApp) UpdateNote(_ context.Context, _ string, in app.UpdateNoteInput) (domain.Note, error) {
	f.updatedCalls++
	f.updated = in
	if f.err != nil {
		return domain.Note{}, f.err
	}
	return f.note, nil
}
func (f *fakeApp) ArchiveNote(context.Context, string) (domain.Note, error) {
	f.archivedCalls++
	if f.err != nil {
		return domain.Note{}, f.err
	}
	return f.note, nil
}
func (f *fakeApp) ListNotes(_ context.Context, query app.ListNotesQuery) ([]domain.Note, error) {
	f.listQuery = query
	return f.notes, f.err
}
func (f *fakeApp) SearchNotes(_ context.Context, query app.SearchQuery) ([]domain.Note, error) {
	f.searchQuery = query
	return f.notes, f.err
}
func (f *fakeApp) ListCollections(context.Context) ([]domain.Collection, error) {
	return f.collections, f.err
}
func (f *fakeApp) CreateCollection(_ context.Context, name string) (domain.Collection, error) {
	f.collectionCreates++
	if f.err != nil {
		return domain.Collection{}, f.err
	}
	return domain.Collection{ID: "c", Name: name}, nil
}
func (f *fakeApp) NoteStats(context.Context) (app.NoteStats, error) { return f.stats, f.err }

type fakeReminders struct{ values []domain.Reminder }

func (f fakeReminders) ListDueReminders(context.Context) ([]domain.Reminder, error) {
	return f.values, nil
}

func newTestSession(t *testing.T, application Application, grants *GrantStore, auditor Auditor, max int) (*sdk.ClientSession, *Server) {
	t.Helper()
	server, err := NewServer(application, nil, ServerConfig{ClientID: "test-client", Grants: grants, Auditor: auditor, MaxResponseBytes: max})
	if err != nil {
		t.Fatal(err)
	}
	serverTransport, clientTransport := sdk.NewInMemoryTransports()
	ctx := context.Background()
	go func() { _ = server.mcp.Run(ctx, serverTransport) }()
	client := sdk.NewClient(&sdk.Implementation{Name: "test-client", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session, server
}
func grantAll(t *testing.T, store *GrantStore, scopes ...Scope) {
	t.Helper()
	if err := store.Grant("test-client", scopes); err != nil {
		t.Fatal(err)
	}
}
func call(t *testing.T, session *sdk.ClientSession, name string, args map[string]any) *sdk.CallToolResult {
	t.Helper()
	raw, _ := json.Marshal(args)
	var decoded map[string]any
	_ = json.Unmarshal(raw, &decoded)
	result, err := session.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: decoded})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestUnauthorizedClientCannotReadOrWrite(t *testing.T) {
	store, _ := OpenGrantStore(filepath.Join(t.TempDir(), "grants.json"))
	application := &fakeApp{}
	session, _ := newTestSession(t, application, store, nil, 0)
	result := call(t, session, "create_note", map[string]any{"title": "x", "content": "body", "type": "idea"})
	if !result.IsError {
		t.Fatal("expected permission error")
	}
	if application.created != 0 {
		t.Fatal("unauthorized call reached app service")
	}
}
func TestReadOnlyCannotWriteButCanSearch(t *testing.T) {
	store, _ := OpenGrantStore(filepath.Join(t.TempDir(), "grants.json"))
	grantAll(t, store, ScopeNotesRead)
	application := &fakeApp{}
	session, _ := newTestSession(t, application, store, nil, 0)
	written := call(t, session, "create_note", map[string]any{"title": "x", "content": "body", "type": "idea"})
	if !written.IsError || application.created != 0 {
		t.Fatal("read-only grant allowed write")
	}
	read := call(t, session, "search_notes", map[string]any{"query": "hello"})
	if read.IsError {
		t.Fatalf("read denied: %v", read.Content)
	}
}

func TestEditArchiveAndCollectionWriteRequireSeparateScopes(t *testing.T) {
	store, _ := OpenGrantStore(filepath.Join(t.TempDir(), "grants.json"))
	grantAll(t, store, ScopeNotesRead, ScopeNotesCreate)
	application := &fakeApp{note: domain.Note{ID: "note"}}
	session, _ := newTestSession(t, application, store, nil, 0)
	update := call(t, session, "update_note", map[string]any{"id": "note", "title": "changed", "content": "body", "type": "idea"})
	archive := call(t, session, "archive_note", map[string]any{"id": "note"})
	collection := call(t, session, "create_collection", map[string]any{"name": "private"})
	if !update.IsError || !archive.IsError || !collection.IsError {
		t.Fatal("create/read scopes granted edit, archive, or collection write")
	}
	if application.updatedCalls != 0 || application.archivedCalls != 0 || application.collectionCreates != 0 {
		t.Fatal("unauthorized mutation reached application service")
	}
}
func TestCreateRequiresWriteScopeAndSucceedsWithGrant(t *testing.T) {
	store, _ := OpenGrantStore(filepath.Join(t.TempDir(), "grants.json"))
	grantAll(t, store, ScopeNotesCreate)
	application := &fakeApp{}
	session, _ := newTestSession(t, application, store, nil, 0)
	result := call(t, session, "create_note", map[string]any{"title": "idea", "content": "private", "type": "idea"})
	if result.IsError || application.created != 1 {
		t.Fatalf("create failed: %+v", result)
	}
}

func TestSearchNotesSupportsBoundedWorkflowFilters(t *testing.T) {
	store, _ := OpenGrantStore(filepath.Join(t.TempDir(), "grants.json"))
	grantAll(t, store, ScopeNotesRead)
	application := &fakeApp{}
	session, _ := newTestSession(t, application, store, nil, 0)
	result := call(t, session, "search_notes", map[string]any{"query": "project", "type": "task", "created_after": "2026-10-01T00:00:00Z", "created_before": "2026-11-01T00:00:00Z", "tag_ids": []string{"tag-a"}, "pinned": false, "archived": false, "limit": 12})
	if result.IsError {
		t.Fatalf("filtered search failed: %v", result.Content)
	}
	q := application.searchQuery
	if q.Text != "project" || q.Type != domain.NoteTypeTask || q.Limit != 12 || q.CreatedAfter == nil || q.CreatedBefore == nil || len(q.TagIDs) != 1 || q.TagIDs[0] != "tag-a" || q.Pinned == nil || *q.Pinned || q.Archived == nil || *q.Archived {
		t.Fatalf("filters not passed to app service: %+v", q)
	}
}

func TestInboxToolRequestsOnlyUncollectedActiveNotes(t *testing.T) {
	store, _ := OpenGrantStore(filepath.Join(t.TempDir(), "grants.json"))
	grantAll(t, store, ScopeNotesRead)
	application := &fakeApp{}
	session, _ := newTestSession(t, application, store, nil, 0)
	result := call(t, session, "get_inbox_notes", map[string]any{"limit": 7})
	if result.IsError {
		t.Fatalf("inbox query failed: %v", result.Content)
	}
	if !application.listQuery.InboxOnly || application.listQuery.Limit != 7 || application.listQuery.Archived == nil || *application.listQuery.Archived {
		t.Fatalf("inbox filter not passed: %+v", application.listQuery)
	}
}
func TestInvalidArgumentsLargeResultsAndNoSensitiveAuditData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "grants.json")
	store, _ := OpenGrantStore(path)
	grantAll(t, store, ScopeNotesRead, ScopeNotesCreate)
	var events []AuditEvent
	auditor := AuditFunc(func(_ context.Context, e AuditEvent) { events = append(events, e) })
	application := &fakeApp{note: domain.Note{ID: "private-id", Title: "TOP SECRET TITLE", Content: "TOP SECRET BODY password-canary ciphertext-canary", Type: domain.NoteTypeIdea}, notes: []domain.Note{{ID: "private-id", Title: "TOP SECRET TITLE", Content: "TOP SECRET BODY password-canary ciphertext-canary", Type: domain.NoteTypeIdea}}}
	session, _ := newTestSession(t, application, store, auditor, 32)
	bad := call(t, session, "create_note", map[string]any{"title": "bad", "content": "body", "type": "unrecognized"})
	if !bad.IsError {
		t.Fatal("invalid note type was accepted")
	}
	large := call(t, session, "search_notes", map[string]any{"query": "x"})
	if !large.IsError {
		t.Fatal("oversized result was accepted")
	}
	read := call(t, session, "get_note", map[string]any{"id": "private-id"})
	if !read.IsError {
		t.Fatal("small response limit should reject private note")
	}
	encoded, _ := json.Marshal(events)
	if strings.Contains(string(encoded), "TOP SECRET") || strings.Contains(string(encoded), "private-id") || strings.Contains(string(encoded), "password-canary") || strings.Contains(string(encoded), "ciphertext-canary") {
		t.Fatalf("audit contains private data: %s", encoded)
	}
}

func TestRecentNotesResourceRedactsAttachmentIDs(t *testing.T) {
	store, _ := OpenGrantStore(filepath.Join(t.TempDir(), "grants.json"))
	grantAll(t, store, ScopeNotesRead)
	application := &fakeApp{notes: []domain.Note{{ID: "note", Title: "title", Content: "body", Type: domain.NoteTypeIdea, AttachmentIDs: []string{"hidden-attachment"}}}}
	session, _ := newTestSession(t, application, store, nil, 0)
	resource, err := session.ReadResource(context.Background(), &sdk.ReadResourceParams{URI: "braindump://notes/recent"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(resource)
	if strings.Contains(string(raw), "hidden-attachment") {
		t.Fatalf("resource leaked attachment reference: %s", raw)
	}
}

func TestNotesReadDoesNotExposeAttachmentReferencesWithoutAttachmentScope(t *testing.T) {
	store, _ := OpenGrantStore(filepath.Join(t.TempDir(), "grants.json"))
	grantAll(t, store, ScopeNotesRead)
	application := &fakeApp{note: domain.Note{ID: "note", Title: "private title", Content: "private body", Type: domain.NoteTypeIdea, AttachmentIDs: []string{"attachment-secret-id"}}}
	session, _ := newTestSession(t, application, store, nil, 0)
	result := call(t, session, "get_note", map[string]any{"id": "note"})
	if result.IsError {
		t.Fatalf("get_note failed: %v", result.Content)
	}
	raw, _ := json.Marshal(result)
	if strings.Contains(string(raw), "attachment-secret-id") {
		t.Fatalf("attachment reference escaped without attachments:read: %s", raw)
	}
}

func TestAttachmentScopeAloneDoesNotGrantNoteOrAttachmentAccess(t *testing.T) {
	store, _ := OpenGrantStore(filepath.Join(t.TempDir(), "grants.json"))
	grantAll(t, store, ScopeAttachmentsRead)
	session, _ := newTestSession(t, &fakeApp{}, store, nil, 0)
	result := call(t, session, "get_note", map[string]any{"id": "note"})
	if !result.IsError {
		t.Fatal("attachments:read improperly granted notes:read")
	}
	resources, err := session.ListResources(context.Background(), &sdk.ListResourcesParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(resources.Resources) != 4 {
		t.Fatalf("expected only the four declared resources, got %d", len(resources.Resources))
	}
}

func TestToolsDoNotExposeAttachmentReadOrPermanentDelete(t *testing.T) {
	store, _ := OpenGrantStore(filepath.Join(t.TempDir(), "grants.json"))
	session, _ := newTestSession(t, &fakeApp{}, store, nil, 0)
	tools, err := session.ListTools(context.Background(), &sdk.ListToolsParams{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools.Tools {
		if tool.Name == "delete_note" || strings.Contains(tool.Name, "attachment") {
			t.Fatalf("unexpected sensitive operation registered: %s", tool.Name)
		}
	}
}

func TestEveryResourceRequiresItsDedicatedScope(t *testing.T) {
	resources := []struct {
		uri   string
		scope Scope
	}{{"braindump://notes/recent", ScopeNotesRead}, {"braindump://collections", ScopeCollectionsRead}, {"braindump://stats", ScopeMetadataRead}, {"braindump://tasks/due", ScopeMetadataRead}}
	for _, resource := range resources {
		t.Run(resource.uri, func(t *testing.T) {
			store, _ := OpenGrantStore(filepath.Join(t.TempDir(), "grants.json"))
			for _, scope := range []Scope{ScopeNotesRead, ScopeCollectionsRead, ScopeMetadataRead} {
				if scope != resource.scope {
					grantAll(t, store, scope)
				}
			}
			session, _ := newTestSession(t, &fakeApp{}, store, nil, 0)
			if _, err := session.ReadResource(context.Background(), &sdk.ReadResourceParams{URI: resource.uri}); err == nil {
				t.Fatalf("resource accessible without %s", resource.scope)
			}
		})
	}
}
func TestRevocationTakesEffectForLiveServer(t *testing.T) {
	store, _ := OpenGrantStore(filepath.Join(t.TempDir(), "grants.json"))
	grantAll(t, store, ScopeNotesRead)
	session, _ := newTestSession(t, &fakeApp{}, store, nil, 0)
	if err := store.Revoke("test-client"); err != nil {
		t.Fatal(err)
	}
	result := call(t, session, "search_notes", map[string]any{"query": "x"})
	if !result.IsError {
		t.Fatal("revoked client retained access")
	}
}

func TestExternalGrantRevocationTakesEffectForLiveServer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "grants.json")
	serverGrants, _ := OpenGrantStore(path)
	grantAll(t, serverGrants, ScopeNotesRead)
	adminGrants, _ := OpenGrantStore(path)
	session, _ := newTestSession(t, &fakeApp{}, serverGrants, nil, 0)
	if err := adminGrants.Revoke("test-client"); err != nil {
		t.Fatal(err)
	}
	result := call(t, session, "search_notes", map[string]any{"query": "x"})
	if !result.IsError {
		t.Fatal("external revocation was not observed")
	}
}

func TestRegrantDoesNotReviveRevokedServerProcess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "grants.json")
	oldGrants, _ := OpenGrantStore(path)
	grantAll(t, oldGrants, ScopeNotesRead)
	oldSession, _ := newTestSession(t, &fakeApp{}, oldGrants, nil, 0)
	admin, _ := OpenGrantStore(path)
	if err := admin.Revoke("test-client"); err != nil {
		t.Fatal(err)
	}
	if err := admin.Grant("test-client", []Scope{ScopeNotesRead}); err != nil {
		t.Fatal(err)
	}
	oldResult := call(t, oldSession, "search_notes", map[string]any{"query": "x"})
	if !oldResult.IsError {
		t.Fatal("regrant revived old server process")
	}
	newSession, _ := newTestSession(t, &fakeApp{}, oldGrants, nil, 0)
	newResult := call(t, newSession, "search_notes", map[string]any{"query": "x"})
	if newResult.IsError {
		t.Fatal("fresh server did not receive new grant")
	}
}

func TestUpdatePreservesOmittedURLAndAttachments(t *testing.T) {
	store, _ := OpenGrantStore(filepath.Join(t.TempDir(), "grants.json"))
	grantAll(t, store, ScopeNotesEdit)
	application := &fakeApp{note: domain.Note{ID: "note", Metadata: domain.NoteMetadata{SourceURL: "https://example.com"}, AttachmentIDs: []string{"attachment"}}}
	session, _ := newTestSession(t, application, store, nil, 0)
	result := call(t, session, "update_note", map[string]any{"id": "note", "title": "new", "content": "body", "type": "idea"})
	if result.IsError {
		t.Fatalf("update failed: %v", result.Content)
	}
	if application.updated.SourceURL != "https://example.com" || len(application.updated.AttachmentIDs) != 1 || application.updated.AttachmentIDs[0] != "attachment" {
		t.Fatalf("omitted values were not preserved: %+v", application.updated)
	}
}

func TestWriteScopesDoNotReadNoteBodiesThroughMutationResults(t *testing.T) {
	store, _ := OpenGrantStore(filepath.Join(t.TempDir(), "grants.json"))
	grantAll(t, store, ScopeNotesEdit, ScopeNotesArchive)
	privateBody := "private-note-body-canary"
	application := &fakeApp{note: domain.Note{ID: "note-id", Title: "private-title-canary", Content: privateBody, Type: domain.NoteTypeIdea, UpdatedAt: time.Now().UTC()}}
	session, _ := newTestSession(t, application, store, nil, 0)

	for _, tc := range []struct {
		name string
		args map[string]any
	}{{"update_note", map[string]any{"id": "note-id", "title": "changed", "content": "new-content", "type": "idea"}}, {"archive_note", map[string]any{"id": "note-id"}}} {
		result := call(t, session, tc.name, tc.args)
		if result.IsError {
			t.Fatalf("%s failed: %v", tc.name, result.Content)
		}
		encoded, _ := json.Marshal(result)
		for _, secret := range []string{privateBody, "private-title-canary"} {
			if strings.Contains(string(encoded), secret) {
				t.Fatalf("%s returned private note data without notes:read: %s", tc.name, encoded)
			}
		}
		if !strings.Contains(string(encoded), "note-id") {
			t.Fatalf("%s response lacks a mutation receipt: %s", tc.name, encoded)
		}
	}
}

func TestMalformedToolArgumentsRejectedBySDK(t *testing.T) {
	store, _ := OpenGrantStore(filepath.Join(t.TempDir(), "grants.json"))
	grantAll(t, store, ScopeNotesRead)
	session, _ := newTestSession(t, &fakeApp{}, store, nil, 0)
	result := call(t, session, "get_note", map[string]any{"id": 42, "unrecognized": "password-canary"})
	if !result.IsError {
		t.Fatal("invalid argument type accepted")
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(strings.ToLower(string(encoded)), "password-canary") {
		t.Fatalf("malformed-argument response echoed sensitive input: %s", encoded)
	}
}

func TestApplicationErrorsDoNotEchoSensitiveMessages(t *testing.T) {
	store, _ := OpenGrantStore(filepath.Join(t.TempDir(), "grants.json"))
	grantAll(t, store, ScopeNotesRead)
	session, _ := newTestSession(t, &fakeApp{err: errors.New("password-canary ciphertext-canary private note body")}, store, nil, 0)
	result := call(t, session, "get_note", map[string]any{"id": "note"})
	if !result.IsError {
		t.Fatal("expected application error")
	}
	encoded, _ := json.Marshal(result)
	for _, secret := range []string{"password-canary", "ciphertext-canary", "private note body"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("tool error echoed sensitive text: %s", encoded)
		}
	}
}

func TestStructuredAuditLoggerNeverWritesSensitiveValues(t *testing.T) {
	var output bytes.Buffer
	auditor := SlogAuditor{Logger: slog.New(slog.NewJSONHandler(&output, nil))}
	auditor.Record(context.Background(), AuditEvent{ClientID: "client", Action: "get_note", Scope: ScopeNotesRead, Outcome: "success", At: time.Now()})
	line := output.String()
	for _, secret := range []string{"note body canary", "password canary", "ciphertext canary", "secret-note-id"} {
		if strings.Contains(line, secret) {
			t.Fatalf("audit logger wrote sensitive value %q: %s", secret, line)
		}
	}
	for _, field := range []string{"client_id", "action", "scope", "outcome", "at"} {
		if !strings.Contains(line, field) {
			t.Fatalf("audit field %q missing: %s", field, line)
		}
	}
}
func TestGrantStorePersistsAndRejectsUnknownScopes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "grants.json")
	store, err := OpenGrantStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Grant("desktop-client", []Scope{ScopeNotesRead}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := OpenGrantStore(path)
	if err != nil || !reloaded.Has("desktop-client", ScopeNotesRead) {
		t.Fatalf("grant not persisted: %v", err)
	}
	if err = reloaded.Grant("desktop-client", []Scope{"notes:all"}); err == nil {
		t.Fatal("unknown scope accepted")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0077 != 0 && os.PathSeparator == '/' {
		t.Fatalf("grant file permissions too open: %v", info.Mode())
	}
}
func TestPromptNeverChangesAuthorization(t *testing.T) {
	store, _ := OpenGrantStore(filepath.Join(t.TempDir(), "grants.json"))
	session, _ := newTestSession(t, &fakeApp{}, store, nil, 0)
	if _, err := session.GetPrompt(context.Background(), &sdk.GetPromptParams{Name: "organise_my_inbox"}); err != nil {
		t.Fatal(err)
	}
	result := call(t, session, "search_notes", map[string]any{"query": "x"})
	if !result.IsError {
		t.Fatal("prompt implicitly granted read access")
	}
}

func TestMeetingPromptUsesOnlyUserSuppliedExcerptAndGrantsNothing(t *testing.T) {
	store, _ := OpenGrantStore(filepath.Join(t.TempDir(), "grants.json"))
	session, _ := newTestSession(t, &fakeApp{}, store, nil, 0)
	meetingText := "Discussed Q4 milestone. Alex owns a draft by Friday."
	prompt, err := session.GetPrompt(context.Background(), &sdk.GetPromptParams{Name: "meeting_action_extraction", Arguments: map[string]string{"meeting_notes": meetingText}})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(prompt)
	if !strings.Contains(string(raw), meetingText) {
		t.Fatal("user-supplied meeting text missing from requested prompt")
	}
	result := call(t, session, "get_note", map[string]any{"id": "secret"})
	if !result.IsError {
		t.Fatal("prompt granted note-read scope")
	}
}

func TestWorkflowPromptArgumentsAreBounded(t *testing.T) {
	store, _ := OpenGrantStore(filepath.Join(t.TempDir(), "grants.json"))
	session, _ := newTestSession(t, &fakeApp{}, store, nil, 0)
	if _, err := session.GetPrompt(context.Background(), &sdk.GetPromptParams{Name: "meeting_action_extraction", Arguments: map[string]string{"meeting_notes": strings.Repeat("x", 64*1024+1)}}); err == nil {
		t.Fatal("oversized meeting transcript accepted")
	}
	if _, err := session.GetPrompt(context.Background(), &sdk.GetPromptParams{Name: "summarise_recent_notes", Arguments: map[string]string{"limit": "51"}}); err == nil {
		t.Fatal("oversized recent-note prompt accepted")
	}
	if _, err := session.GetPrompt(context.Background(), &sdk.GetPromptParams{Name: "weekly_personal_review", Arguments: map[string]string{"created_after": "2026-11-01T00:00:00Z", "created_before": "2026-10-01T00:00:00Z"}}); err == nil {
		t.Fatal("reversed review range accepted")
	}
}
func TestResourceScopesAreEnforced(t *testing.T) {
	store, _ := OpenGrantStore(filepath.Join(t.TempDir(), "grants.json"))
	session, _ := newTestSession(t, &fakeApp{}, store, nil, 0)
	if _, err := session.ReadResource(context.Background(), &sdk.ReadResourceParams{URI: "braindump://stats"}); err == nil {
		t.Fatal("resource read without scope")
	}
}
