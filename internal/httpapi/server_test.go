package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"example.com/local-capture/internal/app"
	"example.com/local-capture/internal/domain"
)

type fakeApplication struct {
	Application
	mu          sync.Mutex
	notes       map[string]domain.Note
	collections map[string]domain.Collection
	tags        map[string]domain.Tag
	attachments map[string][]byte
	reminders   map[string]domain.Reminder
	next        int
}

func newFakeApplication() *fakeApplication {
	return &fakeApplication{notes: map[string]domain.Note{}, collections: map[string]domain.Collection{}, tags: map[string]domain.Tag{}, attachments: map[string][]byte{}, reminders: map[string]domain.Reminder{}}
}
func (f *fakeApplication) CreateNote(_ context.Context, in app.CreateNoteInput) (domain.Note, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next++
	now := time.Now().UTC()
	n, err := domain.NewNote(fmt.Sprintf("note-%d", f.next), in.Title, in.Content, in.Type, now)
	if err != nil {
		return domain.Note{}, err
	}
	n.CollectionID = in.CollectionID
	n.TagIDs = append([]string(nil), in.TagIDs...)
	n.Pinned = in.Pinned
	n.Metadata.SourceURL = in.SourceURL
	n.AttachmentIDs = append([]string(nil), in.AttachmentIDs...)
	n.Reminder = in.Reminder
	f.notes[n.ID] = n
	return n, nil
}
func (f *fakeApplication) GetNote(_ context.Context, id string) (domain.Note, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, ok := f.notes[id]
	if !ok {
		return domain.Note{}, app.ErrNotFound
	}
	return n, nil
}
func (f *fakeApplication) UpdateNote(_ context.Context, id string, in app.UpdateNoteInput) (domain.Note, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, ok := f.notes[id]
	if !ok {
		return domain.Note{}, app.ErrNotFound
	}
	if err := n.Update(in.Title, in.Content, in.Type, in.SourceURL, in.Reminder, in.AttachmentIDs, time.Now().UTC()); err != nil {
		return domain.Note{}, err
	}
	if in.CollectionID != nil {
		n.CollectionID = *in.CollectionID
	}
	if in.TagIDs != nil {
		n.TagIDs = *in.TagIDs
	}
	if in.Pinned != nil {
		n.Pinned = *in.Pinned
	}
	f.notes[id] = n
	return n, nil
}
func (f *fakeApplication) DeleteNote(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.notes[id]; !ok {
		return app.ErrNotFound
	}
	delete(f.notes, id)
	return nil
}
func (f *fakeApplication) ArchiveNote(_ context.Context, id string) (domain.Note, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, ok := f.notes[id]
	if !ok {
		return domain.Note{}, app.ErrNotFound
	}
	n.Archive(time.Now().UTC())
	f.notes[id] = n
	return n, nil
}
func (f *fakeApplication) RestoreNote(_ context.Context, id string) (domain.Note, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, ok := f.notes[id]
	if !ok {
		return domain.Note{}, app.ErrNotFound
	}
	n.Restore(time.Now().UTC())
	f.notes[id] = n
	return n, nil
}
func (f *fakeApplication) ListNotes(_ context.Context, _ app.ListNotesQuery) ([]domain.Note, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]domain.Note, 0, len(f.notes))
	for _, n := range f.notes {
		out = append(out, n)
	}
	return out, nil
}

