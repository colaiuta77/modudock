// 각 SQLite 연결의 안전 설정과 실제 엔진 버전을 검증한다.
package core

import (
	"testing"
)

func TestSQLitePragmas(t *testing.T) {
	c := newClient(t)
	c.s.db.SetMaxIdleConns(0)
	var foreignKeys, timeout int
	var mode, version string
	if e := c.s.db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); e != nil {
		t.Fatal(e)
	}
	if e := c.s.db.QueryRow("PRAGMA busy_timeout").Scan(&timeout); e != nil {
		t.Fatal(e)
	}
	if e := c.s.db.QueryRow("PRAGMA journal_mode").Scan(&mode); e != nil {
		t.Fatal(e)
	}
	if foreignKeys != 1 || timeout != 5000 || mode != "wal" {
		t.Fatalf("unsafe SQLite pragmas fk=%d busy=%d journal=%s", foreignKeys, timeout, mode)
	}
	if e := c.s.db.QueryRow("SELECT sqlite_version()").Scan(&version); e != nil {
		t.Fatal(e)
	}
	t.Logf("SQLite runtime version: %s", version)
}
