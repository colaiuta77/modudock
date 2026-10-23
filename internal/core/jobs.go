// SQLite 영속 작업 큐와 취소·재시도·백그라운드 생명주기를 관리한다.
package core

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

const jobConcurrency = 2
const maxQueuedJobs = 1000

var errConflict = errors.New("conflicting idempotency key")
var errPluginDisabled = errors.New("plugin is disabled")
var errQueueFull = errors.New("queue capacity exceeded")
var errInvalidTask = errors.New("task or input does not match manifest")

type Run struct {
	ID         string         `json:"id"`
	PluginID   string         `json:"plugin_id"`
	ReleaseID  string         `json:"release_id"`
	TaskID     string         `json:"task_id"`
	State      string         `json:"state"`
	Progress   int            `json:"progress"`
	CreatedAt  string         `json:"created_at"`
	StartedAt  string         `json:"started_at"`
	FinishedAt string         `json:"finished_at"`
	Error      string         `json:"error"`
	Input      map[string]any `json:"input,omitempty"`
	Result     map[string]any `json:"result,omitempty"`
}

const runColumns = "id,plugin_id,release_id,task_id,state,progress,created_at,started_at,finished_at,error,input,result"

type rowScanner interface{ Scan(...any) error }

func scanRun(row rowScanner) (Run, error) {
	var r Run
	var input, result string
	e := row.Scan(&r.ID, &r.PluginID, &r.ReleaseID, &r.TaskID, &r.State, &r.Progress, &r.CreatedAt, &r.StartedAt, &r.FinishedAt, &r.Error, &input, &result)
	if e != nil {
		return r, e
	}
	if e = json.Unmarshal([]byte(input), &r.Input); e != nil {
		return r, e
	}
	e = json.Unmarshal([]byte(result), &r.Result)
	return r, e
}
func (s *Server) run(id string) (Run, error) {
	return scanRun(s.db.QueryRow("SELECT "+runColumns+" FROM runs WHERE id=?", id))
}
func publicRun(r Run, p principal) Run {
	if p.User.Role == "viewer" {
		r.Input = nil
		r.Result = nil
		r.Error = ""
	}
	return r
}
func taskFromManifest(m Manifest, id string) (Task, bool) {
	for _, task := range m.Tasks {
		if task.ID == id {
			return task, true
		}
	}
	return Task{}, false
}
func enqueueTx(tx *sql.Tx, pluginID, taskID string, input map[string]any, userID, key, scheduleID, slot string) (Run, error) {
	payload, _ := json.Marshal(map[string]any{"plugin_id": pluginID, "task_id": taskID, "input": input})
	if key != "" {
		var existingPayload, id string
		e := tx.QueryRow("SELECT id,payload FROM runs WHERE user_id=? AND idempotency_key=?", userID, key).Scan(&id, &existingPayload)
		if e == nil {
			if existingPayload != string(payload) {
				return Run{}, errConflict
			}
			return scanRun(tx.QueryRow("SELECT "+runColumns+" FROM runs WHERE id=?", id))
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return Run{}, e
		}
	}
	var releaseID, state, raw string
	if e := tx.QueryRow("SELECT p.active_release_id,p.state,r.manifest FROM plugins p JOIN releases r ON r.id=p.active_release_id WHERE p.id=?", pluginID).Scan(&releaseID, &state, &raw); e != nil {
		return Run{}, e
	}
	if state != "active" {
		return Run{}, errPluginDisabled
	}
	var m Manifest
	if e := json.Unmarshal([]byte(raw), &m); e != nil {
		return Run{}, e
	}
	task, ok := taskFromManifest(m, taskID)
	if !ok || validateInput(task.InputSchema, input) != nil {
		return Run{}, errInvalidTask
	}
	var count int
	if e := tx.QueryRow("SELECT count(*) FROM runs WHERE state='queued'").Scan(&count); e != nil {
		return Run{}, e
	}
	if count >= maxQueuedJobs {
		return Run{}, errQueueFull
	}
	data, e := json.Marshal(input)
	if e != nil {
		return Run{}, errInvalidTask
	}
	id, now := randomID(), nowString()
	var idem any
	if key != "" {
		idem = key
	}
	if _, e = tx.Exec("INSERT INTO runs(id,plugin_id,release_id,task_id,state,created_at,input,user_id,idempotency_key,payload,next_attempt_at,schedule_id,scheduled_at) VALUES(?,?,?,?,'queued',?,?,?,?,?,?,?,?)", id, pluginID, releaseID, taskID, now, string(data), userID, idem, string(payload), now, scheduleID, slot); e != nil {
		return Run{}, e
	}
	return Run{ID: id, PluginID: pluginID, ReleaseID: releaseID, TaskID: taskID, State: "queued", CreatedAt: now, Input: input, Result: map[string]any{}}, nil
}
func (s *Server) wakeJobs() {
	for i := 0; i < jobConcurrency; i++ {
		select {
		case s.jobWake <- struct{}{}:
		default:
		}
	}
	s.notify()
}

