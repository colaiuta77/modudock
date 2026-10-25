// 비상 MFA 복구가 비밀번호를 보존하고 모든 인증 수단을 폐기하는지 검증한다.
package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResetMFAPreservesPasswordAndRevokesAll(t *testing.T) {
	c := newClient(t)
	id := c.login("admin")["id"].(string)
	token(t, c, "status:read")
	_, codes := enrollTOTP(t, c)
	acceptMFASession(t, c, finishMFA(c, beginMFA(t, c), "recovery", codes[0]))
	reauth(t, c, "totp-disable", "recovery", codes[1])
	if _, e := c.s.db.Exec(`INSERT INTO passkeys(id,user_id,name,credential,created_at) VALUES('reset-key',?,'test','{}',?)`, id, time.Now().UTC().Format(time.RFC3339)); e != nil {
		t.Fatal(e)
	}
	before, e := c.s.loadAuthAccount(t.Context(), id)
	if e != nil {
		t.Fatal(e)
	}
	if e = c.s.ResetMFA("missing"); e == nil {
		t.Fatal("unknown user accepted")
	}
	if e = c.s.ResetMFA("admin"); e != nil {
		t.Fatal(e)
	}
	after, e := c.s.loadAuthAccount(t.Context(), id)
	if e != nil {
		t.Fatal(e)
	}
	if before.PasswordHash != after.PasswordHash || before.Role != after.Role || !after.IsActive {
		t.Fatal("reset changed account")
	}
	for _, table := range []string{"sessions", "tokens", "user_totp", "recovery_codes", "passkeys", "auth_challenges"} {
		var n int
		if e = c.s.db.QueryRow("SELECT count(*) FROM "+table+" WHERE user_id=?", id).Scan(&n); e != nil || n != 0 {
			t.Fatalf("%s retained %d: %v", table, n, e)
		}
	}
	var auditCount int
	c.s.db.QueryRow("SELECT count(*) FROM audit WHERE action='mfa_reset_cli' AND user_id=?", id).Scan(&auditCount)
	if auditCount != 1 {
		t.Fatal("reset was not audited")
	}
	c.login("admin")
}
func TestResetMFAWorksWithoutEncryptionKey(t *testing.T) {
	for _, mode := range []string{"missing", "corrupt"} {
		t.Run(mode, func(t *testing.T) {
			c := newClient(t)
			c.login("admin")
			enrollTOTP(t, c)
			key := filepath.Join(c.s.dataDir, "auth.key")
			var e error
			if mode == "missing" {
				e = os.Remove(key)
			} else {
				e = os.WriteFile(key, []byte("broken"), 0600)
			}
			if e != nil {
				t.Fatal(e)
			}
			if e = c.s.ResetMFA("admin"); e != nil {
				t.Fatal(e)
			}
			c.login("admin")
			if mode == "corrupt" {
				b, e := os.ReadFile(key)
				if e != nil || string(b) != "broken" {
					t.Fatal("reset silently replaced damaged key")
				}
			}
		})
	}
}
