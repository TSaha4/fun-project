package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Permission string

const (
	PermissionNotesRead         Permission = "notes.read"
	PermissionNotesWrite        Permission = "notes.write"
	PermissionNotesArchive      Permission = "notes.archive"
	PermissionNotesDelete       Permission = "notes.delete"
	PermissionCollectionsRead   Permission = "collections.read"
	PermissionCollectionsWrite  Permission = "collections.write"
	PermissionCollectionsDelete Permission = "collections.delete"
	PermissionTagsRead          Permission = "tags.read"
	PermissionTagsWrite         Permission = "tags.write"
	PermissionAttachmentsRead   Permission = "attachments.read"
	PermissionAttachmentsWrite  Permission = "attachments.write"
	PermissionAttachmentsDelete Permission = "attachments.delete"
	PermissionRemindersRead     Permission = "reminders.read"
	PermissionRemindersWrite    Permission = "reminders.write"
)

type Config struct {
	Address         string
	Token           string
	AllowedOrigins  []string
	Permissions     map[Permission]bool
	MaxRequestBytes int64
	Logger          *slog.Logger
}

type Server struct {
	config    Config
	listener  net.Listener
	http      *http.Server
	closeOnce sync.Once
}

func GenerateToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate API token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

// LoadOrCreateToken stores a random token in a caller-provided private config
// directory. The directory's OS permissions/ACL are the caller's responsibility.
func LoadOrCreateToken(path string) (string, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return "", fmt.Errorf("read API token: %w", readErr)
		}
		token := strings.TrimSpace(string(data))
		if len(token) < 32 {
			return "", errors.New("stored API token is invalid")
		}
		return token, nil
	}
	if err != nil {
		return "", fmt.Errorf("create API token: %w", err)
	}
	ok := false
	defer func() {
		_ = f.Close()
		if !ok {
			_ = os.Remove(path)
		}
	}()
	token, err := GenerateToken()
	if err != nil {
		return "", err
	}
	if _, err = f.WriteString(token + "\n"); err != nil {
		return "", fmt.Errorf("write API token: %w", err)
	}
	if err = f.Sync(); err != nil {
		return "", fmt.Errorf("flush API token: %w", err)
	}
	if err = f.Close(); err != nil {
		return "", fmt.Errorf("close API token: %w", err)
	}
	ok = true
	return token, nil
}

// NewServer binds immediately and refuses non-loopback addresses. The caller
// owns the returned server and should call Shutdown during process shutdown.
func NewServer(cfg Config, application Application) (*Server, error) {
	if application == nil {
		return nil, errors.New("application service is required")
	}
	if cfg.Address == "" {
		cfg.Address = "127.0.0.1:0"
	}
	host, _, err := net.SplitHostPort(cfg.Address)
	if err != nil {
		return nil, fmt.Errorf("invalid API address: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return nil, errors.New("API must bind to a literal loopback IP address")
	}
	if len(cfg.Token) < 32 {
		return nil, errors.New("API token must contain at least 32 characters")
	}
	if cfg.MaxRequestBytes <= 0 {
		cfg.MaxRequestBytes = 1 << 20
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.NewJSONHandler(os.Stderr, nil))
	}
	for _, origin := range cfg.AllowedOrigins {
		if !validOrigin(origin) {
			return nil, fmt.Errorf("invalid allowed origin %q", origin)
		}
	}
	listener, err := net.Listen("tcp", cfg.Address)
	if err != nil {
		return nil, fmt.Errorf("listen for local API: %w", err)
	}
	if addr, ok := listener.Addr().(*net.TCPAddr); !ok || !addr.IP.IsLoopback() {
		listener.Close()
		return nil, errors.New("listener is not loopback-only")
	}
	s := &Server{config: cfg, listener: listener}
	s.http = &http.Server{Handler: s.middleware(s.routes(application)), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	return s, nil
}

func (s *Server) Addr() net.Addr { return s.listener.Addr() }

func (s *Server) Serve() error {
	err := s.http.Serve(s.listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) Shutdown(ctx context.Context) error {
	var err error
	s.closeOnce.Do(func() {
		err = s.http.Shutdown(ctx)
		if err != nil {
			_ = s.listener.Close()
		}
	})
	return err
}

func (s *Server) Run(ctx context.Context) error {
	done := make(chan error, 1)
	go func() { done <- s.Serve() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Shutdown(shutdownCtx); err != nil {
		return err
	}
	return <-done
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		requestID := newRequestID()
		w.Header().Set("X-Request-ID", requestID)
		status := http.StatusOK
		defer func() {
			s.config.Logger.Info("local API request", "request_id", requestID, "method", r.Method, "status", status, "duration_ms", time.Since(started).Milliseconds())
		}()
		if !s.validHost(r.Host) {
			status = http.StatusForbidden
			writeError(w, status, "invalid_host", "Request host is not allowed", requestID)
			return
		}
		origin := r.Header.Get("Origin")
		if origin != "" {
			if !s.validOrigin(origin) {
				status = http.StatusForbidden
				writeError(w, status, "invalid_origin", "Request origin is not allowed", requestID)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			if origin == "" {
				status = http.StatusForbidden
				writeError(w, status, "invalid_origin", "Origin is required for preflight", requestID)
				return
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Filename")
			w.Header().Set("Access-Control-Max-Age", "300")
			status = http.StatusNoContent
			w.WriteHeader(status)
			return
		}
		if r.URL.RawQuery != "" && strings.Contains(strings.ToLower(r.URL.RawQuery), "token=") {
			status = http.StatusBadRequest
			writeError(w, status, "invalid_request", "Credentials must not be sent in the URL", requestID)
			return
		}
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if provided == r.Header.Get("Authorization") || subtle.ConstantTimeCompare([]byte(provided), []byte(s.config.Token)) != 1 {
			status = http.StatusUnauthorized
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeError(w, status, "unauthorized", "Valid bearer token required", requestID)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		r.Body = http.MaxBytesReader(w, r.Body, s.config.MaxRequestBytes)
		if r.ContentLength > s.config.MaxRequestBytes {
			status = http.StatusRequestEntityTooLarge
			writeError(w, status, "request_too_large", "Request body exceeds the configured limit", requestID)
			return
		}
		next.ServeHTTP(&statusWriter{ResponseWriter: w, status: &status}, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, requestID)))
	})
}

func (s *Server) validHost(value string) bool {
	host, port, err := net.SplitHostPort(value)
	if err != nil {
		return false
	}
	listener := s.listener.Addr().(*net.TCPAddr)
	if port != strconv.Itoa(listener.Port) {
		return false
	}
	if strings.EqualFold(strings.TrimSuffix(host, "."), "localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

func (s *Server) validOrigin(raw string) bool {
	if !validOrigin(raw) {
		return false
	}
	for _, allowed := range s.config.AllowedOrigins {
		if raw == allowed {
			return true
		}
	}
	return false
}

type statusWriter struct {
	http.ResponseWriter
	status *int
	wrote  bool
}

func (w *statusWriter) WriteHeader(code int) {
	if w.wrote {
		return
	}
	w.wrote = true
	*w.status = code
	w.ResponseWriter.WriteHeader(code)
}
func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

type requestIDKey struct{}

func RequestID(ctx context.Context) string { v, _ := ctx.Value(requestIDKey{}).(string); return v }
func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "unavailable"
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}
func validOrigin(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme != "" && u.Host != "" && u.User == nil && u.Path == "" && u.RawQuery == "" && u.Fragment == "" && raw != "null"
}
