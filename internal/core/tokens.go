// 만료와 권한 범위를 가진 해시 저장 API 토큰을 관리한다.
package core

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type TokenMetadata struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Scopes    []string `json:"scopes"`
	ExpiresAt string   `json:"expires_at"`
	CreatedAt string   `json:"created_at"`
}

var allowedScopes = map[string]bool{"status:read": true, "plugins:read": true, "plugins:write": true, "runs:read": true, "runs:write": true, "schedules:read": true, "schedules:write": true, "logs:read": true, "users:write": true, "tokens:write": true}

func (s *Server) createToken(w http.ResponseWriter, r *http.Request, p principal) {
	var body struct {
		Name      string   `json:"name"`
		Scopes    []string `json:"scopes"`
		ExpiresAt string   `json:"expires_at"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	expiry, e := time.Parse(time.RFC3339, body.ExpiresAt)
	if e != nil || !expiry.After(time.Now()) || len(strings.TrimSpace(body.Name)) == 0 || len(body.Name) > 100 || len(body.Scopes) == 0 || len(body.Scopes) > len(allowedScopes) {
		apiError(w, 400, "invalid_input", "Token requires name, scopes and future expiry")
		return
	}
	seen := map[string]bool{}
	for _, scope := range body.Scopes {
		if !allowedScopes[scope] || seen[scope] || (p.TokenID != "" && !p.Scopes[scope]) {
			apiError(w, 400, "invalid_scope", "Invalid, duplicate or unavailable scope")
			return
		}
		seen[scope] = true
	}
	raw := randomID() + randomID()
	meta := TokenMetadata{ID: randomID(), Name: body.Name, Scopes: body.Scopes, ExpiresAt: expiry.UTC().Format(time.RFC3339), CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	scopes, _ := json.Marshal(body.Scopes)
	_, e = s.db.ExecContext(r.Context(), "INSERT INTO tokens(id,hash,user_id,name,scopes,expires_at,created_at) VALUES(?,?,?,?,?,?,?)", meta.ID, hashToken(raw), p.User.ID, meta.Name, string(scopes), meta.ExpiresAt, meta.CreatedAt)
	if e != nil {
		internalError(w)
		return
	}
	s.audit(r, "token_created", p.User.ID)
	writeJSON(w, 201, map[string]any{"token": raw, "metadata": meta})
}
func (s *Server) listTokens(w http.ResponseWriter, r *http.Request, p principal) {
	limit, offset, ok := pagination(w, r)
	if !ok {
		return
	}
	var total int
	if e := s.db.QueryRowContext(r.Context(), "SELECT count(*) FROM tokens").Scan(&total); e != nil {
		internalError(w)
		return
	}
	rows, e := s.db.QueryContext(r.Context(), "SELECT id,name,scopes,expires_at,created_at FROM tokens ORDER BY created_at,id LIMIT ? OFFSET ?", limit, offset)
	if e != nil {
		internalError(w)
		return
	}
	defer rows.Close()
	items := []TokenMetadata{}
	for rows.Next() {
		var m TokenMetadata
		var scopes string
		if e = rows.Scan(&m.ID, &m.Name, &scopes, &m.ExpiresAt, &m.CreatedAt); e != nil {
			internalError(w)
			return
		}
		if e = jsonUnmarshal(scopes, &m.Scopes); e != nil {
			internalError(w)
			return
		}
		items = append(items, m)
	}
	if rows.Err() != nil {
		internalError(w)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": total})
}
func (s *Server) deleteToken(w http.ResponseWriter, r *http.Request, p principal) {
	result, e := s.db.ExecContext(r.Context(), "DELETE FROM tokens WHERE id=?", r.PathValue("id"))
	if e != nil {
		internalError(w)
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		apiError(w, 404, "not_found", "Token not found")
		return
	}
	s.audit(r, "token_deleted", p.User.ID)
	w.WriteHeader(204)
}
