// 실제 HTTP와 SQLite로 인증 보안 경계를 검증한다.
package core

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEventsBroadcastToAllClients(t *testing.T) {
	c := newClient(t)
	c.login("admin")
	server := httptest.NewServer(c.s.Handler())
	defer server.Close()
	readers := []*bufio.Reader{}
	for i := 0; i < 2; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		request, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/v1/events", nil)
		request.AddCookie(c.cookie)
		response, e := server.Client().Do(request)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		reader := bufio.NewReader(response.Body)
		for j := 0; j < 3; j++ {
			if _, e = reader.ReadString('\n'); e != nil {
				t.Fatal(e)
			}
		}
		readers = append(readers, reader)
	}
	c.s.notify()
	for i, reader := range readers {
		line, e := reader.ReadString('\n')
		if e != nil || line != "event: change\n" {
			t.Fatalf("client %d missed change: %q %v", i, line, e)
		}
	}
}
func TestRejectInvalidContentType(t *testing.T) {
	c := newClient(t)
	r := httptest.NewRequest("POST", "http://localhost/api/v1/auth/login", strings.NewReader(fmt.Sprintf(`{"username":"admin","password":%q}`, password)))
	r.Header.Set("Origin", "http://localhost")
	r.Header.Set("Content-Type", "application/jsonevil")
	w := httptest.NewRecorder()
	c.s.Handler().ServeHTTP(w, r)
	status(t, w, 415)
}
func TestRoleAndPaginationBoundaries(t *testing.T) {
	c := newClient(t)
	c.login("admin")
	for _, role := range []string{"operator", "viewer"} {
		status(t, c.request("POST", "/api/v1/users", fmt.Sprintf(`{"username":%q,"password":%q,"role":%q}`, role, password, role)), 201)
		other := &client{t: t, s: c.s}
		other.login(role)
		status(t, other.request("GET", "/api/v1/status", ""), 200)
		status(t, other.request("GET", "/api/v1/users", ""), 403)
		status(t, other.request("GET", "/api/v1/logs", ""), 403)
		want := 200
		if role == "viewer" {
			want = 403
		}
		status(t, other.request("GET", "/api/v1/logs?run_id=run-1", ""), want)
	}
	for _, path := range []string{"/api/v1/users?limit=201", "/api/v1/users?offset=-1", "/api/v1/users?limit=1%20OR%201=1"} {
		status(t, c.request("GET", path, ""), 400)
	}
	status(t, c.request("POST", "/api/v1/users", fmt.Sprintf(`{"username":"admin' OR 1=1--","password":%q,"role":"admin"}`, password)), 400)
	status(t, c.request("GET", "/api/v1/unknown", ""), 404)
}

func TestUserUpdateRejectsNullPassword(t *testing.T) {
	c := newClient(t)
	u := c.login("admin")
	status(t, c.request("PUT", "/api/v1/users/"+u["id"].(string), `{"username":"admin","role":"admin","is_active":true,"password":null}`), 400)
}
func TestSuccessfulLoginsDoNotConsumeFailureBudget(t *testing.T) {
	c := newClient(t)
	for i := 0; i < 10; i++ {
		c.login("admin")
	}
}

func TestPasswordOperationsShareConcurrencyBudget(t *testing.T) {
	for _, operation := range []string{"viewer_wrong_password", "viewer_password_change", "user_create", "user_password_reset"} {
		t.Run(operation, func(t *testing.T) {
			admin := newClient(t)
			admin.login("admin")
			status(t, admin.request("POST", "/api/v1/users", fmt.Sprintf(`{"username":"viewer","password":%q,"role":"viewer"}`, password)), 201)
			viewer := &client{t: t, s: admin.s}
			user := viewer.login("viewer")
			caller := admin
			method, path, body, want := "PUT", "/api/v1/users/"+user["id"].(string), `{"username":"viewer","role":"viewer","is_active":true,"password":"replacement-password"}`, 200
			switch operation {
			case "viewer_wrong_password":
				caller = viewer
				path = "/api/v1/auth/password"
				body = `{"current_password":"incorrect","new_password":"replacement-password"}`
				want = 403
			case "viewer_password_change":
				caller = viewer
				path = "/api/v1/auth/password"
				body = fmt.Sprintf(`{"current_password":%q,"new_password":"replacement-password"}`, password)
				want = 204
			case "user_create":
				method = "POST"
				path = "/api/v1/users"
				body = fmt.Sprintf(`{"username":"newuser","password":%q,"role":"viewer"}`, password)
				want = 201
			}
			for i := 0; i < cap(admin.s.passwordSlots); i++ {
				admin.s.passwordSlots <- struct{}{}
			}
			response := caller.request(method, path, body)
			for i := 0; i < cap(admin.s.passwordSlots); i++ {
				<-admin.s.passwordSlots
			}
			status(t, response, 429)
			if response.Header().Get("Retry-After") == "" {
				t.Fatal("busy password operation missing retry hint")
			}
			status(t, caller.request(method, path, body), want)
			if len(admin.s.passwordSlots) != 0 {
				t.Fatal("completed password operation leaked its reservation")
			}
		})
	}
}
func TestNonPasswordUserUpdateDoesNotConsumeConcurrencyBudget(t *testing.T) {
	c := newClient(t)
	user := c.login("admin")
	for i := 0; i < cap(c.s.passwordSlots); i++ {
		c.s.passwordSlots <- struct{}{}
	}
	defer func() {
		for i := 0; i < cap(c.s.passwordSlots); i++ {
			<-c.s.passwordSlots
		}
	}()
	status(t, c.request("PUT", "/api/v1/users/"+user["id"].(string), `{"username":"admin","role":"admin","is_active":true}`), 200)
}
func TestPublicOriginAndSecureCookie(t *testing.T) {
	s, e := Open(t.TempDir(), Options{PublicURL: "https://console.test"})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if e = s.CreateAdmin("admin", password); e != nil {
		t.Fatal(e)
	}
	for _, origin := range []string{"http://localhost", "https://evil.test", "https://console.test"} {
		r := httptest.NewRequest("POST", "http://internal/api/v1/auth/login", strings.NewReader(fmt.Sprintf(`{"username":"admin","password":%q}`, password)))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", origin)
		r.Header.Set("X-Forwarded-Host", "console.test")
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		want := 403
		if origin == "https://console.test" {
			want = 200
		}
		status(t, w, want)
		if want == 200 && !w.Result().Cookies()[0].Secure {
			t.Fatal("HTTPS public URL requires Secure cookie")
		}
	}
}

