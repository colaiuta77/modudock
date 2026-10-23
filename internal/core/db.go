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
	if version > 1 {
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
	if e = os.Chmod(filepath.Join(dir, "core.db"), 0600); e != nil {
		s.Close()
		return e
	}
	return nil
}
