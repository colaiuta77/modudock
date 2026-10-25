// 보안 설정 변경 시 현재 브라우저의 세션 교체와 다른 세션 폐기를 검증한다.
package core

import (
	"fmt"
	"net/http/httptest"
	"testing"
)

func assertSecuritySession(t *testing.T, c, other *client, w *httptest.ResponseRecorder) {
	t.Helper()
	old := *c
	result := authJSON(t, w, 200)
	if result["signed_out"] == true {
		t.Fatal("current browser was signed out")
	}
	if result["csrf_token"] == nil {
		t.Fatal("new CSRF token missing")
	}
	acceptMFASession(t, c, w)
	if c.cookie.Value == old.cookie.Value || c.csrf == old.csrf {
		t.Fatal("session was not rotated")
	}
	if !c.cookie.HttpOnly || c.cookie.MaxAge <= 0 {
		t.Fatal("invalid session cookie")
	}
	status(t, old.request("GET", "/api/v1/auth/me", ""), 401)
	status(t, other.request("GET", "/api/v1/auth/me", ""), 401)
	status(t, c.request("GET", "/api/v1/auth/security", ""), 200)
	c.csrf = old.csrf
	status(t, c.request("DELETE", "/api/v1/auth/totp/setup", ""), 403)
	c.csrf = result["csrf_token"].(string)
	status(t, c.request("DELETE", "/api/v1/auth/totp/setup", ""), 204)
}

func TestSecurityChangesRotateCurrentSession(t *testing.T) {
	for _, action := range []string{"enable", "reset", "disable", "recovery"} {
		t.Run(action, func(t *testing.T) {
			c := newClient(t)
			c.login("admin")
			other := &client{t: t, s: c.s}
			var codes []string
			if action == "enable" {
				other.login("admin")
			} else {
				_, codes = enrollTOTP(t, c)
				acceptMFASession(t, c, finishMFA(c, beginMFA(t, c), "recovery", codes[0]))
				acceptMFASession(t, other, finishMFA(other, beginMFA(t, other), "recovery", codes[1]))
			}
			purpose := "totp-setup"
			if action == "disable" {
				purpose = "totp-disable"
			}
			if action == "recovery" {
				purpose = "recovery-regenerate"
			}
			method, factor := "", ""
			if len(codes) > 0 {
				method, factor = "recovery", codes[2]
			}
			proof := reauth(t, c, purpose, method, factor)
			var w *httptest.ResponseRecorder
			switch action {
			case "enable", "reset":
				setup := setupTOTP(t, c, proof)
				w = c.request("POST", "/api/v1/auth/totp/confirm", fmt.Sprintf(`{"proof":%q,"code":%q}`, proof, otpCode(t, setup["secret"].(string), 0)))
			case "disable":
				w = c.request("DELETE", "/api/v1/auth/totp", fmt.Sprintf(`{"proof":%q}`, proof))
			case "recovery":
				w = c.request("POST", "/api/v1/auth/recovery-codes", fmt.Sprintf(`{"proof":%q}`, proof))
			}
			assertSecuritySession(t, c, other, w)
			status(t, c.request("POST", "/api/v1/auth/totp/setup", fmt.Sprintf(`{"proof":%q}`, proof)), 403)
			state := authJSON(t, c.request("GET", "/api/v1/auth/security", ""), 200)
			if state["totp_enabled"] != (action != "disable") {
				t.Fatal(state)
			}
		})
	}
}

func TestPasskeyChangesRotateCurrentSession(t *testing.T) {
	for _, action := range []string{"register", "delete"} {
		t.Run(action, func(t *testing.T) {
			c := passkeyClient(t)
			var key *testPasskey
			if action == "delete" {
				key = registerTestPasskey(t, c)
				c.login("admin")
			}
			other := &client{t: t, s: c.s}
			other.login("admin")
			var w *httptest.ResponseRecorder
			if action == "register" {
				proof := reauth(t, c, "passkey-register", "", "")
				begin := passkeyRequest(c, "passkeys/register/begin", map[string]any{"proof": proof}, nil)
				key = newTestPasskey(t)
				w = passkeyRequest(c, "passkeys/register/finish", map[string]any{"proof": proof, "name": "device", "credential": key.registration(t, authJSON(t, begin, 200), 5)}, begin.Result().Cookies())
			} else {
				proof := reauth(t, c, "passkey-delete:"+encodeB64(key.id), "", "")
				w = c.request("DELETE", "/api/v1/auth/passkeys/"+encodeB64(key.id), fmt.Sprintf(`{"proof":%q}`, proof))
			}
			assertSecuritySession(t, c, other, w)
			state := authJSON(t, c.request("GET", "/api/v1/auth/security", ""), 200)
			want := 1
			if action == "delete" {
				want = 0
			}
			if len(state["passkeys"].([]any)) != want {
				t.Fatal(state)
			}
		})
	}
}

func TestSecuritySessionFailureRollsBackChange(t *testing.T) {
	c := newClient(t)
	c.login("admin")
	proof := reauth(t, c, "totp-setup", "", "")
	setup := setupTOTP(t, c, proof)
	if _, err := c.s.db.Exec(`CREATE TRIGGER reject_new_session BEFORE INSERT ON sessions BEGIN SELECT RAISE(ABORT, 'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	w := c.request("POST", "/api/v1/auth/totp/confirm", fmt.Sprintf(`{"proof":%q,"code":%q}`, proof, otpCode(t, setup["secret"].(string), 0)))
	status(t, w, 500)
	state := authJSON(t, c.request("GET", "/api/v1/auth/security", ""), 200)
	if state["totp_enabled"] != false || state["recovery_codes_remaining"] != float64(0) {
		t.Fatal("security change was not rolled back", state)
	}
}