func (f *fakeApplication) CreateCollection(_ context.Context, name string) (domain.Collection, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next++
	now := time.Now().UTC()
	c, err := domain.NewCollection(fmt.Sprintf("collection-%d", f.next), name, now)
	if err == nil {
		f.collections[c.ID] = c
	}
	return c, err
}
func (f *fakeApplication) ListCollections(context.Context) ([]domain.Collection, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []domain.Collection{}
	for _, v := range f.collections {
		out = append(out, v)
	}
	return out, nil
}
func (f *fakeApplication) UpdateCollection(_ context.Context, id, name string) (domain.Collection, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.collections[id]
	if !ok {
		return domain.Collection{}, app.ErrNotFound
	}
	v.Name = name
	v.UpdatedAt = time.Now().UTC()
	f.collections[id] = v
	return v, nil
}
func (f *fakeApplication) DeleteCollection(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.collections[id]; !ok {
		return app.ErrNotFound
	}
	delete(f.collections, id)
	return nil
}
func (f *fakeApplication) CreateTag(_ context.Context, name string) (domain.Tag, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next++
	v, err := domain.NewTag(fmt.Sprintf("tag-%d", f.next), name, time.Now().UTC())
	if err == nil {
		f.tags[v.ID] = v
	}
	return v, err
}
func (f *fakeApplication) ListTags(context.Context) ([]domain.Tag, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []domain.Tag{}
	for _, v := range f.tags {
		out = append(out, v)
	}
	return out, nil
}
func (f *fakeApplication) AssignTags(_ context.Context, id string, ids []string) (domain.Note, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, ok := f.notes[id]
	if !ok {
		return domain.Note{}, app.ErrNotFound
	}
	for _, tagID := range ids {
		if _, ok := f.tags[tagID]; !ok {
			return domain.Note{}, app.ErrInvalidRelationship
		}
	}
	n.TagIDs = append([]string(nil), ids...)
	f.notes[id] = n
	return n, nil
}
func (f *fakeApplication) RemoveTag(_ context.Context, id, tagID string) (domain.Note, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, ok := f.notes[id]
	if !ok {
		return domain.Note{}, app.ErrNotFound
	}
	for i, v := range n.TagIDs {
		if v == tagID {
			n.TagIDs = append(n.TagIDs[:i], n.TagIDs[i+1:]...)
			break
		}
	}
	f.notes[id] = n
	return n, nil
}
func (f *fakeApplication) UploadAttachment(_ context.Context, noteID, name, mediaType string, reader io.Reader) (domain.Attachment, error) {
	b, err := io.ReadAll(reader)
	if err != nil {
		return domain.Attachment{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if noteID != "" {
		if _, ok := f.notes[noteID]; !ok {
			return domain.Attachment{}, app.ErrNotFound
		}
	}
	f.next++
	id := fmt.Sprintf("attachment-%d", f.next)
	f.attachments[id] = b
	return domain.Attachment{ID: id, FileName: name, MediaType: mediaType, Size: int64(len(b)), CreatedAt: time.Now().UTC()}, nil
}
func (f *fakeApplication) DownloadAttachment(_ context.Context, id string) (domain.Attachment, io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.attachments[id]
	if !ok {
		return domain.Attachment{}, nil, app.ErrNotFound
	}
	return domain.Attachment{ID: id, FileName: "capture.png", MediaType: "image/png", Size: int64(len(b))}, io.NopCloser(strings.NewReader(string(b))), nil
}
func (f *fakeApplication) DeleteAttachment(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.attachments[id]; !ok {
		return app.ErrNotFound
	}
	delete(f.attachments, id)
	return nil
}
func (f *fakeApplication) CreateReminder(_ context.Context, noteID string, due time.Time) (domain.Reminder, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.notes[noteID]; !ok {
		return domain.Reminder{}, app.ErrNotFound
	}
	r := domain.Reminder{ID: noteID, NoteID: noteID, DueAt: due}
	f.reminders[r.ID] = r
	return r, nil
}
func (f *fakeApplication) UpdateReminder(_ context.Context, id string, due time.Time) (domain.Reminder, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.reminders[id]
	if !ok {
		return domain.Reminder{}, app.ErrNotFound
	}
	r.DueAt = due
	f.reminders[id] = r
	return r, nil
}
func (f *fakeApplication) ListReminders(context.Context) ([]domain.Reminder, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []domain.Reminder{}
	for _, v := range f.reminders {
		out = append(out, v)
	}
	return out, nil
}
func (f *fakeApplication) ListDueReminders(context.Context) ([]domain.Reminder, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	out := []domain.Reminder{}
	for _, r := range f.reminders {
		if !r.IsCompleted() && !r.EffectiveDueAt().After(now) {
			out = append(out, r)
		}
	}
	return out, nil
}
func (f *fakeApplication) ListReminderHistory(context.Context, string) ([]domain.ReminderEvent, error) {
	return []domain.ReminderEvent{}, nil
}
func (f *fakeApplication) SnoozeReminder(_ context.Context, id string, until time.Time) (domain.Reminder, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.reminders[id]
	if !ok {
		return domain.Reminder{}, app.ErrNotFound
	}
	r.SnoozedUntil = &until
	f.reminders[id] = r
	return r, nil
}
func (f *fakeApplication) CompleteReminder(_ context.Context, id string) (domain.Reminder, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.reminders[id]
	if !ok {
		return domain.Reminder{}, app.ErrNotFound
	}
	now := time.Now().UTC()
	r.CompletedAt = &now
	f.reminders[id] = r
	return r, nil
}

func allPermissions() map[Permission]bool {
	m := map[Permission]bool{}
	for _, p := range []Permission{PermissionNotesRead, PermissionNotesWrite, PermissionNotesArchive, PermissionNotesDelete, PermissionCollectionsRead, PermissionCollectionsWrite, PermissionCollectionsDelete, PermissionTagsRead, PermissionTagsWrite, PermissionAttachmentsRead, PermissionAttachmentsWrite, PermissionAttachmentsDelete, PermissionRemindersRead, PermissionRemindersWrite} {
		m[p] = true
	}
	return m
}
func startTestServer(t *testing.T, a Application, permissions map[Permission]bool, maxBytes int64) (*Server, *http.Client, string) {
	t.Helper()
	token, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewServer(Config{Address: "127.0.0.1:0", Token: token, AllowedOrigins: []string{"http://localhost:3000"}, Permissions: permissions, MaxRequestBytes: maxBytes, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}, a)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.Serve() }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = s.Shutdown(ctx)
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("serve: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("server did not stop")
		}
	})
	return s, &http.Client{Timeout: 3 * time.Second}, "http://" + s.Addr().String()
}

