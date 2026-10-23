// 자격 폐기와 지연된 토큰 발급의 순서를 고정해 권한 경합을 검증한다.
package core

import (
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type gatedBody struct {
	io.Reader
	entered, resume chan struct{}
	once            sync.Once
}

func (b *gatedBody) Read(p []byte) (int, error) {
	b.once.Do(func() { close(b.entered); <-b.resume })
	return b.Reader.Read(p)
}

func TestDelayedTokenCreationRejectsRevokedCredentials(t *testing.T) {
	for _, scenario := range []string{"session_password_reset", "session_logout", "session_expired", "token_deleted", "token_expired", "account_inactive", "account_demoted"} {
		t.Run(scenario, func(t *testing.T) {
			admin := newClient(t)
			admin.login("admin")
			w := admin.request("POST", "/api/v1/users", fmt.Sprintf(`{"username":"subject","password":%q,"role":"admin"}`, password))
			status(t, w, 201)
			subject := &client{t: t, s: admin.s}
			user := subject.login("subject")
			id := user["id"].(string)
			if strings.HasPrefix(scenario, "token_") {
				subject.bearer = token(t, subject, "tokens:write")
				subject.cookie = nil
			}
			body := &gatedBody{Reader: strings.NewReader(fmt.Sprintf(`{"name":"delayed","scopes":["tokens:write"],"expires_at":%q}`, time.Now().Add(time.Hour).UTC().Format(time.RFC3339))), entered: make(chan struct{}), resume: make(chan struct{})}
			request := httptest.NewRequest("POST", "http://localhost/api/v1/tokens", body)
			request.Header.Set("Origin", "http://localhost")
			request.Header.Set("Content-Type", "application/json")
			if subject.cookie != nil {
				request.AddCookie(subject.cookie)
				request.Header.Set("X-CSRF-Token", subject.csrf)
			} else {
				request.Header.Set("Authorization", "Bearer "+subject.bearer)
			}
			response := httptest.NewRecorder()
			finished := make(chan struct{})
			go func() { defer close(finished); admin.s.Handler().ServeHTTP(response, request) }()
			var resumeOnce sync.Once
			resume := func() { resumeOnce.Do(func() { close(body.resume) }) }
			defer resume()
			select {
			case <-body.entered:
			case <-time.After(5 * time.Second):
				t.Fatal("request did not reach body after authentication")
			}
			switch scenario {
			case "session_password_reset":
				status(t, admin.request("PUT", "/api/v1/users/"+id, `{"username":"subject","role":"admin","is_active":true,"password":"replacement-password"}`), 200)
			case "session_logout":
				status(t, subject.request("POST", "/api/v1/auth/logout", ""), 204)
			case "session_expired":
				if _, e := admin.s.db.Exec("UPDATE sessions SET expires_at='2000-01-01T00:00:00Z' WHERE hash=?", hashToken(subject.cookie.Value)); e != nil {
					t.Fatal(e)
				}
			case "token_deleted":
				var tokenID string
				if e := admin.s.db.QueryRow("SELECT id FROM tokens WHERE hash=?", hashToken(subject.bearer)).Scan(&tokenID); e != nil {
					t.Fatal(e)
				}
				status(t, admin.request("DELETE", "/api/v1/tokens/"+tokenID, ""), 204)
			case "token_expired":
				if _, e := admin.s.db.Exec("UPDATE tokens SET expires_at='2000-01-01T00:00:00Z' WHERE hash=?", hashToken(subject.bearer)); e != nil {
					t.Fatal(e)
				}
			case "account_inactive":
				status(t, admin.request("PUT", "/api/v1/users/"+id, `{"username":"subject","role":"admin","is_active":false}`), 200)
			case "account_demoted":
				status(t, admin.request("PUT", "/api/v1/users/"+id, `{"username":"subject","role":"operator","is_active":true}`), 200)
			}
			resume()
			select {
			case <-finished:
			case <-time.After(5 * time.Second):
				t.Fatal("delayed request did not finish")
			}
			status(t, response, 401)
			var created int
			if e := admin.s.db.QueryRow("SELECT count(*) FROM tokens WHERE name='delayed'").Scan(&created); e != nil {
				t.Fatal(e)
			}
			if created != 0 {
				t.Fatalf("revoked credential minted %d token(s)", created)
			}
		})
	}
}
