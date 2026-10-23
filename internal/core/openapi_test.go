// OpenAPI의 인증 요청과 성공 상태가 실제 API 계약과 같은지 검증한다.
package core

import (
	"encoding/json"
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
