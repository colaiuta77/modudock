// 인증 상태의 단회 소비와 계정 변경 경계 및 기존 DB 보존을 검증한다.
package core

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSecurityMigrationPreservesV2Data(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "core.db"))
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{authSchema, pluginSchema,
		`INSERT INTO users VALUES('legacy','legacy','old-hash','admin',1,'2026-01-01T00:00:00Z')`,
		`INSERT INTO sessions VALUES('session','legacy','csrf','2099-01-01T00:00:00Z')`,
		`INSERT INTO tokens VALUES('token','hash','legacy','automation','[]','2099-01-01T00:00:00Z','2026-01-01T00:00:00Z')`,
		`INSERT INTO releases VALUES('r','legacy-plugin','1','commit',1,'2026-01-01T00:00:00Z','{}','tool')`,
		`INSERT INTO plugins VALUES('legacy-plugin','active','https://github.com/example/legacy','r','{}')`,
	} {
		if _, err = db.Exec(statement); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var version int
	if err = s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 3 {
		t.Fatalf("schema version = %d, want 3", version)
	}
	for _, table := range []string{"users", "sessions", "tokens", "plugins", "releases"} {
		var count int
		if err = s.db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("%s count=%d err=%v", table, count, err)
		}
	}
	var hash string
	if err = s.db.QueryRow("SELECT password_hash FROM users WHERE id='legacy'").Scan(&hash); err != nil || hash != "old-hash" {
		t.Fatalf("password changed: %v", err)
	}
}

func TestAuthChallengeSingleUseAndBindings(t *testing.T) {
	c := newClient(t)
	userID := c.login("admin")["id"].(string)
	ctx := context.Background()
	sessionHash := hashToken(c.cookie.Value)
	state := authChallenge{Purpose: "reauth:totp-setup", UserID: userID, SessionHash: sessionHash, BindingHash: "browser", AuthVersion: 0, ExpiresAt: time.Now().Add(time.Minute), Payload: json.RawMessage(`{"ok":true}`)}
	raw, err := c.s.putAuthChallenge(ctx, state)
	if err != nil {
		t.Fatal(err)
	}
	for _, mismatch := range []string{"purpose", "user", "session", "binding", "version"} {
		purpose, user, session, binding, version := state.Purpose, userID, sessionHash, "browser", int64(0)
		switch mismatch {
		case "purpose":
			purpose = "other"
		case "user":
			user = "other"
		case "session":
			session = "other"
		case "binding":
			binding = "other"
		case "version":
			version = 1
		}
		tx, e := c.s.db.BeginTx(ctx, nil)
		if e != nil {
			t.Fatal(e)
		}
		_, e = claimAuthChallenge(tx, raw, purpose, user, session, binding, version, time.Now())
		tx.Rollback()
		if e == nil {
			t.Fatalf("accepted wrong %s", mismatch)
		}
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			tx, e := c.s.db.BeginTx(ctx, nil)
			if e != nil {
				return
			}
			defer tx.Rollback()
			payload, e := claimAuthChallenge(tx, raw, state.Purpose, userID, sessionHash, "browser", 0, time.Now())
			if e == nil && string(payload) == `{"ok":true}` && tx.Commit() == nil {
				successes.Add(1)
			}
		})
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("successful consumers = %d", successes.Load())
	}
	if _, err = c.s.readAuthChallenge(ctx, raw); err == nil {
		t.Fatal("consumed challenge still readable")
	}
	state.ExpiresAt = time.Now().Add(-time.Minute)
	expired, err := c.s.putAuthChallenge(ctx, state)
	if err == nil {
		if _, err = c.s.readAuthChallenge(ctx, expired); err == nil {
			t.Fatal("expired challenge accepted")
		}
	}
}

func TestAuthSessionRejectsChangedAccount(t *testing.T) {
	c := newClient(t)
	userID := c.login("admin")["id"].(string)
	rawToken := token(t, c, "status:read")
	ctx := context.Background()
	account, err := c.s.loadAuthAccount(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := c.s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = revokeWebCredentials(tx, userID); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	tx, err = c.s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.s.issueSession(ctx, tx, userID, account.AuthVersion); err == nil {
		tx.Rollback()
		t.Fatal("stale account got session")
	}
	tx.Rollback()
	status(t, c.request("GET", "/api/v1/auth/me", ""), 401)
	c.bearer = rawToken
	status(t, c.request("GET", "/api/v1/status", ""), 200)
}
