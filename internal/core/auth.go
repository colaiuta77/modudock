// 비밀번호 검증과 세션 인증 및 로그인 제한을 담당한다.
package core

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"golang.org/x/crypto/argon2"
	"net"
	"net/http"
	"strings"
	"time"
)

const cookieName = "modudock_session"

type loginAttempt struct {
	Count int
	Until time.Time
}

func randomID() string            { return rand.Text() }
func hashToken(raw string) string { h := sha256.Sum256([]byte(raw)); return hex.EncodeToString(h[:]) }
func constantEqual(a, b string) bool {
	return len(a) > 0 && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
func hashPassword(password string) string {
	salt := make([]byte, 16)
	if _, e := rand.Read(salt); e != nil {
		panic(e)
	}
	key := argon2.IDKey([]byte(password), salt, 1, 64*1024, 2, 32)
	return "$argon2id$v=19$m=65536,t=1,p=2$" + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(key)
}
func checkPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" || parts[3] != "m=65536,t=1,p=2" {
		return false
	}
	salt, e := base64.RawStdEncoding.DecodeString(parts[4])
	if e != nil || len(salt) != 16 {
		return false
	}
	want, e := base64.RawStdEncoding.DecodeString(parts[5])
	if e != nil || len(want) != 32 {
		return false
	}
	key := argon2.IDKey([]byte(password), salt, 1, 64*1024, 2, 32)
	return subtle.ConstantTimeCompare(key, want) == 1
}
func validPassword(password string) bool { return len(password) >= 12 && len(password) <= 1024 }
func (s *Server) reservePassword(w http.ResponseWriter) bool {
	select {
	case s.passwordSlots <- struct{}{}:
		return true
	default:
		w.Header().Set("Retry-After", "1")
		apiError(w, 429, "rate_limited", "Authentication busy")
		return false
	}
}
func (s *Server) authenticate(r *http.Request) (principal, error) {
	var p principal
	var expires, scopes string
	var e error
	auth := r.Header.Get("Authorization")
	if auth != "" {
		if !strings.HasPrefix(auth, "Bearer ") || len(auth) > 256 {
			return p, errors.New("invalid authorization")
		}
		e = s.db.QueryRowContext(r.Context(), `SELECT u.id,u.username,u.role,u.is_active,u.created_at,t.id,t.scopes,t.expires_at FROM tokens t JOIN users u ON u.id=t.user_id WHERE t.hash=?`, hashToken(strings.TrimPrefix(auth, "Bearer "))).Scan(&p.User.ID, &p.User.Username, &p.User.Role, &p.User.IsActive, &p.User.CreatedAt, &p.TokenID, &scopes, &expires)
		if e == nil {
			var list []string
			if e = jsonUnmarshal(scopes, &list); e == nil {
				p.Scopes = map[string]bool{}
				for _, scope := range list {
					p.Scopes[scope] = true
				}
			}
		}
	} else {
		cookie, err := r.Cookie(cookieName)
		if err != nil || len(cookie.Value) > 256 {
			return p, errors.New("missing session")
		}
		p.SessionHash = hashToken(cookie.Value)
		e = s.db.QueryRowContext(r.Context(), `SELECT u.id,u.username,u.role,u.is_active,u.created_at,s.csrf,s.expires_at FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.hash=?`, p.SessionHash).Scan(&p.User.ID, &p.User.Username, &p.User.Role, &p.User.IsActive, &p.User.CreatedAt, &p.CSRF, &expires)
	}
	if e != nil {
		return p, e
	}
	expiry, e := time.Parse(time.RFC3339, expires)
	if e != nil || !expiry.After(time.Now()) || !p.User.IsActive {
		return principal{}, errors.New("expired credentials")
	}
	return p, nil
}
func loginIP(r *http.Request) string {
	ip, _, e := net.SplitHostPort(r.RemoteAddr)
	if e != nil {
		ip = r.RemoteAddr
	}
	return ip
}
func (s *Server) releaseLogin(r *http.Request) {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	ip := loginIP(r)
	a := s.attempts[ip]
	a.Count--
	if a.Count <= 0 {
		delete(s.attempts, ip)
	} else {
		s.attempts[ip] = a
	}
}
func (s *Server) allowLogin(r *http.Request) bool {
	ip := loginIP(r)
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	now := time.Now()
	for key, a := range s.attempts {
		if now.After(a.Until) {
			delete(s.attempts, key)
		}
	}
	a := s.attempts[ip]
	if a.Count >= 8 {
		return false
	}
	if a.Count == 0 {
		if len(s.attempts) >= 4096 {
			return false
		}
		a.Until = now.Add(15 * time.Minute)
	}
	a.Count++
	s.attempts[ip] = a
	return true
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.validOrigin(r) {
		apiError(w, 403, "csrf", "Invalid origin")
		return
	}
	var body struct{ Username, Password string }
	if !decodeJSON(w, r, &body) {
		return
	}
	if !validUsername(body.Username) || len(body.Password) > 1024 {
		apiError(w, 400, "invalid_input", "Invalid username or password")
		return
	}
	if !s.allowLogin(r) {
		w.Header().Set("Retry-After", "900")
		apiError(w, 429, "rate_limited", "Login attempt limit reached")
		return
	}
	if !s.reservePassword(w) {
		s.releaseLogin(r)
		return
	}
	defer func() { <-s.passwordSlots }()
	var u User
	var encoded string
	e := s.db.QueryRowContext(r.Context(), `SELECT id,username,role,is_active,created_at,password_hash FROM users WHERE username=?`, body.Username).Scan(&u.ID, &u.Username, &u.Role, &u.IsActive, &u.CreatedAt, &encoded)
	if e != nil {
		encoded = "$argon2id$v=19$m=65536,t=1,p=2$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	}
	valid := checkPassword(body.Password, encoded)
	if e != nil || !valid || !u.IsActive {
		s.audit(r, "login_failed", "")
		apiError(w, 401, "invalid_credentials", "Invalid username or password")
		return
	}
	raw, csrf := randomID(), randomID()
	tx, e := s.db.BeginTx(r.Context(), nil)
	if e != nil {
		internalError(w)
		return
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	_, e = tx.ExecContext(r.Context(), `DELETE FROM sessions WHERE expires_at<=?`, now.Format(time.RFC3339))
	if e == nil {
		_, e = tx.ExecContext(r.Context(), `DELETE FROM sessions WHERE hash IN (SELECT hash FROM sessions WHERE user_id=? ORDER BY expires_at DESC LIMIT -1 OFFSET 19)`, u.ID)
	}
	if e == nil {
		_, e = tx.ExecContext(r.Context(), `INSERT INTO sessions(hash,user_id,csrf,expires_at) SELECT ?,id,?,? FROM users WHERE id=? AND password_hash=? AND is_active=1`, hashToken(raw), csrf, now.Add(24*time.Hour).Format(time.RFC3339), u.ID, encoded)
	}
	if e != nil {
		internalError(w)
		return
	}
	var exists int
	if e = tx.QueryRowContext(r.Context(), "SELECT count(*) FROM sessions WHERE hash=?", hashToken(raw)).Scan(&exists); e != nil || exists != 1 {
		apiError(w, 401, "invalid_credentials", "Account changed during login")
		return
	}
	if e = tx.Commit(); e != nil {
		internalError(w)
		return
	}
	s.setCookie(w, raw, 86400)
	s.releaseLogin(r)
	s.audit(r, "login", u.ID)
	writeJSON(w, 200, map[string]any{"user": u, "csrf_token": csrf})
}
func (s *Server) setCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: value, Path: "/", HttpOnly: true, Secure: strings.HasPrefix(s.options.PublicURL, "https://"), SameSite: http.SameSiteStrictMode, MaxAge: maxAge})
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request, p principal) {
	if p.SessionHash != "" {
		if _, e := s.db.ExecContext(r.Context(), "DELETE FROM sessions WHERE hash=?", p.SessionHash); e != nil {
			internalError(w)
			return
		}
		s.setCookie(w, "", -1)
	} else if p.TokenID != "" {
		if _, e := s.db.ExecContext(r.Context(), "DELETE FROM tokens WHERE id=?", p.TokenID); e != nil {
			internalError(w)
			return
		}
	}
	s.audit(r, "logout", p.User.ID)
	w.WriteHeader(204)
}
func (s *Server) changePassword(w http.ResponseWriter, r *http.Request, p principal) {
	var body struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if !validPassword(body.NewPassword) || len(body.CurrentPassword) > 1024 {
		apiError(w, 400, "invalid_input", "Password must contain 12 to 1024 bytes")
		return
	}
	if !s.reservePassword(w) {
		return
	}
	defer func() { <-s.passwordSlots }()
	var old string
	if e := s.db.QueryRowContext(r.Context(), "SELECT password_hash FROM users WHERE id=?", p.User.ID).Scan(&old); e != nil {
		internalError(w)
		return
	}
	if !checkPassword(body.CurrentPassword, old) {
		apiError(w, 403, "invalid_credentials", "Current password is incorrect")
		return
	}
	encoded := hashPassword(body.NewPassword)
	tx, e := s.db.BeginTx(r.Context(), nil)
	if e != nil {
		internalError(w)
		return
	}
	defer tx.Rollback()
	result, e := tx.ExecContext(r.Context(), "UPDATE users SET password_hash=? WHERE id=? AND password_hash=?", encoded, p.User.ID, old)
	if e != nil {
		internalError(w)
		return
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		apiError(w, 409, "conflict", "Account changed")
		return
	}
	if e = revokeCredentials(tx, p.User.ID); e != nil {
		internalError(w)
		return
	}
	if e = tx.Commit(); e != nil {
		internalError(w)
		return
	}
	s.setCookie(w, "", -1)
	s.audit(r, "password_changed", p.User.ID)
	w.WriteHeader(204)
}
func revokeCredentials(tx *sql.Tx, userID string) error {
	if _, e := tx.Exec("DELETE FROM sessions WHERE user_id=?", userID); e != nil {
		return e
	}
	_, e := tx.Exec("DELETE FROM tokens WHERE user_id=?", userID)
	return e
}
func internalError(w http.ResponseWriter) {
	apiError(w, 500, "internal_error", "Internal server error")
}
func (s *Server) CreateAdmin(username, password string) error {
	if !validUsername(username) || !validPassword(password) {
		return fmt.Errorf("username must be 1-64 ASCII letters, digits, dots, underscores or hyphens; password 12-1024 bytes")
	}
	s.passwordSlots <- struct{}{}
	defer func() { <-s.passwordSlots }()
	_, e := s.db.Exec("INSERT INTO users(id,username,password_hash,role,created_at) VALUES(?,?,?,'admin',?)", randomID(), username, hashPassword(password), time.Now().UTC().Format(time.RFC3339))
	return e
}
