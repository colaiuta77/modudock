// 사용자 관리와 마지막 활성 관리자 보호 규칙을 적용한다.
package core

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

type User struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	IsActive  bool   `json:"is_active"`
	CreatedAt string `json:"created_at"`
}

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

func validUsername(s string) bool         { return usernamePattern.MatchString(s) }
func jsonUnmarshal(s string, v any) error { return json.Unmarshal([]byte(s), v) }
func pagination(w http.ResponseWriter, r *http.Request) (int, int, bool) {
	limit, offset := 50, 0
	var e error
	if value := r.URL.Query().Get("limit"); value != "" {
		limit, e = strconv.Atoi(value)
		if e != nil || limit < 1 || limit > 200 {
			apiError(w, 400, "invalid_pagination", "Limit must be 1 to 200")
			return 0, 0, false
		}
	}
	if value := r.URL.Query().Get("offset"); value != "" {
		offset, e = strconv.Atoi(value)
		if e != nil || offset < 0 {
			apiError(w, 400, "invalid_pagination", "Offset must be nonnegative")
			return 0, 0, false
		}
	}
	return limit, offset, true
}
func (s *Server) listUsers(w http.ResponseWriter, r *http.Request, p principal) {
	limit, offset, ok := pagination(w, r)
	if !ok {
		return
	}
	var total int
	if e := s.db.QueryRowContext(r.Context(), "SELECT count(*) FROM users").Scan(&total); e != nil {
		internalError(w)
		return
	}
	rows, e := s.db.QueryContext(r.Context(), "SELECT id,username,role,is_active,created_at FROM users ORDER BY created_at,id LIMIT ? OFFSET ?", limit, offset)
	if e != nil {
		internalError(w)
		return
	}
	defer rows.Close()
	items := []User{}
	for rows.Next() {
		var u User
		if e = rows.Scan(&u.ID, &u.Username, &u.Role, &u.IsActive, &u.CreatedAt); e != nil {
			internalError(w)
			return
		}
		items = append(items, u)
	}
	if rows.Err() != nil {
		internalError(w)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": total})
}
func (s *Server) createUser(w http.ResponseWriter, r *http.Request, p principal) {
	var body struct{ Username, Password, Role string }
	if !decodeJSON(w, r, &body) {
		return
	}
	if !validUsername(body.Username) || !validPassword(body.Password) || roleRank(body.Role) == 0 {
		apiError(w, 400, "invalid_input", "Invalid username, password or role")
		return
	}
	u := User{ID: randomID(), Username: body.Username, Role: body.Role, IsActive: true, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	_, e := s.db.ExecContext(r.Context(), "INSERT INTO users(id,username,password_hash,role,created_at) VALUES(?,?,?,?,?)", u.ID, u.Username, hashPassword(body.Password), u.Role, u.CreatedAt)
	if e != nil {
		apiError(w, 409, "conflict", "Username already exists")
		return
	}
	s.audit(r, "user_created", p.User.ID)
	s.notify()
	writeJSON(w, 201, u)
}
func (s *Server) updateUser(w http.ResponseWriter, r *http.Request, p principal) {
	var body struct {
		Username, Role string
		IsActive       *bool           `json:"is_active"`
		Password       json.RawMessage `json:"password"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	var password *string
	if len(body.Password) > 0 {
		var value string
		if string(body.Password) == "null" || json.Unmarshal(body.Password, &value) != nil {
			apiError(w, 400, "invalid_input", "Password must be a string")
			return
		}
		password = &value
	}
	if !validUsername(body.Username) || roleRank(body.Role) == 0 || body.IsActive == nil || (password != nil && !validPassword(*password)) {
		apiError(w, 400, "invalid_input", "Invalid user fields")
		return
	}
	encoded := ""
	if password != nil {
		encoded = hashPassword(*password)
	}
	id := r.PathValue("id")
	tx, e := s.db.BeginTx(r.Context(), nil)
	if e != nil {
		internalError(w)
		return
	}
	defer tx.Rollback()
	var u User
	var oldHash string
	e = tx.QueryRowContext(r.Context(), "SELECT id,username,role,is_active,created_at,password_hash FROM users WHERE id=?", id).Scan(&u.ID, &u.Username, &u.Role, &u.IsActive, &u.CreatedAt, &oldHash)
	if e == sql.ErrNoRows {
		apiError(w, 404, "not_found", "User not found")
		return
	}
	if e != nil {
		internalError(w)
		return
	}
	if u.Role == "admin" && u.IsActive && (body.Role != "admin" || !*body.IsActive) {
		var admins int
		if e = tx.QueryRow("SELECT count(*) FROM users WHERE role='admin' AND is_active=1").Scan(&admins); e != nil {
			internalError(w)
			return
		}
		if admins <= 1 {
			apiError(w, 409, "last_admin", "Cannot remove last active administrator")
			return
		}
	}
	revoke := u.Role != body.Role || u.IsActive != *body.IsActive || password != nil
	if encoded == "" {
		encoded = oldHash
	}
	if _, e = tx.ExecContext(r.Context(), "UPDATE users SET username=?,role=?,is_active=?,password_hash=? WHERE id=?", body.Username, body.Role, *body.IsActive, encoded, id); e != nil {
		apiError(w, 409, "conflict", "Username already exists")
		return
	}
	if revoke {
		if e = revokeCredentials(tx, id); e != nil {
			internalError(w)
			return
		}
	}
	if e = tx.Commit(); e != nil {
		internalError(w)
		return
	}
	u.Username, u.Role, u.IsActive = body.Username, body.Role, *body.IsActive
	s.audit(r, "user_updated", p.User.ID)
	s.notify()
	writeJSON(w, 200, u)
}
func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request, p principal) {
	id := r.PathValue("id")
	if id == p.User.ID {
		apiError(w, 409, "self_delete", "Cannot delete own account")
		return
	}
	tx, e := s.db.BeginTx(r.Context(), nil)
	if e != nil {
		internalError(w)
		return
	}
	defer tx.Rollback()
	var role string
	var active bool
	e = tx.QueryRow("SELECT role,is_active FROM users WHERE id=?", id).Scan(&role, &active)
	if e == sql.ErrNoRows {
		apiError(w, 404, "not_found", "User not found")
		return
	}
	if e != nil {
		internalError(w)
		return
	}
	if role == "admin" && active {
		var admins int
		if e = tx.QueryRow("SELECT count(*) FROM users WHERE role='admin' AND is_active=1").Scan(&admins); e != nil {
			internalError(w)
			return
		}
		if admins <= 1 {
			apiError(w, 409, "last_admin", "Cannot remove last active administrator")
			return
		}
	}
	if _, e = tx.Exec("DELETE FROM users WHERE id=?", id); e != nil {
		internalError(w)
		return
	}
	if e = tx.Commit(); e != nil {
		internalError(w)
		return
	}
	s.audit(r, "user_deleted", p.User.ID)
	s.notify()
	w.WriteHeader(204)
}