func TestExpiredCredentialsRemovedOnReopen(t *testing.T) {
	dir := t.TempDir()
	s, e := Open(dir, Options{})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.CreateAdmin("admin", password); e != nil {
		t.Fatal(e)
	}
	var id string
	s.db.QueryRow("SELECT id FROM users").Scan(&id)
	_, e = s.db.Exec("INSERT INTO sessions(hash,user_id,csrf,expires_at) VALUES('stale',?,'csrf','2000-01-01T00:00:00Z')", id)
	if e != nil {
		t.Fatal(e)
	}
	s.Close()
	s, e = Open(dir, Options{})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	var count int
	s.db.QueryRow("SELECT count(*) FROM sessions").Scan(&count)
	if count != 0 {
		t.Fatalf("expired sessions remain: %d", count)
	}
}
func TestAccountChangesRevokeCredentials(t *testing.T) {
	for _, change := range []string{`{"username":"second","role":"operator","is_active":true}`, `{"username":"second","role":"admin","is_active":false}`, `{"username":"second","role":"admin","is_active":true,"password":"replacement-password"}`} {
		t.Run(change, func(t *testing.T) {
			c := newClient(t)
			c.login("admin")
			w := c.request("POST", "/api/v1/users", fmt.Sprintf(`{"username":"second","password":%q,"role":"admin"}`, password))
			status(t, w, 201)
			var user User
			json.Unmarshal(w.Body.Bytes(), &user)
			other := &client{t: t, s: c.s}
			other.login("second")
			raw := token(t, other, "status:read")
			status(t, c.request("PUT", "/api/v1/users/"+user.ID, change), 200)
			status(t, other.request("GET", "/api/v1/auth/me", ""), 401)
			other.cookie = nil
			other.bearer = raw
			status(t, other.request("GET", "/api/v1/auth/me", ""), 401)
		})
	}
}
func TestTokenHashExpiryAndDeletion(t *testing.T) {
	c := newClient(t)
	c.login("admin")
	raw := token(t, c, "status:read")
	var id, hash string
	if e := c.s.db.QueryRow("SELECT id,hash FROM tokens").Scan(&id, &hash); e != nil {
		t.Fatal(e)
	}
	if hash == raw || len(hash) != 64 {
		t.Fatal("raw token stored")
	}
	external := &client{t: t, s: c.s, bearer: raw}
	status(t, external.request("GET", "/api/v1/auth/me", ""), 200)
	_, e := c.s.db.Exec("UPDATE tokens SET expires_at='2000-01-01T00:00:00Z' WHERE id=?", id)
	if e != nil {
		t.Fatal(e)
	}
	status(t, external.request("GET", "/api/v1/auth/me", ""), 401)
	status(t, c.request("DELETE", "/api/v1/tokens/"+id, ""), 204)
	logs, e := os.ReadFile(filepath.Join(c.s.dataDir, "logs", "core.jsonl"))
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(logs), raw) || strings.Contains(string(logs), password) {
		t.Fatal("secret present in logs")
	}
}

const password = "test-password-1234"

type client struct {
	t            *testing.T
	s            *Server
	cookie       *http.Cookie
	csrf, bearer string
}

