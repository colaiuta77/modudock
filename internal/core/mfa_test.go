// TOTP 등록과 복구 코드 및 HTTP 인증 대기 상태의 보안 경계를 검증한다.
package core

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/pquerna/otp/totp"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func authJSON(t *testing.T, w *httptest.ResponseRecorder, want int) map[string]any {
	t.Helper()
	status(t, w, want)
	var body map[string]any
	if e := json.Unmarshal(w.Body.Bytes(), &body); e != nil {
		t.Fatal(e)
	}
	return body
}
func reauth(t *testing.T, c *client, purpose, method, code string) string {
	t.Helper()
	return authJSON(t, c.request("POST", "/api/v1/auth/reauth", fmt.Sprintf(`{"purpose":%q,"password":%q,"method":%q,"code":%q}`, purpose, password, method, code)), 200)["proof"].(string)
}
func setupTOTP(t *testing.T, c *client, proof string) map[string]any {
	t.Helper()
	return authJSON(t, c.request("POST", "/api/v1/auth/totp/setup", fmt.Sprintf(`{"proof":%q}`, proof)), 200)
}
func otpCode(t *testing.T, secret string, offset time.Duration) string {
	t.Helper()
	code, e := totp.GenerateCode(secret, time.Now().Add(offset))
	if e != nil {
		t.Fatal(e)
	}
	return code
}
func enrollTOTP(t *testing.T, c *client) (string, []string) {
	t.Helper()
	proof := reauth(t, c, "totp-setup", "", "")
	setup := setupTOTP(t, c, proof)
	secret := setup["secret"].(string)
	body := authJSON(t, c.request("POST", "/api/v1/auth/totp/confirm", fmt.Sprintf(`{"proof":%q,"code":%q}`, proof, otpCode(t, secret, -30*time.Second))), 200)
	codes := []string{}
	for _, v := range body["recovery_codes"].([]any) {
		codes = append(codes, v.(string))
	}
	return secret, codes
}
func beginMFA(t *testing.T, c *client) []*http.Cookie {
	t.Helper()
	w := c.request("POST", "/api/v1/auth/login", fmt.Sprintf(`{"username":"admin","password":%q}`, password))
	body := authJSON(t, w, 202)
	if body["mfa_required"] != true {
		t.Fatal(body)
	}
	return w.Result().Cookies()
}
func finishMFA(c *client, cookies []*http.Cookie, method, code string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "http://localhost/api/v1/auth/mfa", strings.NewReader(fmt.Sprintf(`{"method":%q,"code":%q}`, method, code)))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://localhost")
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	c.s.Handler().ServeHTTP(w, r)
	return w
}
func acceptMFASession(t *testing.T, c *client, w *httptest.ResponseRecorder) {
	t.Helper()
	body := authJSON(t, w, 200)
	c.csrf = body["csrf_token"].(string)
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == cookieName {
			c.cookie = cookie
		}
	}
}