func apiRequest(t *testing.T, client *http.Client, base, method, path, token, origin, body string, host string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, base+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if host != "" {
		req.Host = host
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestAuthenticationOriginHostAndPermissionFailures(t *testing.T) {
	a := newFakeApplication()
	s, c, base := startTestServer(t, a, allPermissions(), 1024)
	token := s.config.Token
	resp := apiRequest(t, c, base, http.MethodGet, "/api/v1/notes", "wrong", "", "", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("auth status=%d", resp.StatusCode)
	}
	resp = apiRequest(t, c, base, http.MethodGet, "/api/v1/notes", token, "http://evil.example", "", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("origin status=%d", resp.StatusCode)
	}
	resp = apiRequest(t, c, base, http.MethodGet, "/api/v1/notes", token, "", "", "attacker.example:80")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("host status=%d", resp.StatusCode)
	}
	denied := map[Permission]bool{PermissionNotesRead: true}
	s2, c2, base2 := startTestServer(t, a, denied, 1024)
	resp = apiRequest(t, c2, base2, http.MethodPost, "/api/v1/notes", s2.config.Token, "", "{\"title\":\"x\",\"content\":\"\",\"type\":\"thought\"}", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("permission status=%d", resp.StatusCode)
	}
}

func TestMalformedOversizedAndMissingResourceResponses(t *testing.T) {
	s, c, base := startTestServer(t, newFakeApplication(), allPermissions(), 100)
	token := s.config.Token
	resp := apiRequest(t, c, base, http.MethodPost, "/api/v1/notes", token, "", "{bad json", "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed status=%d", resp.StatusCode)
	}
	resp.Body.Close()
	large := `{"title":"` + strings.Repeat("a", 300) + `","content":"","type":"thought"}`
	resp = apiRequest(t, c, base, http.MethodPost, "/api/v1/notes", token, "", large, "")
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("large status=%d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = apiRequest(t, c, base, http.MethodGet, "/api/v1/notes/missing", token, "", "", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing status=%d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestNoteCRUDArchiveRestoreAndDelete(t *testing.T) {
	s, c, base := startTestServer(t, newFakeApplication(), allPermissions(), 4096)
	token := s.config.Token
	body := `{"title":"Example","content":"body","type":"idea","tag_ids":[]}`
	resp := apiRequest(t, c, base, http.MethodPost, "/api/v1/notes", token, "", body, "")
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create status=%d body=%s", resp.StatusCode, b)
	}
	var note domain.Note
	if err := json.NewDecoder(resp.Body).Decode(&note); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if note.ID == "" || note.Title != "Example" {
		t.Fatalf("bad note response: %+v", note)
	}
	resp = apiRequest(t, c, base, http.MethodGet, "/api/v1/notes/"+url.PathEscape(note.ID), token, "", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get status=%d", resp.StatusCode)
	}
	resp.Body.Close()
	update := `{"title":"Updated","content":"new","type":"thought"}`
	resp = apiRequest(t, c, base, http.MethodPut, "/api/v1/notes/"+note.ID, token, "", update, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update status=%d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = apiRequest(t, c, base, http.MethodPost, "/api/v1/notes/"+note.ID+"/archive", token, "", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("archive status=%d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = apiRequest(t, c, base, http.MethodPost, "/api/v1/notes/"+note.ID+"/restore", token, "", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("restore status=%d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = apiRequest(t, c, base, http.MethodDelete, "/api/v1/notes/"+note.ID, token, "", "", "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status=%d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestConcurrentRequests(t *testing.T) {
	s, c, base := startTestServer(t, newFakeApplication(), allPermissions(), 4096)
	token := s.config.Token
	const n = 20
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := fmt.Sprintf(`{"title":"note-%d","content":"","type":"thought"}`, i)
			resp, err := http.NewRequest(http.MethodPost, base+"/api/v1/notes", strings.NewReader(body))
			if err != nil {
				errs <- err
				return
			}
			resp.Header.Set("Authorization", "Bearer "+token)
			resp.Header.Set("Content-Type", "application/json")
			response, err := c.Do(resp)
			if err != nil {
				errs <- err
				return
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusCreated {
				errs <- fmt.Errorf("status %d", response.StatusCode)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestCollectionTagAttachmentAndReminderRoutes(t *testing.T) {
	s, c, base := startTestServer(t, newFakeApplication(), allPermissions(), 4096)
	token := s.config.Token
	resp := apiRequest(t, c, base, http.MethodPost, "/api/v1/notes", token, "", `{"title":"Resource note","content":"","type":"reference"}`, "")
	var note domain.Note
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("note create: %d", resp.StatusCode)
	}
	_ = json.NewDecoder(resp.Body).Decode(&note)
	resp.Body.Close()
	resp = apiRequest(t, c, base, http.MethodPost, "/api/v1/collections", token, "", `{"name":"Reading"}`, "")
	var collection domain.Collection
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("collection create: %d", resp.StatusCode)
	}
	_ = json.NewDecoder(resp.Body).Decode(&collection)
	resp.Body.Close()
	for _, tc := range []struct {
		method, path, body string
		want               int
	}{{http.MethodGet, "/api/v1/collections", "", http.StatusOK}, {http.MethodPut, "/api/v1/collections/" + collection.ID, `{"name":"Research"}`, http.StatusOK}, {http.MethodDelete, "/api/v1/collections/" + collection.ID, "", http.StatusNoContent}} {
		resp = apiRequest(t, c, base, tc.method, tc.path, token, "", tc.body, "")
		if resp.StatusCode != tc.want {
			t.Fatalf("%s %s: %d", tc.method, tc.path, resp.StatusCode)
		}
		resp.Body.Close()
	}
	resp = apiRequest(t, c, base, http.MethodPost, "/api/v1/tags", token, "", `{"name":"Useful"}`, "")
	var tag domain.Tag
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("tag create: %d", resp.StatusCode)
	}
	_ = json.NewDecoder(resp.Body).Decode(&tag)
	resp.Body.Close()
	resp = apiRequest(t, c, base, http.MethodGet, "/api/v1/tags", token, "", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tag list: %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = apiRequest(t, c, base, http.MethodPost, "/api/v1/tags/assign", token, "", fmt.Sprintf(`{"note_id":%q,"tag_ids":[%q]}`, note.ID, tag.ID), "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("assign tag: %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = apiRequest(t, c, base, http.MethodDelete, "/api/v1/notes/"+note.ID+"/tags/"+tag.ID, token, "", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("remove tag: %d", resp.StatusCode)
	}
	resp.Body.Close()
	request, _ := http.NewRequest(http.MethodPost, base+"/api/v1/attachments?note_id="+note.ID, strings.NewReader("image-data"))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-Filename", "capture.png")
	request.Header.Set("Content-Type", "image/png")
	resp, err := c.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var attachment domain.Attachment
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("attachment upload: %d", resp.StatusCode)
	}
	_ = json.NewDecoder(resp.Body).Decode(&attachment)
	resp.Body.Close()
	resp = apiRequest(t, c, base, http.MethodGet, "/api/v1/attachments/"+attachment.ID, token, "", "", "")
	download, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || string(download) != "image-data" || resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("attachment download: %q %v", download, err)
	}
	resp = apiRequest(t, c, base, http.MethodDelete, "/api/v1/attachments/"+attachment.ID, token, "", "", "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("attachment delete: %d", resp.StatusCode)
	}
	resp.Body.Close()
	due := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	resp = apiRequest(t, c, base, http.MethodPost, "/api/v1/reminders", token, "", fmt.Sprintf(`{"note_id":%q,"due_at":%q}`, note.ID, due), "")
	var reminder domain.Reminder
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("reminder create: %d", resp.StatusCode)
	}
	_ = json.NewDecoder(resp.Body).Decode(&reminder)
	resp.Body.Close()
	resp = apiRequest(t, c, base, http.MethodGet, "/api/v1/reminders", token, "", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reminder list: %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = apiRequest(t, c, base, http.MethodGet, "/api/v1/reminders/due", token, "", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("due reminder list: %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = apiRequest(t, c, base, http.MethodPut, "/api/v1/reminders/"+reminder.ID, token, "", fmt.Sprintf(`{"due_at":%q}`, due), "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reminder update: %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = apiRequest(t, c, base, http.MethodPost, "/api/v1/reminders/"+reminder.ID+"/snooze", token, "", fmt.Sprintf(`{"until":%q}`, time.Now().UTC().Add(2*time.Hour).Format(time.RFC3339)), "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reminder snooze: %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = apiRequest(t, c, base, http.MethodPost, "/api/v1/reminders/"+reminder.ID+"/snooze", token, "", `{"preset":"tomorrow","timezone":"America/New_York"}`, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("preset reminder snooze: %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = apiRequest(t, c, base, http.MethodGet, "/api/v1/reminders/"+reminder.ID+"/history", token, "", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reminder history: %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = apiRequest(t, c, base, http.MethodPost, "/api/v1/reminders/"+reminder.ID+"/complete", token, "", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reminder complete: %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestGracefulShutdownAndTokenPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "api-token")
	first, err := LoadOrCreateToken(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadOrCreateToken(path)
	if err != nil || first != second {
		t.Fatalf("token not stable: %v", err)
	}
	s, _, _ := startTestServer(t, newFakeApplication(), allPermissions(), 1024)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestServerRejectsNonLoopbackBindingAndInvalidToken(t *testing.T) {
	if _, err := NewServer(Config{Address: "0.0.0.0:0", Token: strings.Repeat("x", 32)}, newFakeApplication()); err == nil {
		t.Fatal("expected non-loopback bind rejected")
	}
	if _, err := NewServer(Config{Address: "127.0.0.1:0", Token: "short"}, newFakeApplication()); err == nil {
		t.Fatal("expected short token rejected")
	}
	if _, err := NewServer(Config{Address: "127.0.0.1:0", Token: strings.Repeat("x", 32), AllowedOrigins: []string{"*"}}, newFakeApplication()); err == nil {
		t.Fatal("expected invalid wildcard origin rejected")
	}
}

func TestRequestLogsDoNotContainNoteContent(t *testing.T) {
	var logs bytes.Buffer
	token, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(Config{Address: "127.0.0.1:0", Token: token, Permissions: allPermissions(), Logger: slog.New(slog.NewJSONHandler(&logs, nil))}, newFakeApplication())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve() }()
	client := &http.Client{Timeout: time.Second}
	body := `{"title":"private-title","content":"private-body","type":"thought"}`
	resp := apiRequest(t, client, "http://"+server.Addr().String(), http.MethodPost, "/api/v1/notes", token, "", body, "")
	resp.Body.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	<-done
	if strings.Contains(logs.String(), "private-title") || strings.Contains(logs.String(), "private-body") {
		t.Fatalf("private content found in logs: %s", logs.String())
	}
}
