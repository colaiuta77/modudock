// 계정별 TOTP 등록과 복구 코드 및 세션 한정 재인증을 처리한다.
package core

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"image/png"
	"net/http"
	"strings"
	"time"
)

const preauthCookie = "modudock_preauth"
const bindingCookie = "modudock_auth_binding"

func (s *Server) registerSecurityRoutes() {
	s.mux.HandleFunc("GET /api/v1/auth/options", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"passkey_available": false, "passkey_reason": "Passkey initialization pending"})
	})
	s.mux.HandleFunc("POST /api/v1/auth/mfa", s.finishMFALogin)
	s.mux.HandleFunc("POST /api/v1/auth/login/cancel", func(w http.ResponseWriter, r *http.Request) {
		if !s.checkAuthOrigin(w, r) {
			return
		}
		s.cancelPreauth(r)
		s.setPreauthCookies(w, "", "", -1)
		w.WriteHeader(204)
	})
	for _, entry := range []struct {
		pattern string
		handler endpoint
	}{
		{"GET /api/v1/auth/security", s.securityStatus}, {"POST /api/v1/auth/reauth", s.reauthenticate},
		{"POST /api/v1/auth/totp/setup", s.setupTOTP}, {"DELETE /api/v1/auth/totp/setup", s.cancelTOTP},
		{"POST /api/v1/auth/totp/confirm", s.confirmTOTP}, {"DELETE /api/v1/auth/totp", s.disableTOTP}, {"POST /api/v1/auth/recovery-codes", s.regenerateRecovery},
	} {
		s.route(entry.pattern, "viewer", "", func(w http.ResponseWriter, r *http.Request, p principal) {
			if requireSecuritySession(w, r, p) {
				entry.handler(w, r, p)
			}
		})
	}
}
func requireSecuritySession(w http.ResponseWriter, r *http.Request, p principal) bool {
	if p.SessionHash == "" || p.TokenID != "" {
		apiError(w, 403, "session_required", "Account security requires a browser session")
		return false
	}
	return true
}
func (s *Server) checkAuthOrigin(w http.ResponseWriter, r *http.Request) bool {
	if !s.validOrigin(r) {
		apiError(w, 403, "csrf", "Invalid origin")
		return false
	}
	return true
}
func (s *Server) setPreauthCookies(w http.ResponseWriter, raw, binding string, maxAge int) {
	for name, value := range map[string]string{preauthCookie: raw, bindingCookie: binding} {
		http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/api/v1/auth", HttpOnly: true, Secure: strings.HasPrefix(s.options.PublicURL, "https://"), SameSite: http.SameSiteStrictMode, MaxAge: maxAge})
	}
}
func preauthValues(r *http.Request) (string, string) {
	a, e := r.Cookie(preauthCookie)
	b, f := r.Cookie(bindingCookie)
	if e != nil || f != nil || len(a.Value) != 43 || len(b.Value) != 43 {
		return "", ""
	}
	return a.Value, hashToken(b.Value)
}
func (s *Server) cancelPreauth(r *http.Request) {
	raw, binding := preauthValues(r)
	if raw != "" {
		s.db.ExecContext(r.Context(), `DELETE FROM auth_challenges WHERE hash=? AND binding_hash=? AND session_hash=''`, hashToken(raw), binding)
	}
}
func (s *Server) startMFALogin(w http.ResponseWriter, r *http.Request, userID string, version int64) {
	s.cancelPreauth(r)
	binding := randomAuthID()
	expires := time.Now().Add(5 * time.Minute)
	raw, e := s.putAuthChallenge(r.Context(), authChallenge{Purpose: "mfa-login", UserID: userID, AuthVersion: version, BindingHash: hashToken(binding), ExpiresAt: expires, Payload: json.RawMessage(`{}`)})
	if e != nil {
		apiError(w, 429, "rate_limited", "Too many pending authentication requests")
		return
	}
	s.setPreauthCookies(w, raw, binding, 300)
	s.releaseLogin(r)
	writeJSON(w, 202, map[string]any{"mfa_required": true, "expires_at": expires.UTC().Format(time.RFC3339)})
}
func (s *Server) authAttempt(w http.ResponseWriter, r *http.Request, purpose, userID string) bool {
	keys := []string{"auth:" + purpose + ":ip:" + loginIP(r)}
	if userID != "" {
		keys = append(keys, "auth:"+purpose+":user:"+userID)
	}
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	now := time.Now()
	for key, a := range s.attempts {
		if !a.Until.After(now) {
			delete(s.attempts, key)
		}
	}
	for _, key := range keys {
		if s.attempts[key].Count >= 8 || (s.attempts[key].Count == 0 && len(s.attempts) >= 4094) {
			w.Header().Set("Retry-After", "900")
			apiError(w, 429, "rate_limited", "Authentication attempt limit reached")
			return false
		}
	}
	for _, key := range keys {
		a := s.attempts[key]
		if a.Count == 0 {
			a.Until = now.Add(15 * time.Minute)
		}
		a.Count++
		s.attempts[key] = a
	}
	return true
}
func (s *Server) authAttemptSucceeded(r *http.Request, purpose, userID string) {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	for _, key := range []string{"auth:" + purpose + ":ip:" + loginIP(r), "auth:" + purpose + ":user:" + userID} {
		a := s.attempts[key]
		a.Count--
		if a.Count <= 0 {
			delete(s.attempts, key)
		} else {
			s.attempts[key] = a
		}
	}
}
func (s *Server) failChallenge(ctx context.Context, raw string) {
	s.db.ExecContext(ctx, `UPDATE auth_challenges SET attempts=attempts+1 WHERE hash=?`, hashToken(raw))
}
func authRejected(w http.ResponseWriter) {
	apiError(w, 401, "invalid_credentials", "Invalid or expired authentication code")
}
func reauthRejected(w http.ResponseWriter) {
	apiError(w, 403, "reauth_required", "Authenticate again to continue")
}