func newClient(t *testing.T) *client {
	t.Helper()
	s, e := Open(t.TempDir(), Options{Version: "test"})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	if e = s.CreateAdmin("admin", password); e != nil {
		t.Fatal(e)
	}
	return &client{t: t, s: s}
}
func (c *client) request(method, path, body string) *httptest.ResponseRecorder {
	c.t.Helper()
	r := httptest.NewRequest(method, "http://localhost"+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://localhost")
	if c.cookie != nil {
		r.AddCookie(c.cookie)
		r.Header.Set("X-CSRF-Token", c.csrf)
	}
	if c.bearer != "" {
		r.Header.Set("Authorization", "Bearer "+c.bearer)
	}
	w := httptest.NewRecorder()
	c.s.Handler().ServeHTTP(w, r)
	return w
}
func (c *client) login(name string) map[string]any {
	c.t.Helper()
	w := c.request("POST", "/api/v1/auth/login", fmt.Sprintf(`{"username":%q,"password":%q}`, name, password))
	status(c.t, w, 200)
	var b map[string]any
	if e := json.Unmarshal(w.Body.Bytes(), &b); e != nil {
		c.t.Fatal(e)
	}
	c.csrf = b["csrf_token"].(string)
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		c.t.Fatal("missing cookie")
	}
	c.cookie = cookies[0]
	return b["user"].(map[string]any)
}
func status(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("want %d got %d %s", want, w.Code, w.Body)
	}
}
func token(t *testing.T, c *client, scope string) string {
	t.Helper()
	w := c.request("POST", "/api/v1/tokens", fmt.Sprintf(`{"name":"automation","scopes":[%q],"expires_at":%q}`, scope, time.Now().Add(time.Hour).UTC().Format(time.RFC3339)))
	status(t, w, 201)
	var b struct{ Token string }
	if e := json.Unmarshal(w.Body.Bytes(), &b); e != nil {
		t.Fatal(e)
	}
	return b.Token
}
func TestLoginLogoutAndCSRF(t *testing.T) {
	c := newClient(t)
	c.login("admin")
	if !c.cookie.HttpOnly || c.cookie.SameSite != http.SameSiteStrictMode {
		t.Fatal("unsafe cookie")
	}
	status(t, c.request("GET", "/api/v1/auth/me", ""), 200)
	saved := c.csrf
	c.csrf = "wrong"
	status(t, c.request("POST", "/api/v1/auth/logout", ""), 403)
	c.csrf = saved
	r := httptest.NewRequest("POST", "http://localhost/api/v1/auth/logout", nil)
	r.AddCookie(c.cookie)
	r.Header.Set("X-CSRF-Token", c.csrf)
	r.Header.Set("Origin", "https://evil.test")
	r.Header.Set("X-Forwarded-Host", "evil.test")
	w := httptest.NewRecorder()
	c.s.Handler().ServeHTTP(w, r)
	status(t, w, 403)
	status(t, c.request("POST", "/api/v1/auth/logout", ""), 204)
	status(t, c.request("GET", "/api/v1/auth/me", ""), 401)
}
func TestStrictJSONAndLoginLimit(t *testing.T) {
	c := newClient(t)
	for _, body := range []string{`{"username":"admin","password":"x","extra":true}`, `{"username":1}`, `{} {}`, strings.Repeat("x", 70000)} {
		w := c.request("POST", "/api/v1/auth/login", body)
		if w.Code != 400 && w.Code != 413 {
			t.Fatalf("malformed accepted: %d", w.Code)
		}
	}
	for i := 0; i < 10; i++ {
		w := c.request("POST", "/api/v1/auth/login", `{"username":"admin","password":"bad"}`)
		if i == 9 {
			status(t, w, 429)
		}
	}
}
func TestTokenScopesExpiryAndHash(t *testing.T) {
	c := newClient(t)
	c.login("admin")
	raw := token(t, c, "status:read")
	c.bearer = raw
	c.cookie = nil
	status(t, c.request("GET", "/api/v1/status", ""), 200)
	status(t, c.request("GET", "/api/v1/auth/me", ""), 200)
	status(t, c.request("GET", "/api/v1/users", ""), 403)
	c.bearer = "invalid"
	status(t, c.request("GET", "/api/v1/status", ""), 401)
}
func TestLastAdminAndPasswordRevocation(t *testing.T) {
	c := newClient(t)
	u := c.login("admin")
	id := u["id"].(string)
	status(t, c.request("PUT", "/api/v1/users/"+id, `{"username":"admin","role":"viewer","is_active":true}`), 409)
	status(t, c.request("DELETE", "/api/v1/users/"+id, ""), 409)
	raw := token(t, c, "status:read")
	status(t, c.request("PUT", "/api/v1/auth/password", fmt.Sprintf(`{"current_password":%q,"new_password":"new-password-12345"}`, password)), 204)
	status(t, c.request("GET", "/api/v1/auth/me", ""), 401)
	c.cookie = nil
	c.bearer = raw
	status(t, c.request("GET", "/api/v1/status", ""), 401)
}
func TestDataDirLockAndInvalidPublicURL(t *testing.T) {
	dir := t.TempDir()
	s, e := Open(dir, Options{})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	other, e := Open(dir, Options{})
	if e == nil {
		other.Close()
		t.Fatal("second core acquired same data directory")
	}
	if s, e := Open(t.TempDir(), Options{PublicURL: "https://example.com/path"}); e == nil {
		s.Close()
		t.Fatal("path accepted as origin")
	}
}
