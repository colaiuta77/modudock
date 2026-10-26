// 회전 JSONL 로그와 감사 이벤트 및 제한된 로그 검색을 제공한다.
package core

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const logMaxBytes int64 = 5 << 20

type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Component string `json:"component"`
	PluginID  string `json:"plugin_id"`
	TaskID    string `json:"task_id"`
	RunID     string `json:"run_id"`
	UserID    string `json:"user_id"`
	RequestID string `json:"request_id"`
}
type logStore struct {
	mu     sync.Mutex
	dir    string
	file   *os.File
	size   int64
	logger *slog.Logger
}

func openLogs(dataDir string, operational io.Writer) (*logStore, error) {
	dir := filepath.Join(dataDir, "logs")
	if e := os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	l := &logStore{dir: dir}
	if e := l.open(); e != nil {
		return nil, e
	}
	writers := []io.Writer{l, os.Stdout}
	if operational != nil {
		writers = append(writers, operational)
	}
	l.logger = slog.New(slog.NewJSONHandler(io.MultiWriter(writers...), &slog.HandlerOptions{ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
		if a.Key == slog.TimeKey {
			a.Key = "timestamp"
			a.Value = slog.TimeValue(a.Value.Time().UTC())
		}
		if a.Key == slog.MessageKey {
			a.Key = "message"
		}
		return a
	}}))
	return l, nil
}
func (l *logStore) open() error {
	f, e := os.OpenFile(filepath.Join(l.dir, "core.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if e != nil {
		return e
	}
	info, e := f.Stat()
	if e != nil {
		f.Close()
		return e
	}
	l.file = f
	l.size = info.Size()
	return nil
}
func (l *logStore) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.size+int64(len(p)) > logMaxBytes {
		if e := l.file.Close(); e != nil {
			return 0, e
		}
		oldest := filepath.Join(l.dir, "core.3.jsonl")
		if e := os.Remove(oldest); e != nil && !os.IsNotExist(e) {
			return 0, e
		}
		for i := 2; i >= 0; i-- {
			old := filepath.Join(l.dir, "core.jsonl")
			if i > 0 {
				old = filepath.Join(l.dir, fmt.Sprintf("core.%d.jsonl", i))
			}
			next := filepath.Join(l.dir, fmt.Sprintf("core.%d.jsonl", i+1))
			if e := os.Rename(old, next); e != nil && !os.IsNotExist(e) {
				return 0, e
			}
		}
		if e := l.open(); e != nil {
			return 0, e
		}
	}
	n, e := l.file.Write(p)
	l.size += int64(n)
	return n, e
}
func (l *logStore) Close() error { l.mu.Lock(); defer l.mu.Unlock(); return l.file.Close() }
func (s *Server) log(entry LogEntry) {
	level := slog.LevelInfo
	if entry.Level != "" {
		_ = level.UnmarshalText([]byte(entry.Level))
	}
	s.logs.logger.LogAttrs(nil, level, entry.Message, slog.String("component", entry.Component), slog.String("plugin_id", entry.PluginID), slog.String("task_id", entry.TaskID), slog.String("run_id", entry.RunID), slog.String("user_id", entry.UserID), slog.String("request_id", entry.RequestID))
}
func (s *Server) audit(r *http.Request, action, userID string) {
	requestID, _ := r.Context().Value(requestIDKey{}).(string)
	_, e := s.db.ExecContext(r.Context(), "INSERT INTO audit(id,timestamp,action,user_id,request_id) VALUES(?,?,?,?,?)", randomID(), time.Now().UTC().Format(time.RFC3339), action, userID, requestID)
	if e != nil {
		s.logs.logger.Error("Audit write failed", "component", "auth", "request_id", requestID)
	}
	s.log(LogEntry{Message: action, Component: "auth", UserID: userID, RequestID: requestID})
}
func (s *Server) listLogs(w http.ResponseWriter, r *http.Request, p principal) {
	limit, offset, ok := pagination(w, r)
	if !ok {
		return
	}
	query := r.URL.Query()
	runID := query.Get("run_id")
	if p.User.Role == "operator" && runID == "" {
		apiError(w, 403, "forbidden", "Operators must select a run")
		return
	}
	level := strings.ToUpper(query.Get("level"))
	if level != "" && level != "DEBUG" && level != "INFO" && level != "WARN" && level != "ERROR" {
		apiError(w, 400, "invalid_input", "Invalid log level")
		return
	}
	if len(query.Get("q")) > 200 || len(runID) > 128 {
		apiError(w, 400, "invalid_input", "Log filter is too long")
		return
	}
	entries, e := s.logs.read()
	if e != nil {
		internalError(w)
		return
	}
	items := []LogEntry{}
	total := 0
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if runID != "" && entry.RunID != runID || level != "" && entry.Level != level || !strings.Contains(strings.ToLower(entry.Message), strings.ToLower(query.Get("q"))) {
			continue
		}
		if total >= offset && len(items) < limit {
			items = append(items, entry)
		}
		total++
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": total})
}
func (l *logStore) read() ([]LogEntry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entries := []LogEntry{}
	for i := 3; i >= 0; i-- {
		name := "core.jsonl"
		if i > 0 {
			name = fmt.Sprintf("core.%d.jsonl", i)
		}
		f, e := os.Open(filepath.Join(l.dir, name))
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return nil, e
		}
		scanner := bufio.NewScanner(io.LimitReader(f, logMaxBytes))
		scanner.Buffer(make([]byte, 4096), 64<<10)
		for scanner.Scan() {
			var entry LogEntry
			if e = json.Unmarshal(scanner.Bytes(), &entry); e != nil {
				f.Close()
				return nil, e
			}
			entries = append(entries, entry)
		}
		e = scanner.Err()
		f.Close()
		if e != nil {
			return nil, e
		}
	}
	return entries, nil
}