func (s *Server) finishMFALogin(w http.ResponseWriter, r *http.Request) {
	if !s.checkAuthOrigin(w, r) {
		return
	}
	var body struct{ Method, Code string }
	if !decodeJSON(w, r, &body) {
		return
	}
	raw, binding := preauthValues(r)
	c, e := s.readAuthChallenge(r.Context(), raw)
	if e != nil || c.Purpose != "mfa-login" || c.BindingHash != binding {
		authRejected(w)
		return
	}
	if !s.authAttempt(w, r, "mfa-login", c.UserID) {
		return
	}
	tx, e := s.db.BeginTx(r.Context(), nil)
	if e != nil {
		internalError(w)
		return
	}
	defer tx.Rollback()
	if _, e = claimAuthChallenge(tx, raw, "mfa-login", c.UserID, "", binding, c.AuthVersion, time.Now()); e == nil {
		e = s.consumeSecondFactor(tx, c.UserID, body.Method, body.Code)
	}
	var session authSession
	if e == nil {
		session, e = s.issueSession(r.Context(), tx, c.UserID, c.AuthVersion)
	}
	if e != nil {
		tx.Rollback()
		s.failChallenge(r.Context(), raw)
		authRejected(w)
		return
	}
	if e = tx.Commit(); e != nil {
		internalError(w)
		return
	}
	s.authAttemptSucceeded(r, "mfa-login", c.UserID)
	s.setCookie(w, session.RawCookie, 86400)
	s.setPreauthCookies(w, "", "", -1)
	s.audit(r, "mfa_login", c.UserID)
	writeJSON(w, 200, session)
}
func totpStep(secret, code string, now time.Time) (int64, error) {
	if len(code) != 6 {
		return 0, errAuthState
	}
	for _, ch := range code {
		if ch < '0' || ch > '9' {
			return 0, errAuthState
		}
	}
	step := now.Unix() / 30
	for _, offset := range []int64{0, 1, -1} {
		expected, e := totp.GenerateCode(secret, time.Unix((step+offset)*30, 0))
		if e != nil {
			return 0, e
		}
		if constantEqual(code, expected) {
			return step + offset, nil
		}
	}
	return 0, errAuthState
}
func normalizeRecovery(code string) string {
	var b strings.Builder
	for _, ch := range code {
		if ch == '-' {
			continue
		}
		if ch >= 'a' && ch <= 'z' {
			ch -= 32
		}
		if !(ch >= 'A' && ch <= 'Z' || ch >= '2' && ch <= '7') {
			return ""
		}
		b.WriteRune(ch)
	}
	if b.Len() != 26 {
		return ""
	}
	return b.String()
}
func (s *Server) consumeSecondFactor(tx *sql.Tx, userID, method, code string) error {
	if method == "recovery" {
		normalized := normalizeRecovery(code)
		if normalized == "" {
			return errAuthState
		}
		result, e := tx.Exec(`DELETE FROM recovery_codes WHERE user_id=? AND hash=?`, userID, hashToken(normalized))
		if e != nil {
			return e
		}
		n, e := result.RowsAffected()
		if e != nil {
			return e
		}
		if n != 1 {
			return errAuthState
		}
		return nil
	}
	if method != "totp" {
		return errAuthState
	}
	var sealed []byte
	var last int64
	if e := tx.QueryRow(`SELECT secret,last_step FROM user_totp WHERE user_id=?`, userID).Scan(&sealed, &last); e != nil {
		return e
	}
	secret, e := s.openAuthSecret(userID, "totp", sealed)
	if e != nil {
		return e
	}
	step, e := totpStep(string(secret), code, time.Now())
	if e != nil || step <= last {
		return errAuthState
	}
	result, e := tx.Exec(`UPDATE user_totp SET last_step=? WHERE user_id=? AND last_step<?`, step, userID, step)
	if e != nil {
		return e
	}
	n, e := result.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return errAuthState
	}
	return nil
}
func newRecoveryCodes(tx *sql.Tx, userID string) ([]string, error) {
	if _, e := tx.Exec(`DELETE FROM recovery_codes WHERE user_id=?`, userID); e != nil {
		return nil, e
	}
	codes := make([]string, 0, 10)
	for range 10 {
		b := make([]byte, 16)
		rand.Read(b)
		raw := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
		parts := []string{}
		for i := 0; i < len(raw); i += 5 {
			parts = append(parts, raw[i:min(i+5, len(raw))])
		}
		if _, e := tx.Exec(`INSERT INTO recovery_codes(user_id,hash) VALUES(?,?)`, userID, hashToken(raw)); e != nil {
			return nil, e
		}
		codes = append(codes, strings.Join(parts, "-"))
	}
	return codes, nil
}
func validReauthPurpose(purpose string) bool {
	switch purpose {
	case "totp-setup", "totp-disable", "recovery-regenerate", "passkey-register":
		return true
	}
	return strings.HasPrefix(purpose, "passkey-delete:") && len(purpose) > len("passkey-delete:") && len(purpose) < 1024
}
func (s *Server) reauthenticate(w http.ResponseWriter, r *http.Request, p principal) {
	var body struct{ Purpose, Password, Method, Code string }
	if !decodeJSON(w, r, &body) {
		return
	}
	if !validReauthPurpose(body.Purpose) || len(body.Password) > 1024 {
		apiError(w, 400, "invalid_input", "Invalid authentication request")
		return
	}
	if !s.authAttempt(w, r, "reauth", p.User.ID) || !s.reservePassword(w) {
		return
	}
	defer func() { <-s.passwordSlots }()
	account, e := s.loadAuthAccount(r.Context(), p.User.ID)
	if e != nil || !checkPassword(body.Password, account.PasswordHash) {
		reauthRejected(w)
		return
	}
	tx, e := s.db.BeginTx(r.Context(), nil)
	if e != nil {
		internalError(w)
		return
	}
	defer tx.Rollback()
	var valid, enabled bool
	e = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM users u JOIN sessions se ON se.user_id=u.id WHERE u.id=? AND u.auth_version=? AND u.is_active=1 AND se.hash=? AND se.expires_at>?),EXISTS(SELECT 1 FROM user_totp WHERE user_id=?)`, p.User.ID, account.AuthVersion, p.SessionHash, time.Now().UTC().Format(time.RFC3339), p.User.ID).Scan(&valid, &enabled)
	if e != nil || !valid {
		reauthRejected(w)
		return
	}
	if enabled {
		if e = s.consumeSecondFactor(tx, p.User.ID, body.Method, body.Code); e != nil {
			reauthRejected(w)
			return
		}
	}
	expires := time.Now().Add(5 * time.Minute)
	proof, e := insertAuthChallenge(r.Context(), tx, authChallenge{Purpose: "reauth:" + body.Purpose, UserID: p.User.ID, SessionHash: p.SessionHash, AuthVersion: account.AuthVersion, ExpiresAt: expires, Payload: json.RawMessage(`{}`)})
	if e != nil {
		apiError(w, 429, "rate_limited", "Too many pending authentication requests")
		return
	}
	if e = tx.Commit(); e != nil {
		internalError(w)
		return
	}
	s.authAttemptSucceeded(r, "reauth", p.User.ID)
	writeJSON(w, 200, map[string]any{"proof": proof, "expires_at": expires.UTC().Format(time.RFC3339)})
}
func (s *Server) validateReauthProof(ctx context.Context, p principal, proof, purpose string) error {
	account, e := s.loadAuthAccount(ctx, p.User.ID)
	if e != nil {
		return e
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	_, e = claimAuthChallenge(tx, proof, "reauth:"+purpose, p.User.ID, p.SessionHash, "", account.AuthVersion, time.Now())
	return e
}
func (s *Server) securityStatus(w http.ResponseWriter, r *http.Request, p principal) {
	var enabled bool
	var remaining int
	e := s.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM user_totp WHERE user_id=?),(SELECT count(*) FROM recovery_codes WHERE user_id=?)`, p.User.ID, p.User.ID).Scan(&enabled, &remaining)
	if e != nil {
		internalError(w)
		return
	}
	writeJSON(w, 200, map[string]any{"totp_enabled": enabled, "recovery_codes_remaining": remaining, "passkeys": []any{}, "passkey_available": false, "passkey_reason": "Passkey initialization pending"})
}
func (s *Server) setupTOTP(w http.ResponseWriter, r *http.Request, p principal) {
	var body struct{ Proof string }
	if !decodeJSON(w, r, &body) {
		return
	}
	if e := s.validateReauthProof(r.Context(), p, body.Proof, "totp-setup"); e != nil {
		reauthRejected(w)
		return
	}
	account, e := s.loadAuthAccount(r.Context(), p.User.ID)
	if e != nil {
		internalError(w)
		return
	}
	key, e := totp.Generate(totp.GenerateOpts{Issuer: "ModuDock", AccountName: p.User.Username, SecretSize: 20, Period: 30, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
	if e != nil {
		internalError(w)
		return
	}
	img, e := key.Image(300, 300)
	if e != nil {
		internalError(w)
		return
	}
	var buf bytes.Buffer
	if e = png.Encode(&buf, img); e != nil {
		internalError(w)
		return
	}
	sealed, e := s.sealAuthSecret(p.User.ID, "totp-setup", []byte(key.Secret()))
	if e != nil {
		apiError(w, 503, "auth_key_unavailable", "Restore the auth.key backup before changing TOTP")
		return
	}
	payload, _ := json.Marshal(sealed)
	tx, e := s.db.BeginTx(r.Context(), nil)
	if e != nil {
		internalError(w)
		return
	}
	defer tx.Rollback()
	if _, e = tx.Exec(`DELETE FROM auth_challenges WHERE purpose='totp-setup' AND user_id=? AND session_hash=?`, p.User.ID, p.SessionHash); e != nil {
		internalError(w)
		return
	}
	expires := time.Now().Add(10 * time.Minute)
	_, e = insertAuthChallenge(r.Context(), tx, authChallenge{Purpose: "totp-setup", UserID: p.User.ID, SessionHash: p.SessionHash, AuthVersion: account.AuthVersion, ExpiresAt: expires, Payload: payload})
	if e != nil {
		apiError(w, 429, "rate_limited", "Too many pending authentication requests")
		return
	}
	if e = tx.Commit(); e != nil {
		internalError(w)
		return
	}
	writeJSON(w, 200, map[string]any{"secret": key.Secret(), "qr_png": "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), "expires_at": expires.UTC().Format(time.RFC3339)})
}
func (s *Server) cancelTOTP(w http.ResponseWriter, r *http.Request, p principal) {
	if _, e := s.db.ExecContext(r.Context(), `DELETE FROM auth_challenges WHERE purpose='totp-setup' AND user_id=? AND session_hash=?`, p.User.ID, p.SessionHash); e != nil {
		internalError(w)
		return
	}
	w.WriteHeader(204)
}
func (s *Server) confirmTOTP(w http.ResponseWriter, r *http.Request, p principal) {
	var body struct{ Proof, Code string }
	if !decodeJSON(w, r, &body) {
		return
	}
	if e := s.validateReauthProof(r.Context(), p, body.Proof, "totp-setup"); e != nil {
		reauthRejected(w)
		return
	}
	var pendingHash string
	var payload []byte
	var version int64
	e := s.db.QueryRowContext(r.Context(), `SELECT hash,payload,auth_version FROM auth_challenges WHERE purpose='totp-setup' AND user_id=? AND session_hash=? AND expires_at>? AND attempts<5`, p.User.ID, p.SessionHash, time.Now().UnixMilli()).Scan(&pendingHash, &payload, &version)
	if e != nil {
		reauthRejected(w)
		return
	}
	if !s.authAttempt(w, r, "totp-confirm", p.User.ID) {
		return
	}
	var sealed []byte
	if e = json.Unmarshal(payload, &sealed); e != nil {
		internalError(w)
		return
	}
	secret, e := s.openAuthSecret(p.User.ID, "totp-setup", sealed)
	if e != nil {
		apiError(w, 503, "auth_key_unavailable", "Restore the auth.key backup before confirming TOTP")
		return
	}
	step, e := totpStep(string(secret), body.Code, time.Now())
	if e != nil {
		s.db.ExecContext(r.Context(), `UPDATE auth_challenges SET attempts=attempts+1 WHERE hash=?`, pendingHash)
		authRejected(w)
		return
	}
	active, e := s.sealAuthSecret(p.User.ID, "totp", secret)
	if e != nil {
		internalError(w)
		return
	}
	tx, e := s.db.BeginTx(r.Context(), nil)
	if e != nil {
		internalError(w)
		return
	}
	defer tx.Rollback()
	if _, e = claimAuthChallenge(tx, body.Proof, "reauth:totp-setup", p.User.ID, p.SessionHash, "", version, time.Now()); e != nil {
		reauthRejected(w)
		return
	}
	var consumed string
	e = tx.QueryRow(`DELETE FROM auth_challenges WHERE hash=? AND user_id=? AND session_hash=? AND auth_version=? AND expires_at>? AND attempts<5 RETURNING hash`, pendingHash, p.User.ID, p.SessionHash, version, time.Now().UnixMilli()).Scan(&consumed)
	if e != nil {
		reauthRejected(w)
		return
	}
	if _, e = tx.Exec(`INSERT INTO user_totp(user_id,secret,last_step) VALUES(?,?,?) ON CONFLICT(user_id) DO UPDATE SET secret=excluded.secret,last_step=excluded.last_step`, p.User.ID, active, step); e != nil {
		internalError(w)
		return
	}
	codes, e := newRecoveryCodes(tx, p.User.ID)
	if e == nil {
		e = revokeWebCredentials(tx, p.User.ID)
	}
	if e != nil {
		internalError(w)
		return
	}
	if e = tx.Commit(); e != nil {
		internalError(w)
		return
	}
	s.authAttemptSucceeded(r, "totp-confirm", p.User.ID)
	s.setCookie(w, "", -1)
	s.audit(r, "totp_enabled", p.User.ID)
	writeJSON(w, 200, map[string]any{"recovery_codes": codes, "signed_out": true})
}
func (s *Server) disableTOTP(w http.ResponseWriter, r *http.Request, p principal) {
	s.mutateRecovery(w, r, p, false)
}
func (s *Server) regenerateRecovery(w http.ResponseWriter, r *http.Request, p principal) {
	s.mutateRecovery(w, r, p, true)
}
func (s *Server) mutateRecovery(w http.ResponseWriter, r *http.Request, p principal, regenerate bool) {
	var body struct{ Proof string }
	if !decodeJSON(w, r, &body) {
		return
	}
	account, e := s.loadAuthAccount(r.Context(), p.User.ID)
	if e != nil {
		internalError(w)
		return
	}
	purpose := "totp-disable"
	if regenerate {
		purpose = "recovery-regenerate"
	}
	tx, e := s.db.BeginTx(r.Context(), nil)
	if e != nil {
		internalError(w)
		return
	}
	defer tx.Rollback()
	if _, e = claimAuthChallenge(tx, body.Proof, "reauth:"+purpose, p.User.ID, p.SessionHash, "", account.AuthVersion, time.Now()); e != nil {
		reauthRejected(w)
		return
	}
	var enabled bool
	if e = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM user_totp WHERE user_id=?)`, p.User.ID).Scan(&enabled); e != nil {
		internalError(w)
		return
	}
	if !enabled {
		apiError(w, 409, "conflict", "TOTP is not enabled")
		return
	}
	var codes []string
	if regenerate {
		codes, e = newRecoveryCodes(tx, p.User.ID)
	} else {
		_, e = tx.Exec(`DELETE FROM user_totp WHERE user_id=?`, p.User.ID)
		if e == nil {
			_, e = tx.Exec(`DELETE FROM recovery_codes WHERE user_id=?`, p.User.ID)
		}
	}
	if e == nil {
		e = revokeWebCredentials(tx, p.User.ID)
	}
	if e != nil {
		internalError(w)
		return
	}
	if e = tx.Commit(); e != nil {
		internalError(w)
		return
	}
	s.setCookie(w, "", -1)
	s.audit(r, purpose, p.User.ID)
	if regenerate {
		writeJSON(w, 200, map[string]any{"recovery_codes": codes, "signed_out": true})
	} else {
		w.WriteHeader(204)
	}
}
