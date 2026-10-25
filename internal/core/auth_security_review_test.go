// 보안 변경 전 인증 요청과 익명 발급 제한 및 키 누락 시 잠금 순서를 검증한다.
package core

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPasskeyLoginRejectsSecurityChange(t *testing.T) {
	for _, change := range []string{"password", "totp"} {
		t.Run(change, func(t *testing.T) {
			c := passkeyClient(t)
			key := registerTestPasskey(t, c)
			c.login("admin")
			anonymous := &client{t: t, s: c.s}
			begin := passkeyRequest(anonymous, "passkey/begin", map[string]any{}, nil)
			assertion := key.assertion(t, authJSON(t, begin, 200), "")
			if change == "password" {
				status(t, c.request("PUT", "/api/v1/auth/password", fmt.Sprintf(`{"current_password":%q,"new_password":"changed-password-2026"}`, password)), 204)
			} else {
				enrollTOTP(t, c)
			}
			status(t, passkeyRequest(anonymous, "passkey/finish", map[string]any{"credential": assertion}, begin.Result().Cookies()), 401)
		})
	}
}

func TestPasskeyBeginKeepsIssuanceBudget(t *testing.T) {
	c := passkeyClient(t)
	for range 8 {
		status(t, passkeyRequest(c, "passkey/begin", map[string]any{}, nil), 200)
	}
	status(t, passkeyRequest(c, "passkey/begin", map[string]any{}, nil), 429)
	var count int
	if e := c.s.db.QueryRow(`SELECT count(*) FROM auth_challenges WHERE purpose='passkey-login'`).Scan(&count); e != nil || count != 8 {
		t.Fatalf("anonymous states: %d, %v", count, e)
	}
	// 익명 발급 제한은 정상 계정의 별도 재인증 예산을 소모하지 않는다.
	reauth(t, c, "totp-setup", "", "")
}

func TestAuthSecretMissingKeyDoesNotInvertDBLock(t *testing.T) {
	c := newClient(t)
	userID := c.login("admin")["id"].(string)
	sealed, err := c.s.sealAuthSecret(userID, "totp", []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.s.db.Exec(`INSERT INTO user_totp(user_id,secret) VALUES(?,?)`, userID, sealed); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(c.s.dataDir, "auth.key")); err != nil {
		t.Fatal(err)
	}
	tx, err := c.s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	waits := c.s.db.Stats().WaitCount
	creating := make(chan error, 1)
	go func() { _, e := c.s.sealAuthSecret(userID, "totp-setup", []byte("new")); creating <- e }()
	deadline := time.Now().Add(2 * time.Second)
	for c.s.db.Stats().WaitCount == waits && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if c.s.db.Stats().WaitCount == waits {
		tx.Rollback()
		<-creating
		t.Fatal("key creation did not wait on occupied DB")
	}
	reading := make(chan error, 1)
	go func() { _, e := c.s.openAuthSecret(userID, "totp", sealed); reading <- e }()
	blocked := false
	select {
	case e := <-reading:
		if e == nil {
			t.Error("missing key decrypted")
		}
	case <-time.After(time.Second):
		blocked = true
	}
	tx.Rollback()
	if e := <-creating; e == nil {
		t.Error("missing key recreated")
	}
	if blocked {
		<-reading
		t.Fatal("key read waited on creation mutex while DB was held")
	}
}
