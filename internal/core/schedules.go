// 5필드 cron 스케줄과 원자적 실행 슬롯을 SQLite에 보관한다.
package core

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/robfig/cron/v3"
)

type Schedule struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	PluginID  string         `json:"plugin_id"`
	TaskID    string         `json:"task_id"`
	Cron      string         `json:"cron"`
	Timezone  string         `json:"timezone"`
	Input     map[string]any `json:"input"`
	Enabled   bool           `json:"enabled"`
	NextRunAt string         `json:"next_run_at"`
	LastRunAt string         `json:"last_run_at"`
}
type scheduleBody struct {
	Name     string         `json:"name"`
	PluginID string         `json:"plugin_id"`
	TaskID   string         `json:"task_id"`
	Cron     string         `json:"cron"`
	Timezone string         `json:"timezone"`
	Input    map[string]any `json:"input"`
	Enabled  *bool          `json:"enabled"`
}

const scheduleColumns = "id,name,plugin_id,task_id,cron,timezone,input,enabled,next_run_at,last_run_at"

func scanSchedule(row rowScanner) (Schedule, error) {
	var schedule Schedule
	var input string
	e := row.Scan(&schedule.ID, &schedule.Name, &schedule.PluginID, &schedule.TaskID, &schedule.Cron, &schedule.Timezone, &input, &schedule.Enabled, &schedule.NextRunAt, &schedule.LastRunAt)
	if e == nil {
		e = json.Unmarshal([]byte(input), &schedule.Input)
	}
	return schedule, e
}
func nextSchedule(expression, timezone string, after time.Time) (time.Time, error) {
	if len(expression) > 200 || len(strings.Fields(expression)) != 5 || timezone == "" || len(timezone) > 100 || timezone == "Local" {
		return time.Time{}, errors.New("expected five-field cron and IANA timezone")
	}
	location, e := time.LoadLocation(timezone)
	if e != nil {
		return time.Time{}, e
	}
	parsed, e := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow).Parse(expression)
	if e != nil {
		return time.Time{}, e
	}
	next := parsed.Next(after.In(location))
	if next.IsZero() {
		return time.Time{}, errors.New("cron has no upcoming occurrence")
	}
	return next.UTC(), nil
}
func (s *Server) registerScheduleRoutes() {
	s.route("GET /api/v1/schedules", "operator", "schedules:read", s.listSchedules)
	s.route("POST /api/v1/schedules", "operator", "schedules:write", s.saveSchedule)
	s.route("PUT /api/v1/schedules/{id}", "operator", "schedules:write", s.saveSchedule)
	s.route("DELETE /api/v1/schedules/{id}", "operator", "schedules:write", s.deleteSchedule)
}
func (s *Server) saveSchedule(w http.ResponseWriter, r *http.Request, p principal) {
	var body scheduleBody
	if !decodeJSON(w, r, &body) {
		return
	}
	next, e := nextSchedule(body.Cron, body.Timezone, time.Now().UTC())
	if e != nil || strings.TrimSpace(body.Name) == "" || len(body.Name) > 100 || body.Input == nil || body.Enabled == nil {
		apiError(w, 400, "invalid_input", "Invalid schedule fields")
		return
	}
	tx, e := s.db.BeginTx(r.Context(), nil)
	if e != nil {
		internalError(w)
		return
	}
	defer tx.Rollback()
	var raw, state string
	if e = tx.QueryRow("SELECT r.manifest,p.state FROM plugins p JOIN releases r ON r.id=p.active_release_id WHERE p.id=?", body.PluginID).Scan(&raw, &state); e != nil {
		jobError(w, e)
		return
	}
	var m Manifest
	if json.Unmarshal([]byte(raw), &m) != nil {
		internalError(w)
		return
	}
	task, ok := taskFromManifest(m, body.TaskID)
	if !ok || validateInput(task.InputSchema, body.Input) != nil {
		jobError(w, errInvalidTask)
		return
	}
	id := r.PathValue("id")
	created := id == ""
	if created {
		id = randomID()
		var total int
		if e = tx.QueryRow("SELECT count(*) FROM schedules").Scan(&total); e != nil {
			internalError(w)
			return
		}
		if total >= 1000 {
			apiError(w, 429, "schedule_limit", "Too many schedules")
			return
		}
	}
	data, _ := json.Marshal(body.Input)
	nextAt := next.Format(timestampFormat)
	if created {
		_, e = tx.Exec("INSERT INTO schedules(id,name,plugin_id,task_id,cron,timezone,input,enabled,next_run_at,user_id) VALUES(?,?,?,?,?,?,?,?,?,?)", id, body.Name, body.PluginID, body.TaskID, body.Cron, body.Timezone, string(data), *body.Enabled, nextAt, p.User.ID)
	} else {
		var result sql.Result
		result, e = tx.Exec("UPDATE schedules SET name=?,plugin_id=?,task_id=?,cron=?,timezone=?,input=?,enabled=?,next_run_at=?,user_id=? WHERE id=?", body.Name, body.PluginID, body.TaskID, body.Cron, body.Timezone, string(data), *body.Enabled, nextAt, p.User.ID, id)
		if e == nil {
			n, _ := result.RowsAffected()
			if n == 0 {
				apiError(w, 404, "not_found", "Schedule not found")
				return
			}
		}
	}
	if e != nil {
		internalError(w)
		return
	}
	schedule, e := scanSchedule(tx.QueryRow("SELECT "+scheduleColumns+" FROM schedules WHERE id=?", id))
	if e != nil {
		internalError(w)
		return
	}
	if e = tx.Commit(); e != nil {
		internalError(w)
		return
	}
	s.notify()
	status := 200
	if created {
		status = 201
	}
	writeJSON(w, status, schedule)
}
func (s *Server) listSchedules(w http.ResponseWriter, r *http.Request, p principal) {
	limit, offset, ok := pagination(w, r)
	if !ok {
		return
	}
	var total int
	if e := s.db.QueryRow("SELECT count(*) FROM schedules").Scan(&total); e != nil {
		internalError(w)
		return
	}
	rows, e := s.db.Query("SELECT "+scheduleColumns+" FROM schedules ORDER BY name,id LIMIT ? OFFSET ?", limit, offset)
	if e != nil {
		internalError(w)
		return
	}
	defer rows.Close()
	items := []Schedule{}
	for rows.Next() {
		schedule, e := scanSchedule(rows)
		if e != nil {
			internalError(w)
			return
		}
		items = append(items, schedule)
	}
	if rows.Err() != nil {
		internalError(w)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": total})
}
func (s *Server) deleteSchedule(w http.ResponseWriter, r *http.Request, p principal) {
	result, e := s.db.Exec("DELETE FROM schedules WHERE id=?", r.PathValue("id"))
	if e != nil {
		internalError(w)
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		apiError(w, 404, "not_found", "Schedule not found")
		return
	}
	s.notify()
	writeJSON(w, 204, nil)
}
func skipMissedSchedules(tx *sql.Tx, now time.Time) error {
	rows, e := tx.Query("SELECT id,cron,timezone FROM schedules WHERE next_run_at<=?", now.UTC().Format(timestampFormat))
	if e != nil {
		return e
	}
	type item struct{ id, cron, timezone string }
	items := []item{}
	for rows.Next() {
		var i item
		if e = rows.Scan(&i.id, &i.cron, &i.timezone); e != nil {
			break
		}
		items = append(items, i)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return e
	}
	for _, i := range items {
		next, e := nextSchedule(i.cron, i.timezone, now)
		if e != nil {
			return e
		}
		if _, e = tx.Exec("UPDATE schedules SET next_run_at=? WHERE id=?", next.Format(timestampFormat), i.id); e != nil {
			return e
		}
	}
	return nil
}
func (s *Server) scheduler(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if e := s.processDueSchedules(now.UTC()); e != nil {
				s.log(LogEntry{Level: "ERROR", Message: "Schedule transaction failed", Component: "schedules"})
			}
		}
	}
}
func (s *Server) processDueSchedules(now time.Time) error {
	tx, e := s.db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	rows, e := tx.Query("SELECT "+scheduleColumns+",user_id FROM schedules WHERE enabled=1 AND next_run_at<=? ORDER BY next_run_at,id", now.UTC().Format(timestampFormat))
	if e != nil {
		return e
	}
	type due struct {
		schedule Schedule
		user     string
	}
	items := []due{}
	for rows.Next() {
		var d due
		var input string
		v := &d.schedule
		if e = rows.Scan(&v.ID, &v.Name, &v.PluginID, &v.TaskID, &v.Cron, &v.Timezone, &input, &v.Enabled, &v.NextRunAt, &v.LastRunAt, &d.user); e != nil {
			break
		}
		if e = json.Unmarshal([]byte(input), &v.Input); e != nil {
			break
		}
		items = append(items, d)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return e
	}
	changed := false
	skips := []string{}
	for _, d := range items {
		schedule := d.schedule
		next, e := nextSchedule(schedule.Cron, schedule.Timezone, now)
		if e != nil {
			return e
		}
		slotTime, e := time.Parse(time.RFC3339Nano, schedule.NextRunAt)
		if e != nil {
			return e
		}
		outcome := "queued"
		runID := ""
		var overlap int
		if slotTime.Before(now.Truncate(time.Minute)) {
			outcome = "missed_skipped"
		} else if e = tx.QueryRow("SELECT count(*) FROM runs WHERE schedule_id=? AND state IN ('queued','running')", schedule.ID).Scan(&overlap); e != nil {
			return e
		} else if overlap > 0 {
			outcome = "overlap_skipped"
		}
		result, e := tx.Exec("INSERT OR IGNORE INTO schedule_slots(schedule_id,scheduled_at,outcome) VALUES(?,?,?)", schedule.ID, schedule.NextRunAt, outcome)
		if e != nil {
			return e
		}
		n, _ := result.RowsAffected()
		if n > 0 && outcome == "queued" {
			run, e := enqueueTx(tx, schedule.PluginID, schedule.TaskID, schedule.Input, d.user, "", schedule.ID, schedule.NextRunAt)
			if e != nil {
				if errors.Is(e, errPluginDisabled) || errors.Is(e, errQueueFull) || errors.Is(e, errInvalidTask) {
					outcome = "unavailable_skipped"
				} else {
					return e
				}
			} else {
				runID = run.ID
				changed = true
			}
		}
		if n > 0 {
			if _, e = tx.Exec("UPDATE schedule_slots SET run_id=?,outcome=? WHERE schedule_id=? AND scheduled_at=?", runID, outcome, schedule.ID, schedule.NextRunAt); e != nil {
				return e
			}
			if outcome != "queued" {
				skips = append(skips, outcome)
			}
		}
		if _, e = tx.Exec("UPDATE schedules SET next_run_at=?,last_run_at=CASE WHEN ?!='' THEN ? ELSE last_run_at END WHERE id=?", next.Format(timestampFormat), runID, schedule.NextRunAt, schedule.ID); e != nil {
			return e
		}
	}
	if e = tx.Commit(); e != nil {
		return e
	}
	if changed {
		s.wakeJobs()
	} else if len(items) > 0 {
		s.notify()
	}
	for _, outcome := range skips {
		s.log(LogEntry{Message: "Schedule " + outcome, Component: "schedules"})
	}
	return nil
}
