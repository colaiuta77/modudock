// HTTP 코어 서버와 공통 요청 처리를 제공한다.
package core

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gofrs/flock"
	"io"
	"io/fs"
	"mime"
	"modudock/internal/web"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Options struct {
	Version, PublicURL string
	WebFS              fs.FS
}
type Server struct {
	db            *sql.DB
	dataDir       string
	options       Options
	mux           *http.ServeMux
	lock          *flock.Flock
	started       time.Time
	logs          *logStore
	loginMu       sync.Mutex
	attempts      map[string]loginAttempt
	passwordSlots chan struct{}
	eventMu       sync.Mutex
	subscribers   map[chan struct{}]struct{}
	done          chan struct{}
	closeOnce     sync.Once
	closeErr      error
	workers       sync.WaitGroup
}
type principal struct {
	User                       User
	SessionHash, CSRF, TokenID string
	Scopes                     map[string]bool
}
type endpoint func(http.ResponseWriter, *http.Request, principal)
type requestIDKey struct{}

func Open(dataDir string, options Options) (*Server, error) {
	if options.PublicURL != "" {
		u, e := url.Parse(options.PublicURL)
		if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
			return nil, errors.New("public-url must be an HTTP origin")
		}
		options.PublicURL = strings.TrimSuffix(options.PublicURL, "/")
	}
	s := &Server{dataDir: dataDir, options: options, mux: http.NewServeMux(), started: time.Now().UTC(), attempts: map[string]loginAttempt{}, passwordSlots: make(chan struct{}, 2), subscribers: map[chan struct{}]struct{}{}, done: make(chan struct{})}
	if e := s.openDB(); e != nil {
		return nil, e
	}
	if e := s.cleanupCredentials(); e != nil {
		s.Close()
		return nil, e
	}
	logs, e := openLogs(s.dataDir)
	if e != nil {
		s.Close()
		return nil, e
	}
	s.logs = logs
	s.registerRoutes()
	s.workers.Add(1)
	go func() { defer s.workers.Done(); s.credentialJanitor() }()
	return s, nil
}
func (s *Server) cleanupCredentials() error {
	now := time.Now().UTC().Format(time.RFC3339)
	if _, e := s.db.Exec("DELETE FROM sessions WHERE expires_at<=?", now); e != nil {
		return e
	}
	_, e := s.db.Exec("DELETE FROM tokens WHERE expires_at<=?", now)
	return e
}
func (s *Server) credentialJanitor() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			if e := s.cleanupCredentials(); e != nil {
				s.logs.logger.Error("Credential cleanup failed", "component", "auth")
			}
		}
	}
}
func (s *Server) Close() error {
	s.closeOnce.Do(func() {
		close(s.done)
		s.workers.Wait()
		if s.logs != nil {
			s.closeErr = s.logs.Close()
		}
		if s.db != nil {
			s.closeErr = errors.Join(s.closeErr, s.db.Close())
		}
		if s.lock != nil {
			s.closeErr = errors.Join(s.closeErr, s.lock.Unlock())
		}
	})
	return s.closeErr
}
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := randomID()
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Cache-Control", "no-store")
		s.mux.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id)))
	})
}
func (s *Server) route(pattern, role, scope string, h endpoint) {
	s.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		p, e := s.authenticate(r)
		if e != nil {
			apiError(w, 401, "unauthorized", "Authentication required")
			return
		}
		if roleRank(p.User.Role) < roleRank(role) || (p.TokenID != "" && scope != "" && !p.Scopes[scope]) {
			apiError(w, 403, "forbidden", "Permission denied")
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" && p.TokenID == "" {
			if !s.validOrigin(r) || !constantEqual(r.Header.Get("X-CSRF-Token"), p.CSRF) {
				apiError(w, 403, "csrf", "Invalid origin or CSRF token")
				return
			}
		}
		h(w, r, p)
	})
}
func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	s.mux.HandleFunc("POST /api/v1/auth/login", s.login)
	s.route("GET /api/v1/auth/me", "viewer", "", func(w http.ResponseWriter, r *http.Request, p principal) {
		writeJSON(w, 200, map[string]any{"user": p.User, "csrf_token": p.CSRF})
	})
	s.route("POST /api/v1/auth/logout", "viewer", "", s.logout)
	s.route("PUT /api/v1/auth/password", "viewer", "users:write", s.changePassword)
	s.route("GET /api/v1/users", "admin", "users:write", s.listUsers)
	s.route("POST /api/v1/users", "admin", "users:write", s.createUser)
	s.route("PUT /api/v1/users/{id}", "admin", "users:write", s.updateUser)
	s.route("DELETE /api/v1/users/{id}", "admin", "users:write", s.deleteUser)
	s.route("GET /api/v1/tokens", "admin", "tokens:write", s.listTokens)
	s.route("POST /api/v1/tokens", "admin", "tokens:write", s.createToken)
	s.route("DELETE /api/v1/tokens/{id}", "admin", "tokens:write", s.deleteToken)
	s.route("GET /api/v1/status", "viewer", "status:read", s.status)
	s.route("GET /api/v1/logs", "operator", "logs:read", s.listLogs)
	s.route("GET /api/v1/events", "viewer", "status:read", s.events)
	s.route("GET /api/v1/openapi.json", "viewer", "", s.openapi)
	s.mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { apiError(w, 404, "not_found", "API route not found") })
	s.mux.Handle("/", web.Handler(s.options.WebFS))
}
func roleRank(role string) int {
	switch role {
	case "admin":
		return 3
	case "operator":
		return 2
	case "viewer":
		return 1
	}
	return 0
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if status != 204 {
		_ = json.NewEncoder(w).Encode(v)
	}
}
func apiError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	mediaType, _, mediaError := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaError != nil || mediaType != "application/json" {
		apiError(w, 415, "unsupported_media_type", "Expected application/json")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	data, e := io.ReadAll(r.Body)
	if e != nil {
		var size *http.MaxBytesError
		if errors.As(e, &size) {
			apiError(w, 413, "body_too_large", "Request body exceeds 64 KiB")
		} else {
			apiError(w, 400, "invalid_json", "Cannot read request body")
		}
		return false
	}
	if len(strings.TrimSpace(string(data))) == 0 || strings.TrimSpace(string(data))[0] != '{' {
		apiError(w, 400, "invalid_json", "Expected JSON object")
		return false
	}
	d := json.NewDecoder(strings.NewReader(string(data)))
	d.DisallowUnknownFields()
	if e = d.Decode(v); e == nil {
		var extra any
		e = d.Decode(&extra)
		if e == io.EOF {
			return true
		}
	}
	apiError(w, 400, "invalid_json", "Malformed JSON or unknown fields")
	return false
}
func (s *Server) validOrigin(r *http.Request) bool {
	expected := s.options.PublicURL
	if expected == "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		expected = scheme + "://" + r.Host
	}
	return r.Header.Get("Origin") == expected
}
func (s *Server) status(w http.ResponseWriter, r *http.Request, p principal) {
	writeJSON(w, 200, map[string]any{"version": s.options.Version, "started_at": s.started.Format(time.RFC3339), "plugins_active": 0, "runs_queued": 0, "runs_running": 0, "runs_succeeded": 0, "runs_failed": 0})
}
func (s *Server) notify() {
	s.eventMu.Lock()
	defer s.eventMu.Unlock()
	for change := range s.subscribers {
		select {
		case change <- struct{}{}:
		default:
		}
	}
}
func (s *Server) events(w http.ResponseWriter, r *http.Request, p principal) {
	f, ok := w.(http.Flusher)
	if !ok {
		apiError(w, 500, "stream_unavailable", "Streaming unavailable")
		return
	}
	change := make(chan struct{}, 1)
	s.eventMu.Lock()
	if len(s.subscribers) >= 128 {
		s.eventMu.Unlock()
		apiError(w, 503, "stream_limit", "Too many event streams")
		return
	}
	s.subscribers[change] = struct{}{}
	s.eventMu.Unlock()
	defer func() { s.eventMu.Lock(); delete(s.subscribers, change); s.eventMu.Unlock() }()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	controller := http.NewResponseController(w)
	_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if _, e := fmt.Fprint(w, "event: snapshot\ndata: {}\n\n"); e != nil {
		return
	}
	f.Flush()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.done:
			return
		case <-change:
			if _, e := s.authenticate(r); e != nil {
				return
			}
			_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, e := fmt.Fprint(w, "event: change\ndata: {}\n\n"); e != nil {
				return
			}
		case <-ticker.C:
			if _, e := s.authenticate(r); e != nil {
				return
			}
			_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, e := fmt.Fprint(w, "event: heartbeat\ndata: {}\n\n"); e != nil {
				return
			}
		}
		f.Flush()
	}
}
func (s *Server) openapi(w http.ResponseWriter, r *http.Request, p principal) {
	writeJSON(w, 200, s.apiDocument())
}
