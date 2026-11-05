package mcp

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Scope names are deliberately fine-grained. Prompts never grant these scopes.
type Scope string

const (
	ScopeMetadataRead     Scope = "metadata:read"
	ScopeNotesRead        Scope = "notes:read"
	ScopeAttachmentsRead  Scope = "attachments:read"
	ScopeNotesCreate      Scope = "notes:create"
	ScopeNotesEdit        Scope = "notes:edit"
	ScopeNotesArchive     Scope = "notes:archive"
	ScopeNotesDelete      Scope = "notes:delete"
	ScopeCollectionsRead  Scope = "collections:read"
	ScopeCollectionsWrite Scope = "collections:write"
)

var allScopes = map[Scope]bool{
	ScopeMetadataRead: true, ScopeNotesRead: true, ScopeAttachmentsRead: true,
	ScopeNotesCreate: true, ScopeNotesEdit: true, ScopeNotesArchive: true,
	ScopeNotesDelete: true, ScopeCollectionsRead: true, ScopeCollectionsWrite: true,
}

type Grant struct {
	Scopes     []Scope   `json:"scopes"`
	Generation string    `json:"generation"`
	Updated    time.Time `json:"updated_at"`
}

type grantFile struct {
	Clients map[string]Grant `json:"clients"`
}

// GrantStore persists per-client permissions. The file contains no note data.
// The user OS account is the trust boundary; same-account processes can access it.
type GrantStore struct {
	mu      sync.RWMutex
	path    string
	clients map[string]Grant
}

func OpenGrantStore(path string) (*GrantStore, error) {
	store := &GrantStore{path: path, clients: map[string]Grant{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read MCP grants: %w", err)
	}
	var file grantFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse MCP grants: %w", err)
	}
	if file.Clients != nil {
		store.clients = file.Clients
	}
	for id, grant := range store.clients {
		if err := validateClientID(id); err != nil {
			return nil, err
		}
		if _, err := normalizeScopes(grant.Scopes); err != nil {
			return nil, fmt.Errorf("client %q: %w", id, err)
		}
	}
	return store, nil
}

func (s *GrantStore) Has(clientID string, scope Scope) bool {
	return s.HasGeneration(clientID, "", scope, false)
}

// HasGeneration also binds a running server process to the grant version it
// observed at startup. A later grant does not revive a revoked old process.
func (s *GrantStore) HasGeneration(clientID, generation string, scope Scope, enforceGeneration bool) bool {
	s.reload()
	s.mu.RLock()
	defer s.mu.RUnlock()
	grant, ok := s.clients[clientID]
	if !ok || (enforceGeneration && grant.Generation != generation) {
		return false
	}
	for _, granted := range grant.Scopes {
		if granted == scope {
			return true
		}
	}
	return false
}

func (s *GrantStore) Generation(clientID string) string {
	s.reload()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.clients[clientID].Generation
}

func (s *GrantStore) reload() {
	// Reload on every authorization so grant/revoke commands in another process
	// take effect without restarting connected stdio servers.
	if data, err := os.ReadFile(s.path); err == nil {
		var file grantFile
		if json.Unmarshal(data, &file) != nil || file.Clients == nil {
			file.Clients = map[string]Grant{}
		}
		s.mu.Lock()
		s.clients = file.Clients
		s.mu.Unlock()
	} else if errors.Is(err, os.ErrNotExist) {
		s.mu.Lock()
		s.clients = map[string]Grant{}
		s.mu.Unlock()
	} else {
		s.mu.Lock()
		s.clients = map[string]Grant{}
		s.mu.Unlock()
	}
}

func (s *GrantStore) Grant(clientID string, scopes []Scope) error {
	if err := validateClientID(clientID); err != nil {
		return err
	}
	normalized, err := normalizeScopes(scopes)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	generationBytes := make([]byte, 16)
	if _, err := rand.Read(generationBytes); err != nil {
		return fmt.Errorf("generate permission grant generation: %w", err)
	}
	s.clients[clientID] = Grant{Scopes: normalized, Generation: hex.EncodeToString(generationBytes), Updated: time.Now().UTC()}
	return s.saveLocked()
}

func (s *GrantStore) Revoke(clientID string) error {
	if err := validateClientID(clientID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.clients, clientID)
	return s.saveLocked()
}

func (s *GrantStore) Scopes(clientID string) []Scope {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Scope(nil), s.clients[clientID].Scopes...)
}

func (s *GrantStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return fmt.Errorf("create MCP grant directory: %w", err)
	}
	data, err := json.MarshalIndent(grantFile{Clients: s.clients}, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".mcp-grants-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, s.path); err != nil {
		return fmt.Errorf("replace MCP grants: %w", err)
	}
	if dir, err := os.Open(filepath.Dir(s.path)); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}

func normalizeScopes(scopes []Scope) ([]Scope, error) {
	set := map[Scope]bool{}
	for _, scope := range scopes {
		if !allScopes[scope] {
			return nil, fmt.Errorf("unknown permission scope %q", scope)
		}
		set[scope] = true
	}
	result := make([]Scope, 0, len(set))
	for scope := range set {
		result = append(result, scope)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result, nil
}

func validateClientID(id string) error {
	id = strings.TrimSpace(id)
	if len(id) < 1 || len(id) > 128 {
		return fmt.Errorf("client ID must be 1 to 128 characters")
	}
	for _, r := range id {
		if !(r == '-' || r == '_' || r == '.' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return fmt.Errorf("client ID contains invalid characters")
		}
	}
	return nil
}
