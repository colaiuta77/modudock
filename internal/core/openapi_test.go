// OpenAPI의 인증 요청과 성공 상태가 실제 API 계약과 같은지 검증한다.
package core

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOpenAPIAuthenticationContract(t *testing.T) {
	c := newClient(t)
	c.login("admin")
	w := c.request("GET", "/api/v1/openapi.json", "")
	status(t, w, 200)
	var doc struct {
		Paths map[string]map[string]struct {
			RequestBody any            `json:"requestBody"`
			Responses   map[string]any `json:"responses"`
		}
		Components map[string]any
	}
	if e := json.Unmarshal(w.Body.Bytes(), &doc); e != nil {
		t.Fatal(e)
	}
	if doc.Paths["/auth/login"]["post"].RequestBody == nil {
		t.Fatal("login request body undocumented")
	}
	if doc.Paths["/users"]["post"].Responses["201"] == nil {
		t.Fatal("create-user status incorrect")
	}
	if doc.Paths["/auth/password"]["put"].Responses["204"] == nil {
		t.Fatal("password status incorrect")
	}
	if doc.Components["securitySchemes"] == nil {
		t.Fatal("security schemes missing")
	}
}
func TestOpenAPIMFAContract(t *testing.T) {
	c := newClient(t)
	paths := c.s.apiDocument()["paths"].(map[string]any)
	for _, path := range []string{"/auth/security", "/auth/reauth", "/auth/totp/setup", "/auth/totp/confirm", "/auth/totp", "/auth/recovery-codes", "/auth/mfa", "/auth/login/cancel", "/auth/options"} {
		if paths[path] == nil {
			t.Errorf("missing %s", path)
		}
	}
	login := paths["/auth/login"].(map[string]any)["post"].(map[string]any)
	if login["responses"].(map[string]any)["202"] == nil {
		t.Error("missing MFA challenge response")
	}
}
func TestOpenAPIPasskeyContract(t *testing.T) {
	c := newClient(t)
	paths := c.s.apiDocument()["paths"].(map[string]any)
	for _, path := range []string{"/auth/passkey/begin", "/auth/passkey/finish", "/auth/passkeys/register/begin", "/auth/passkeys/register/finish", "/auth/passkeys/reauth/begin", "/auth/passkeys/reauth/finish", "/auth/passkeys/{id}"} {
		if paths[path] == nil {
			t.Errorf("missing %s", path)
		}
	}
}
func TestOpenAPIPluginQueueScheduleContract(t *testing.T) {
	c := newClient(t)
	doc := c.s.apiDocument()
	paths := doc["paths"].(map[string]any)
	for _, tc := range []struct{ path, method, status string }{
		{"/plugins/install", "post", "201"}, {"/plugins/{id}/update", "post", "200"}, {"/plugins/{id}/activate", "post", "200"}, {"/plugins/{id}/releases", "get", "200"}, {"/plugins/{id}/settings", "put", "200"}, {"/menus/{plugin_id}/{menu_id}", "put", "200"}, {"/plugins/{id}/tasks/{task_id}/runs", "post", "201"}, {"/runs/{id}/cancel", "post", "200"}, {"/schedules", "post", "201"}, {"/schedules/{id}", "put", "200"}, {"/plugins/{id}/resources/{resource}", "get", "200"},
	} {
		path, ok := paths[tc.path].(map[string]any)
		if !ok {
			t.Errorf("missing path %s", tc.path)
			continue
		}
		op, ok := path[tc.method].(map[string]any)
		if !ok {
			t.Errorf("missing operation %s %s", tc.method, tc.path)
			continue
		}
		responses := op["responses"].(map[string]any)
		if responses[tc.status] == nil {
			t.Errorf("missing success %s %s", tc.status, tc.path)
		}
		params, _ := op["parameters"].([]any)
		for _, name := range []string{"plugin_id", "menu_id", "task_id", "resource"} {
			if strings.Contains(tc.path, "{"+name+"}") {
				found := false
				for _, param := range params {
					p := param.(map[string]any)
					if p["name"] == name && p["in"] == "path" {
						found = true
					}
				}
				if !found {
					t.Errorf("undocumented path parameter %s %s", tc.path, name)
				}
			}
		}
	}
	schemas := doc["components"].(map[string]any)["schemas"].(map[string]any)
	for _, name := range []string{"Manifest", "Plugin", "Run", "Schedule", "Menu", "Release"} {
		if schemas[name] == nil {
			t.Errorf("missing schema %s", name)
		}
	}
}