func TestTOTPEnrollmentRequiresConfirmation(t *testing.T) {
	c := newClient(t)
	c.login("admin")
	proof := reauth(t, c, "totp-setup", "", "")
	setup := setupTOTP(t, c, proof)
	security := authJSON(t, c.request("GET", "/api/v1/auth/security", ""), 200)
	if security["totp_enabled"] != false {
		t.Fatal("pending secret activated")
	}
	pngData, e := base64.StdEncoding.DecodeString(strings.TrimPrefix(setup["qr_png"].(string), "data:image/png;base64,"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = png.Decode(bytes.NewReader(pngData)); e != nil {
		t.Fatal("invalid QR image", e)
	}
	status(t, c.request("POST", "/api/v1/auth/totp/confirm", fmt.Sprintf(`{"proof":%q,"code":"invalid"}`, proof)), 401)
	secret := setup["secret"].(string)
	body := authJSON(t, c.request("POST", "/api/v1/auth/totp/confirm", fmt.Sprintf(`{"proof":%q,"code":%q}`, proof, otpCode(t, secret, 0))), 200)
	if body["csrf_token"] == nil || len(body["recovery_codes"].([]any)) != 10 {
		t.Fatal(body)
	}
	status(t, c.request("GET", "/api/v1/auth/me", ""), 401)
	cookies := beginMFA(t, c)
	status(t, finishMFA(c, cookies, "totp", otpCode(t, secret, 0)), 401)
}
func TestMFAIncompleteCannotAuthenticate(t *testing.T) {
	c := newClient(t)
	c.login("admin")
	_, codes := enrollTOTP(t, c)
	cookies := beginMFA(t, c)
	for _, cookie := range cookies {
		if cookie.Name == cookieName {
			t.Fatal("partial login issued session")
		}
		if !cookie.HttpOnly {
			t.Fatal("preauth cookie is readable")
		}
	}
	status(t, c.request("GET", "/api/v1/status", ""), 401)
	status(t, finishMFA(c, nil, "recovery", codes[0]), 401)
	acceptMFASession(t, c, finishMFA(c, cookies, "recovery", codes[0]))
	status(t, c.request("GET", "/api/v1/status", ""), 200)
	status(t, finishMFA(c, cookies, "recovery", codes[1]), 401)
}
func TestTOTPReplayAndConcurrentConsumption(t *testing.T) {
	c := newClient(t)
	c.login("admin")
	secret, _ := enrollTOTP(t, c)
	one, two := beginMFA(t, c), beginMFA(t, c)
	code := otpCode(t, secret, 0)
	var successes atomic.Int32
	var wg sync.WaitGroup
	for _, cookies := range [][]*http.Cookie{one, two} {
		wg.Go(func() {
			if finishMFA(c, cookies, "totp", code).Code == 200 {
				successes.Add(1)
			}
		})
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("TOTP accepted %d times", successes.Load())
	}
}
func TestRecoveryCodesSingleUseAndRegeneration(t *testing.T) {
	c := newClient(t)
	id := c.login("admin")["id"].(string)
	_, codes := enrollTOTP(t, c)
	if len(codes) != 10 {
		t.Fatal("wrong count")
	}
	for _, code := range codes {
		if len(strings.ReplaceAll(code, "-", "")) != 26 {
			t.Fatal("insufficient entropy")
		}
		var n int
		c.s.db.QueryRow("SELECT count(*) FROM recovery_codes WHERE hash=?", code).Scan(&n)
		if n != 0 {
			t.Fatal("plaintext recovery code persisted")
		}
	}
	cookies := beginMFA(t, c)
	acceptMFASession(t, c, finishMFA(c, cookies, "recovery", strings.ToLower(strings.ReplaceAll(codes[0], "-", ""))))
	proof := reauth(t, c, "recovery-regenerate", "recovery", codes[1])
	body := authJSON(t, c.request("POST", "/api/v1/auth/recovery-codes", fmt.Sprintf(`{"proof":%q}`, proof)), 200)
	if len(body["recovery_codes"].([]any)) != 10 {
		t.Fatal(body)
	}
	var remaining int
	c.s.db.QueryRow("SELECT count(*) FROM recovery_codes WHERE user_id=?", id).Scan(&remaining)
	if remaining != 10 {
		t.Fatal(remaining)
	}
	cookies = beginMFA(t, c)
	status(t, finishMFA(c, cookies, "recovery", codes[2]), 401)
	status(t, finishMFA(c, cookies, "recovery", codes[0]), 401)
}
func TestTOTPResetPreservesOldUntilConfirmed(t *testing.T) {
	c := newClient(t)
	id := c.login("admin")["id"].(string)
	old, codes := enrollTOTP(t, c)
	acceptMFASession(t, c, finishMFA(c, beginMFA(t, c), "recovery", codes[0]))
	proof := reauth(t, c, "totp-setup", "recovery", codes[1])
	next := setupTOTP(t, c, proof)
	var sealed []byte
	c.s.db.QueryRow("SELECT secret FROM user_totp WHERE user_id=?", id).Scan(&sealed)
	plain, e := c.s.openAuthSecret(id, "totp", sealed)
	if e != nil || string(plain) != old {
		t.Fatal("reset destroyed active secret")
	}
	status(t, c.request("DELETE", "/api/v1/auth/totp/setup", ""), 204)
	status(t, c.request("POST", "/api/v1/auth/totp/confirm", fmt.Sprintf(`{"proof":%q,"code":%q}`, proof, otpCode(t, next["secret"].(string), 0))), 403)
}
func TestSecurityRoutesRejectBearerAndCrossUser(t *testing.T) {
	c := newClient(t)
	c.login("admin")
	proof := reauth(t, c, "totp-setup", "", "")
	raw := token(t, c, "users:write")
	c.bearer = raw
	for _, tc := range []struct{ method, path, body string }{{"GET", "/auth/security", ""}, {"POST", "/auth/reauth", `{}`}, {"POST", "/auth/totp/setup", fmt.Sprintf(`{"proof":%q}`, proof)}} {
		status(t, c.request(tc.method, "/api/v1"+tc.path, tc.body), 403)
	}
	c.bearer = ""
	other := &client{t: t, s: c.s}
	other.login("admin")
	status(t, other.request("POST", "/api/v1/auth/totp/setup", fmt.Sprintf(`{"proof":%q}`, proof)), 403)
	r := httptest.NewRequest("POST", "http://localhost/api/v1/auth/reauth", strings.NewReader(`{}`))
	r.AddCookie(c.cookie)
	r.Header.Set("Origin", "https://evil.example")
	r.Header.Set("X-CSRF-Token", c.csrf)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c.s.Handler().ServeHTTP(w, r)
	status(t, w, 403)
}
func TestReauthExpiryDuringTOTPSetup(t *testing.T) {
	c := newClient(t)
	c.login("admin")
	proof := reauth(t, c, "totp-setup", "", "")
	setup := setupTOTP(t, c, proof)
	if _, e := c.s.db.Exec("UPDATE auth_challenges SET expires_at=? WHERE hash=?", time.Now().Add(-time.Second).UnixMilli(), hashToken(proof)); e != nil {
		t.Fatal(e)
	}
	code := otpCode(t, setup["secret"].(string), 0)
	status(t, c.request("POST", "/api/v1/auth/totp/confirm", fmt.Sprintf(`{"proof":%q,"code":%q}`, proof, code)), 403)
	next := reauth(t, c, "totp-setup", "", "")
	authJSON(t, c.request("POST", "/api/v1/auth/totp/confirm", fmt.Sprintf(`{"proof":%q,"code":%q}`, next, code)), 200)
}
func TestMFAFailureBudgetsSurviveNewPasswordLogin(t *testing.T) {
	c := newClient(t)
	id := c.login("admin")["id"].(string)
	enrollTOTP(t, c)
	cookies := beginMFA(t, c)
	for range 5 {
		status(t, finishMFA(c, cookies, "totp", "bad"), 401)
	}
	stateCount := 0
	c.s.db.QueryRowContext(context.Background(), "SELECT count(*) FROM auth_challenges WHERE user_id=? AND attempts<5", id).Scan(&stateCount)
	if stateCount != 0 {
		t.Fatal("exhausted challenge still active")
	}
	cookies = beginMFA(t, c)
	for range 3 {
		status(t, finishMFA(c, cookies, "totp", "bad"), 401)
	}
	cookies = beginMFA(t, c)
	status(t, finishMFA(c, cookies, "totp", "bad"), 429)
}