// StartBackground is called by serve, never by admin create or Open.
func (s *Server) StartBackground() error {
	s.backgroundMu.Lock()
	defer s.backgroundMu.Unlock()
	select {
	case <-s.done:
		return errors.New("server is closed")
	default:
	}
	if s.backgroundStarted {
		return nil
	}
	tx, e := s.db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.Exec("UPDATE runs SET state='interrupted',finished_at=?,error='Core restarted during execution' WHERE state='running'", nowString()); e != nil {
		return e
	}
	if e = skipMissedSchedules(tx, time.Now().UTC()); e != nil {
		return e
	}
	if e = tx.Commit(); e != nil {
		return e
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.backgroundCancel = cancel
	s.backgroundStarted = true
	for i := 0; i < jobConcurrency; i++ {
		s.workers.Add(1)
		go func() { defer s.workers.Done(); s.jobWorker(ctx) }()
	}
	s.workers.Add(1)
	go func() { defer s.workers.Done(); s.scheduler(ctx) }()
	s.wakeJobs()
	return nil
}
func (s *Server) claimRun() (Run, error) {
	now := nowString()
	return scanRun(s.db.QueryRow(`UPDATE runs SET state='running',started_at=?,finished_at='',error='',attempts=attempts+1 WHERE id=(SELECT r.id FROM runs r JOIN plugins p ON p.id=r.plugin_id WHERE r.state='queued' AND r.next_attempt_at<=? AND p.state='active' ORDER BY r.created_at,r.id LIMIT 1) AND state='queued' RETURNING `+runColumns, now, now))
}
func (s *Server) jobWorker(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		run, e := s.claimRun()
		if e == nil {
			s.executeRun(ctx, run)
			continue
		}
		if !errors.Is(e, sql.ErrNoRows) {
			s.log(LogEntry{Level: "ERROR", Message: "Queue claim failed", Component: "jobs"})
		}
		select {
		case <-ctx.Done():
			return
		case <-s.jobWake:
		case <-ticker.C:
		}
	}
}
func (s *Server) executeRun(parent context.Context, run Run) {
	manifest, executable, e := s.release(run.ReleaseID)
	task, ok := taskFromManifest(manifest, run.TaskID)
	if e != nil || !ok {
		s.finishRun(run.ID, "failed", "Pinned release unavailable", nil)
		return
	}
	settings, e := s.settings(run.PluginID)
	if e != nil {
		s.finishRun(run.ID, "failed", "Settings unavailable", nil)
		return
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(task.TimeoutSeconds)*time.Second)
	s.jobMu.Lock()
	s.jobCancels[run.ID] = cancel
	s.jobMu.Unlock()
	defer func() { cancel(); s.jobMu.Lock(); delete(s.jobCancels, run.ID); s.jobMu.Unlock() }()
	// Cancel may commit between atomic claim and context registration.
	current, e := s.run(run.ID)
	if e != nil || current.State != "running" {
		return
	}
	s.notify()
	result, e := runPlugin(ctx, executable, pluginRequest{ProtocolVersion: 1, Operation: "task", TaskID: run.TaskID, Input: run.Input, Settings: settings, DataDir: filepath.Join(s.dataDir, "plugin-data", run.PluginID), RunID: run.ID}, func(progress int) {
		if _, err := s.db.Exec("UPDATE runs SET progress=? WHERE id=? AND state='running'", progress, run.ID); err != nil {
			s.log(LogEntry{Level: "ERROR", Message: "Progress persistence failed", Component: "jobs", RunID: run.ID})
		}
		s.notify()
	}, func(message string) {
		message = redactSettings(message, settings)
		// JSON escaping must still fit the existing 64 KiB log reader limit.
		if len(message) > 8192 {
			message = message[:8192] + " [truncated]"
		}
		s.log(LogEntry{Message: message, Component: "plugin", PluginID: run.PluginID, TaskID: run.TaskID, RunID: run.ID})
	})
	if parent.Err() != nil {
		s.finishRun(run.ID, "interrupted", "Core stopped during execution", nil)
		return
	}
	if e == nil {
		s.finishRun(run.ID, "succeeded", "", result)
		return
	}
	message := "Plugin execution failed"
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		message = "Task timeout"
	}
	if task.Idempotent && ctx.Err() == nil {
		changed, err := s.db.Exec("UPDATE runs SET state='queued',next_attempt_at=?,error=? WHERE id=? AND state='running' AND attempts<3", time.Now().Add(2*time.Second).UTC().Format(timestampFormat), message, run.ID)
		if err == nil {
			n, _ := changed.RowsAffected()
			if n > 0 {
				s.wakeJobs()
				return
			}
		} else {
			s.log(LogEntry{Level: "ERROR", Message: "Retry persistence failed", Component: "jobs", RunID: run.ID})
		}
	}
	s.finishRun(run.ID, "failed", message, nil)
}
func redactSettings(message string, settings map[string]any) string {
	var visit func(any)
	visit = func(v any) {
		switch value := v.(type) {
		case string:
			if value != "" {
				message = strings.ReplaceAll(message, value, "[REDACTED]")
			}
		case map[string]any:
			for _, v := range value {
				visit(v)
			}
		case []any:
			for _, v := range value {
				visit(v)
			}
		}
	}
	visit(settings)
	return message
}
func (s *Server) finishRun(id, state, message string, result map[string]any) {
	data := "{}"
	if result != nil {
		b, _ := json.Marshal(result)
		data = string(b)
	}
	progress := 0
	if state == "succeeded" {
		progress = 100
	}
	if _, e := s.db.Exec("UPDATE runs SET state=?,finished_at=?,error=?,result=?,progress=CASE WHEN ?=100 THEN 100 ELSE progress END WHERE id=? AND state='running'", state, nowString(), message, data, progress, id); e != nil {
		s.log(LogEntry{Level: "ERROR", Message: "Run completion persistence failed", Component: "jobs", RunID: id})
	}
	s.notify()
}
func (s *Server) registerJobRoutes() {
	s.route("GET /api/v1/runs", "viewer", "runs:read", s.listRuns)
	s.route("GET /api/v1/runs/{id}", "viewer", "runs:read", s.getRun)
	s.route("POST /api/v1/plugins/{id}/tasks/{task_id}/runs", "operator", "runs:write", s.createRun)
	s.route("POST /api/v1/runs/{id}/cancel", "operator", "runs:write", s.cancelRun)
}
func (s *Server) createRun(w http.ResponseWriter, r *http.Request, p principal) {
	var body struct {
		Input map[string]any `json:"input"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Input == nil {
		apiError(w, 400, "invalid_input", "input object is required")
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if len(key) > 200 || strings.TrimSpace(key) != key {
		apiError(w, 400, "invalid_input", "Invalid idempotency key")
		return
	}
	select {
	case <-s.done:
		apiError(w, 503, "shutting_down", "Core is stopping")
		return
	default:
	}
	tx, e := s.db.BeginTx(r.Context(), nil)
	if e != nil {
		internalError(w)
		return
	}
	defer tx.Rollback()
	run, e := enqueueTx(tx, r.PathValue("id"), r.PathValue("task_id"), body.Input, p.User.ID, key, "", "")
	if e != nil {
		jobError(w, e)
		return
	}
	if e = tx.Commit(); e != nil {
		internalError(w)
		return
	}
	s.wakeJobs()
	writeJSON(w, 201, publicRun(run, p))
}
func jobError(w http.ResponseWriter, e error) {
	switch {
	case errors.Is(e, errConflict):
		apiError(w, 409, "idempotency_conflict", e.Error())
	case errors.Is(e, errPluginDisabled):
		apiError(w, 409, "plugin_disabled", e.Error())
	case errors.Is(e, errQueueFull):
		apiError(w, 429, "queue_full", e.Error())
	case errors.Is(e, errInvalidTask):
		apiError(w, 400, "invalid_input", e.Error())
	case errors.Is(e, sql.ErrNoRows):
		apiError(w, 404, "not_found", "Run or plugin not found")
	default:
		internalError(w)
	}
}
func (s *Server) getRun(w http.ResponseWriter, r *http.Request, p principal) {
	run, e := s.run(r.PathValue("id"))
	if e != nil {
		jobError(w, e)
		return
	}
	writeJSON(w, 200, publicRun(run, p))
}
func (s *Server) listRuns(w http.ResponseWriter, r *http.Request, p principal) {
	limit, offset, ok := pagination(w, r)
	if !ok {
		return
	}
	var total int
	if e := s.db.QueryRow("SELECT count(*) FROM runs").Scan(&total); e != nil {
		internalError(w)
		return
	}
	rows, e := s.db.Query("SELECT "+runColumns+" FROM runs ORDER BY created_at DESC,id LIMIT ? OFFSET ?", limit, offset)
	if e != nil {
		internalError(w)
		return
	}
	defer rows.Close()
	items := []Run{}
	for rows.Next() {
		run, e := scanRun(rows)
		if e != nil {
			internalError(w)
			return
		}
		items = append(items, publicRun(run, p))
	}
	if rows.Err() != nil {
		internalError(w)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": total})
}
func (s *Server) cancelRun(w http.ResponseWriter, r *http.Request, p principal) {
	id := r.PathValue("id")
	changed, e := s.db.Exec("UPDATE runs SET state='cancelled',finished_at=?,error='Cancelled by operator' WHERE id=? AND state IN ('queued','running')", nowString(), id)
	if e != nil {
		internalError(w)
		return
	}
	n, _ := changed.RowsAffected()
	if n > 0 {
		s.jobMu.Lock()
		if cancel := s.jobCancels[id]; cancel != nil {
			cancel()
		}
		s.jobMu.Unlock()
		s.notify()
	}
	run, e := s.run(id)
	if e != nil {
		jobError(w, e)
		return
	}
	writeJSON(w, 200, publicRun(run, p))
}
