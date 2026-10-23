// SQLite 초기화와 데이터 디렉터리 독점 잠금을 관리한다.
package core

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/gofrs/flock"
	_ "modernc.org/sqlite"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const authSchema = `CREATE TABLE IF NOT EXISTS users(id TEXT PRIMARY KEY,username TEXT NOT NULL UNIQUE COLLATE NOCASE,password_hash TEXT NOT NULL,role TEXT NOT NULL CHECK(role IN ('admin','operator','viewer')),is_active INTEGER NOT NULL DEFAULT 1,created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS sessions(hash TEXT PRIMARY KEY,user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,csrf TEXT NOT NULL,expires_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS tokens(id TEXT PRIMARY KEY,hash TEXT NOT NULL UNIQUE,user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,name TEXT NOT NULL,scopes TEXT NOT NULL,expires_at TEXT NOT NULL,created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS audit(id TEXT PRIMARY KEY,timestamp TEXT NOT NULL,action TEXT NOT NULL,user_id TEXT NOT NULL,request_id TEXT NOT NULL);
PRAGMA user_version=1;`

func (s *Server) openDB() error {
	if s.dataDir == "" {
		return errors.New("data-dir required")
	}
	dir, e := filepath.Abs(s.dataDir)
	if e != nil {
		return e
	}
	s.dataDir = dir
	if e = os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	s.lock = flock.New(filepath.Join(dir, "core.lock"))
	locked, e := s.lock.TryLock()
	if e != nil {
		return e
	}
	if !locked {
		return errors.New("data directory already in use")
	}
	dbPath := filepath.ToSlash(filepath.Join(dir, "core.db"))
	if !strings.HasPrefix(dbPath, "/") {
		dbPath = "/" + dbPath
	}
	u := url.URL{Scheme: "file", Path: dbPath}
	q := u.Query()
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "journal_mode(WAL)")
	u.RawQuery = q.Encode()
	s.db, e = sql.Open("sqlite", u.String())
	if e != nil {
		s.lock.Unlock()
		return e
	}
	s.db.SetMaxOpenConns(1)
	s.db.SetMaxIdleConns(1)
	var version int
	if e = s.db.QueryRow("PRAGMA user_version").Scan(&version); e != nil {
		s.Close()
		return e
	}
	if version > 2 {
		s.Close()
		return fmt.Errorf("unsupported database version %d", version)
	}
	if version < 1 {
		tx, e := s.db.Begin()
		if e != nil {
			s.Close()
			return e
		}
		if _, e = tx.Exec(authSchema); e != nil {
			tx.Rollback()
			s.Close()
			return e
		}
		if e = tx.Commit(); e != nil {
			s.Close()
			return e
		}
	}
	if version < 2 {
		tx, e := s.db.Begin()
		if e != nil {
			s.Close()
			return e
		}
		if _, e = tx.Exec(pluginSchema); e != nil {
			tx.Rollback()
			s.Close()
			return e
		}
		if e = tx.Commit(); e != nil {
			s.Close()
			return e
		}
	}
	if e = os.Chmod(filepath.Join(dir, "core.db"), 0600); e != nil {
		s.Close()
		return e
	}
	return nil
}

const pluginSchema = `CREATE TABLE releases(id TEXT PRIMARY KEY,plugin_id TEXT NOT NULL,version TEXT NOT NULL,commit_sha TEXT NOT NULL,data_schema_version INTEGER NOT NULL CHECK(data_schema_version>=1),created_at TEXT NOT NULL,manifest TEXT NOT NULL,executable TEXT NOT NULL);
CREATE INDEX releases_plugin ON releases(plugin_id);
CREATE TABLE plugins(id TEXT PRIMARY KEY,state TEXT NOT NULL CHECK(state IN ('active','disabled')),repository_url TEXT NOT NULL,active_release_id TEXT NOT NULL REFERENCES releases(id),settings TEXT NOT NULL DEFAULT '{}');
CREATE TABLE menus(plugin_id TEXT NOT NULL REFERENCES plugins(id),id TEXT NOT NULL,title TEXT NOT NULL,icon TEXT NOT NULL,position INTEGER NOT NULL,enabled INTEGER NOT NULL,PRIMARY KEY(plugin_id,id));
CREATE TABLE runs(id TEXT PRIMARY KEY,plugin_id TEXT NOT NULL REFERENCES plugins(id),release_id TEXT NOT NULL REFERENCES releases(id),task_id TEXT NOT NULL,state TEXT NOT NULL CHECK(state IN ('queued','running','succeeded','failed','cancelled','interrupted')),progress INTEGER NOT NULL DEFAULT 0,created_at TEXT NOT NULL,started_at TEXT NOT NULL DEFAULT '',finished_at TEXT NOT NULL DEFAULT '',error TEXT NOT NULL DEFAULT '',input TEXT NOT NULL,result TEXT NOT NULL DEFAULT '{}',user_id TEXT NOT NULL,idempotency_key TEXT,payload TEXT NOT NULL,attempts INTEGER NOT NULL DEFAULT 0,next_attempt_at TEXT NOT NULL,schedule_id TEXT NOT NULL DEFAULT '',scheduled_at TEXT NOT NULL DEFAULT '',UNIQUE(user_id,idempotency_key));
CREATE INDEX runs_queue ON runs(state,next_attempt_at,created_at);
CREATE INDEX runs_schedule ON runs(schedule_id,state);
CREATE TABLE schedules(id TEXT PRIMARY KEY,name TEXT NOT NULL,plugin_id TEXT NOT NULL REFERENCES plugins(id),task_id TEXT NOT NULL,cron TEXT NOT NULL,timezone TEXT NOT NULL,input TEXT NOT NULL,enabled INTEGER NOT NULL,next_run_at TEXT NOT NULL,last_run_at TEXT NOT NULL DEFAULT '',user_id TEXT NOT NULL);
CREATE TABLE schedule_slots(schedule_id TEXT NOT NULL REFERENCES schedules(id) ON DELETE CASCADE,scheduled_at TEXT NOT NULL,run_id TEXT NOT NULL DEFAULT '',outcome TEXT NOT NULL,PRIMARY KEY(schedule_id,scheduled_at));
PRAGMA user_version=2;`
