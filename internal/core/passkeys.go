// WebAuthn 표준 검증으로 Passkey 등록과 로그인 및 계정별 관리를 제공한다.
package core

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

const passkeyCookie = "modudock_passkey"

type passkeyAccount struct {
	authAccount
	handle      []byte
	credentials []webauthn.Credential
}

func (u passkeyAccount) WebAuthnID() []byte                         { return u.handle }
func (u passkeyAccount) WebAuthnName() string                       { return u.Username }
func (u passkeyAccount) WebAuthnDisplayName() string                { return u.Username }
func (u passkeyAccount) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

type passkeyCeremony struct {
	Session webauthn.SessionData `json:"session"`
	Purpose string               `json:"purpose,omitempty"`
}
type passkeyMetadata struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	CreatedAt  string `json:"created_at"`
	LastUsedAt string `json:"last_used_at"`
}

func (s *Server) passkeyService() (*webauthn.WebAuthn, error) {
	u, e := url.Parse(s.options.PublicURL)
	if e != nil || u.Host == "" {
		return nil, errors.New("Passkey requires an explicit public-url")
	}
	host := u.Hostname()
	if (u.Scheme != "https" && !(u.Scheme == "http" && host == "localhost")) || net.ParseIP(host) != nil {
		return nil, errors.New("Passkey requires an HTTPS hostname or http://localhost")
	}
	var pinned string
	var count int
	e = s.db.QueryRow(`SELECT coalesce((SELECT value FROM auth_settings WHERE key='webauthn_rp_id'),''),(SELECT count(*) FROM passkeys)`).Scan(&pinned, &count)
	if e != nil {
		return nil, e
	}
	if count > 0 && pinned != host {
		return nil, errors.New("Passkey hostname changed; remove existing Passkeys before registering again")
	}
	return webauthn.New(&webauthn.Config{RPID: host, RPDisplayName: "ModuDock", RPOrigins: []string{s.options.PublicURL}, AttestationPreference: protocol.PreferNoAttestation, AuthenticatorSelection: protocol.AuthenticatorSelection{ResidentKey: protocol.ResidentKeyRequirementRequired, RequireResidentKey: protocol.ResidentKeyRequired(), UserVerification: protocol.VerificationRequired}, Timeouts: webauthn.TimeoutsConfig{Login: webauthn.TimeoutConfig{Enforce: true, Timeout: 5 * time.Minute}, Registration: webauthn.TimeoutConfig{Enforce: true, Timeout: 5 * time.Minute}}})
}
func (s *Server) passkeyAvailability() (bool, string) {
	_, e := s.passkeyService()
	if e != nil {
		return false, e.Error()
	}
	return true, ""
}
func (s *Server) requirePasskeyService(w http.ResponseWriter) (*webauthn.WebAuthn, bool) {
	service, e := s.passkeyService()
	if e != nil {
		apiError(w, 503, "passkey_unavailable", e.Error())
		return nil, false
	}
	return service, true
}
func loadPasskeyAccount(tx *sql.Tx, userID string, createHandle bool) (passkeyAccount, error) {
	var u passkeyAccount
	e := tx.QueryRow(`SELECT id,username,role,is_active,created_at,password_hash,auth_version,webauthn_handle FROM users WHERE id=? AND is_active=1`, userID).Scan(&u.ID, &u.Username, &u.Role, &u.IsActive, &u.CreatedAt, &u.PasswordHash, &u.AuthVersion, &u.handle)
	if e != nil {
		return u, e
	}
	if len(u.handle) == 0 && createHandle {
		u.handle = make([]byte, 32)
		rand.Read(u.handle)
		if _, e = tx.Exec(`UPDATE users SET webauthn_handle=? WHERE id=?`, u.handle, userID); e != nil {
			return u, e
		}
	}
	rows, e := tx.Query(`SELECT credential FROM passkeys WHERE user_id=?`, userID)
	if e != nil {
		return u, e
	}
	defer rows.Close()
	for rows.Next() {
		var data []byte
		var credential webauthn.Credential
		if e = rows.Scan(&data); e != nil {
			return u, e
		}
		if e = json.Unmarshal(data, &credential); e != nil {
			return u, e
		}
		u.credentials = append(u.credentials, credential)
	}
	return u, rows.Err()
}
func (s *Server) listPasskeyMetadata(ctx context.Context, userID string) ([]passkeyMetadata, error) {
	items := []passkeyMetadata{}
	rows, e := s.db.QueryContext(ctx, `SELECT id,name,created_at,last_used_at FROM passkeys WHERE user_id=? ORDER BY created_at,id`, userID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var item passkeyMetadata
		if e = rows.Scan(&item.ID, &item.Name, &item.CreatedAt, &item.LastUsedAt); e != nil {
			return nil, e
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
func (s *Server) registerPasskeyRoutes() {
	s.mux.HandleFunc("POST /api/v1/auth/passkey/begin", s.beginPasskeyLogin)
	s.mux.HandleFunc("POST /api/v1/auth/passkey/finish", func(w http.ResponseWriter, r *http.Request) {
		if s.checkAuthOrigin(w, r) {
			s.finishPasskey(w, r, nil)
		}
	})
	for _, entry := range []struct {
		pattern string
		handler endpoint
	}{
		{"POST /api/v1/auth/passkeys/register/begin", s.beginPasskeyRegistration}, {"POST /api/v1/auth/passkeys/register/finish", s.finishPasskeyRegistration},
		{"POST /api/v1/auth/passkeys/reauth/begin", s.beginPasskeyReauth}, {"POST /api/v1/auth/passkeys/reauth/finish", func(w http.ResponseWriter, r *http.Request, p principal) { s.finishPasskey(w, r, &p) }},
		{"PUT /api/v1/auth/passkeys/{id}", s.renamePasskey}, {"DELETE /api/v1/auth/passkeys/{id}", s.deletePasskey},
	} {
		s.route(entry.pattern, "viewer", "", func(w http.ResponseWriter, r *http.Request, p principal) {
			if requireSecuritySession(w, r, p) {
				entry.handler(w, r, p)
			}
		})
	}
}
func (s *Server) setPasskeyCookie(w http.ResponseWriter, raw string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: passkeyCookie, Value: raw, Path: "/api/v1/auth", HttpOnly: true, Secure: strings.HasPrefix(s.options.PublicURL, "https://"), SameSite: http.SameSiteStrictMode, MaxAge: maxAge})
}
func readPasskeyCookie(r *http.Request) string {
	cookie, e := r.Cookie(passkeyCookie)
	if e != nil || len(cookie.Value) != 43 {
		return ""
	}
	return cookie.Value
}
func (s *Server) storePasskeyCeremony(w http.ResponseWriter, r *http.Request, p principal, user passkeyAccount, purpose, reauthPurpose string, session *webauthn.SessionData) bool {
	old := readPasskeyCookie(r)
	if old != "" {
		if _, e := s.db.ExecContext(r.Context(), `DELETE FROM auth_challenges WHERE hash=? AND session_hash=? AND user_id=?`, hashToken(old), p.SessionHash, p.User.ID); e != nil {
			internalError(w)
			return false
		}
	}
	payload, e := json.Marshal(passkeyCeremony{Session: *session, Purpose: reauthPurpose})
	if e != nil {
		internalError(w)
		return false
	}
	raw, e := s.putAuthChallenge(r.Context(), authChallenge{Purpose: purpose, UserID: p.User.ID, SessionHash: p.SessionHash, AuthVersion: user.AuthVersion, ExpiresAt: time.Now().Add(5 * time.Minute), Payload: payload})
	if e != nil {
		apiError(w, 429, "rate_limited", "Too many pending authentication requests")
		return false
	}
	s.setPasskeyCookie(w, raw, 300)
	return true
}
func (s *Server) beginPasskeyRegistration(w http.ResponseWriter, r *http.Request, p principal) {
	var body struct{ Proof string }
	if !decodeJSON(w, r, &body) {
		return
	}
	if e := s.validateReauthProof(r.Context(), p, body.Proof, "passkey-register"); e != nil {
		reauthRejected(w)
		return
	}
	service, ok := s.requirePasskeyService(w)
	if !ok {
		return
	}
	tx, e := s.db.BeginTx(r.Context(), nil)
	if e != nil {
		internalError(w)
		return
	}
	defer tx.Rollback()
	user, e := loadPasskeyAccount(tx, p.User.ID, true)
	if e != nil {
		internalError(w)
		return
	}
	if len(user.credentials) >= 10 {
		apiError(w, 409, "passkey_limit", "At most 10 Passkeys are allowed")
		return
	}
	if e = tx.Commit(); e != nil {
		internalError(w)
		return
	}
	exclude := []protocol.CredentialDescriptor{}
	for _, credential := range user.credentials {
		exclude = append(exclude, credential.Descriptor())
	}
	options, session, e := service.BeginRegistration(user, webauthn.WithExclusions(exclude))
	if e != nil {
		internalError(w)
		return
	}
	if s.storePasskeyCeremony(w, r, p, user, "passkey-register", "", session) {
		writeJSON(w, 200, options)
	}
}
func validPasskeyName(name string) bool {
	return utf8.ValidString(name) && utf8.RuneCountInString(name) >= 1 && utf8.RuneCountInString(name) <= 100
}
func (s *Server) finishPasskeyRegistration(w http.ResponseWriter, r *http.Request, p principal) {
	var body struct {
		Proof, Name string
		Credential  json.RawMessage
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if !validPasskeyName(body.Name) {
		apiError(w, 400, "invalid_input", "Passkey name must contain 1 to 100 characters")
		return
	}
	service, ok := s.requirePasskeyService(w)
	if !ok {
		return
	}
	raw := readPasskeyCookie(r)
	state, e := s.readAuthChallenge(r.Context(), raw)
	if e != nil || state.Purpose != "passkey-register" || state.UserID != p.User.ID || state.SessionHash != p.SessionHash {
		reauthRejected(w)
		return
	}
	var ceremony passkeyCeremony
	if e = json.Unmarshal(state.Payload, &ceremony); e != nil {
		internalError(w)
		return
	}
	parsed, e := protocol.ParseCredentialCreationResponseBytes(body.Credential)
	if e != nil {
		s.failChallenge(r.Context(), raw)
		authRejected(w)
		return
	}
	tx, e := s.db.BeginTx(r.Context(), nil)
	if e != nil {
		internalError(w)
		return
	}
	defer tx.Rollback()
	if _, e = claimAuthChallenge(tx, body.Proof, "reauth:passkey-register", p.User.ID, p.SessionHash, "", state.AuthVersion, time.Now()); e != nil {
		reauthRejected(w)
		return
	}
	if _, e = claimAuthChallenge(tx, raw, "passkey-register", p.User.ID, p.SessionHash, "", state.AuthVersion, time.Now()); e != nil {
		reauthRejected(w)
		return
	}
	user, e := loadPasskeyAccount(tx, p.User.ID, false)
	if e != nil {
		reauthRejected(w)
		return
	}
	if len(user.credentials) >= 10 {
		apiError(w, 409, "passkey_limit", "At most 10 Passkeys are allowed")
		return
	}
	credential, e := service.CreateCredential(user, ceremony.Session, parsed)
	if e != nil || !credential.Flags.UserVerified {
		tx.Rollback()
		s.failChallenge(r.Context(), raw)
		authRejected(w)
		return
	}
	var pin string
	var count int
	e = tx.QueryRow(`SELECT coalesce((SELECT value FROM auth_settings WHERE key='webauthn_rp_id'),''),(SELECT count(*) FROM passkeys)`).Scan(&pin, &count)
	if e != nil {
		internalError(w)
		return
	}
	if count > 0 && pin != service.Config.RPID {
		reauthRejected(w)
		return
	}
	if _, e = tx.Exec(`INSERT INTO auth_settings(key,value) VALUES('webauthn_rp_id',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, service.Config.RPID); e != nil {
		internalError(w)
		return
	}
	data, e := json.Marshal(credential)
	if e != nil {
		internalError(w)
		return
	}
	id := base64.RawURLEncoding.EncodeToString(credential.ID)
	result, e := tx.Exec(`INSERT INTO passkeys(id,user_id,name,credential,created_at) VALUES(?,?,?,?,?) ON CONFLICT(id) DO NOTHING`, id, p.User.ID, body.Name, data, time.Now().UTC().Format(time.RFC3339))
	if e != nil {
		internalError(w)
		return
	}
	n, e := result.RowsAffected()
	if e != nil || n != 1 {
		apiError(w, 409, "conflict", "Passkey is already registered")
		return
	}
	if e = revokeWebCredentials(tx, p.User.ID); e != nil {
		internalError(w)
		return
	}
	if e = tx.Commit(); e != nil {
		internalError(w)
		return
	}
	s.setCookie(w, "", -1)
	s.setPasskeyCookie(w, "", -1)
	s.audit(r, "passkey_registered", p.User.ID)
	writeJSON(w, 200, map[string]any{"signed_out": true})
}
func (s *Server) beginPasskeyLogin(w http.ResponseWriter, r *http.Request) {
	if !s.checkAuthOrigin(w, r) {
		return
	}
	var body struct{}
	if !decodeJSON(w, r, &body) {
		return
	}
	service, ok := s.requirePasskeyService(w)
	if !ok {
		return
	}
	if !s.authAttempt(w, r, "passkey-begin", "") {
		return
	}
	options, session, e := service.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired))
	if e != nil {
		internalError(w)
		return
	}
	s.cancelPreauth(r)
	binding := randomAuthID()
	payload, _ := json.Marshal(passkeyCeremony{Session: *session})
	raw, e := s.putAuthChallenge(r.Context(), authChallenge{Purpose: "passkey-login", BindingHash: hashToken(binding), ExpiresAt: time.Now().Add(5 * time.Minute), Payload: payload})
	if e != nil {
		apiError(w, 429, "rate_limited", "Too many pending authentication requests")
		return
	}
	s.setPreauthCookies(w, raw, binding, 300)
	writeJSON(w, 200, options)
}
func (s *Server) beginPasskeyReauth(w http.ResponseWriter, r *http.Request, p principal) {
	var body struct{ Purpose string }
	if !decodeJSON(w, r, &body) {
		return
	}
	if !validReauthPurpose(body.Purpose) {
		apiError(w, 400, "invalid_input", "Invalid authentication purpose")
		return
	}
	service, ok := s.requirePasskeyService(w)
	if !ok {
		return
	}
	tx, e := s.db.BeginTx(r.Context(), nil)
	if e != nil {
		internalError(w)
		return
	}
	user, e := loadPasskeyAccount(tx, p.User.ID, false)
	tx.Rollback()
	if e != nil {
		internalError(w)
		return
	}
	if len(user.credentials) == 0 {
		apiError(w, 409, "no_passkeys", "No registered Passkeys")
		return
	}
	options, session, e := service.BeginLogin(user, webauthn.WithUserVerification(protocol.VerificationRequired))
	if e != nil {
		internalError(w)
		return
	}
	if s.storePasskeyCeremony(w, r, p, user, "passkey-reauth", body.Purpose, session) {
		writeJSON(w, 200, options)
	}
}
func (s *Server) finishPasskey(w http.ResponseWriter, r *http.Request, p *principal) {
	var body struct{ Credential json.RawMessage }
	if !decodeJSON(w, r, &body) {
		return
	}
	service, ok := s.requirePasskeyService(w)
	if !ok {
		return
	}
	raw, binding := preauthValues(r)
	purpose := "passkey-login"
	sessionHash, userID := "", ""
	if p != nil {
		raw, binding = readPasskeyCookie(r), ""
		purpose = "passkey-reauth"
		sessionHash, userID = p.SessionHash, p.User.ID
	}
	state, e := s.readAuthChallenge(r.Context(), raw)
	if e != nil || state.Purpose != purpose || state.SessionHash != sessionHash || state.BindingHash != binding || state.UserID != userID {
		if p != nil {
			reauthRejected(w)
		} else {
			authRejected(w)
		}
		return
	}
	parsed, e := protocol.ParseCredentialRequestResponseBytes(body.Credential)
	if e != nil {
		s.failChallenge(r.Context(), raw)
		authRejected(w)
		return
	}
	if p == nil {
		e = s.db.QueryRowContext(r.Context(), `SELECT u.id FROM users u JOIN passkeys k ON k.user_id=u.id WHERE k.id=? AND u.webauthn_handle=? AND u.is_active=1`, base64.RawURLEncoding.EncodeToString(parsed.RawID), parsed.Response.UserHandle).Scan(&userID)
	}
	if !s.authAttempt(w, r, purpose, userID) {
		return
	}
	if e != nil {
		s.failChallenge(r.Context(), raw)
		authRejected(w)
		return
	}
	var ceremony passkeyCeremony
	if e = json.Unmarshal(state.Payload, &ceremony); e != nil {
		internalError(w)
		return
	}
	tx, e := s.db.BeginTx(r.Context(), nil)
	if e != nil {
		internalError(w)
		return
	}
	defer tx.Rollback()
	reject := func() {
		tx.Rollback()
		s.failChallenge(r.Context(), raw)
		if p != nil {
			reauthRejected(w)
		} else {
			authRejected(w)
		}
	}
	if _, e = claimAuthChallenge(tx, raw, purpose, state.UserID, sessionHash, binding, state.AuthVersion, time.Now()); e != nil {
		reject()
		return
	}
	user, e := loadPasskeyAccount(tx, userID, false)
	if e != nil {
		reject()
		return
	}
	var credential *webauthn.Credential
	if p == nil {
		credential, e = service.ValidateDiscoverableLogin(func(id, handle []byte) (webauthn.User, error) {
			if !bytes.Equal(handle, user.handle) || !bytes.Equal(id, parsed.RawID) {
				return nil, errAuthState
			}
			return user, nil
		}, ceremony.Session, parsed)
	} else {
		credential, e = service.ValidateLogin(user, ceremony.Session, parsed)
	}
	if e != nil || !credential.Flags.UserVerified || (credential.Authenticator.CloneWarning && !credential.Flags.BackupEligible) {
		reject()
		return
	}
	data, e := json.Marshal(credential)
	if e != nil {
		internalError(w)
		return
	}
	result, e := tx.Exec(`UPDATE passkeys SET credential=?,last_used_at=? WHERE id=? AND user_id=?`, data, time.Now().UTC().Format(time.RFC3339), base64.RawURLEncoding.EncodeToString(credential.ID), user.ID)
	if e != nil {
		internalError(w)
		return
	}
	n, e := result.RowsAffected()
	if e != nil || n != 1 {
		reject()
		return
	}
	var response any
	var session authSession
	if p == nil {
		session, e = s.issueSession(r.Context(), tx, user.ID, user.AuthVersion)
		response = session
	} else {
		expires := time.Now().Add(5 * time.Minute)
		var proof string
		proof, e = insertAuthChallenge(r.Context(), tx, authChallenge{Purpose: "reauth:" + ceremony.Purpose, UserID: user.ID, SessionHash: p.SessionHash, AuthVersion: user.AuthVersion, ExpiresAt: expires, Payload: json.RawMessage(`{}`)})
		response = map[string]any{"proof": proof, "expires_at": expires.UTC().Format(time.RFC3339)}
	}
	if e != nil {
		reject()
		return
	}
	if e = tx.Commit(); e != nil {
		internalError(w)
		return
	}
	s.authAttemptSucceeded(r, purpose, user.ID)
	if p == nil {
		s.setCookie(w, session.RawCookie, 86400)
		s.setPreauthCookies(w, "", "", -1)
	} else {
		s.setPasskeyCookie(w, "", -1)
	}
	if credential.Authenticator.CloneWarning {
		s.audit(r, "passkey_counter_warning", user.ID)
	}
	s.audit(r, purpose, user.ID)
	writeJSON(w, 200, response)
}
func (s *Server) renamePasskey(w http.ResponseWriter, r *http.Request, p principal) {
	var body struct{ Name string }
	if !decodeJSON(w, r, &body) {
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if !validPasskeyName(body.Name) {
		apiError(w, 400, "invalid_input", "Passkey name must contain 1 to 100 characters")
		return
	}
	result, e := s.db.ExecContext(r.Context(), `UPDATE passkeys SET name=? WHERE id=? AND user_id=?`, body.Name, r.PathValue("id"), p.User.ID)
	if e != nil {
		internalError(w)
		return
	}
	n, e := result.RowsAffected()
	if e != nil {
		internalError(w)
		return
	}
	if n != 1 {
		apiError(w, 404, "not_found", "Passkey not found")
		return
	}
	s.audit(r, "passkey_renamed", p.User.ID)
	writeJSON(w, 200, map[string]any{"name": body.Name})
}
func (s *Server) deletePasskey(w http.ResponseWriter, r *http.Request, p principal) {
	var body struct{ Proof string }
	if !decodeJSON(w, r, &body) {
		return
	}
	account, e := s.loadAuthAccount(r.Context(), p.User.ID)
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
	if _, e = claimAuthChallenge(tx, body.Proof, "reauth:passkey-delete:"+r.PathValue("id"), p.User.ID, p.SessionHash, "", account.AuthVersion, time.Now()); e != nil {
		reauthRejected(w)
		return
	}
	result, e := tx.Exec(`DELETE FROM passkeys WHERE id=? AND user_id=?`, r.PathValue("id"), p.User.ID)
	if e != nil {
		internalError(w)
		return
	}
	n, e := result.RowsAffected()
	if e != nil {
		internalError(w)
		return
	}
	if n != 1 {
		apiError(w, 404, "not_found", "Passkey not found")
		return
	}
	if e = revokeWebCredentials(tx, p.User.ID); e != nil {
		internalError(w)
		return
	}
	if e = tx.Commit(); e != nil {
		internalError(w)
		return
	}
	s.setCookie(w, "", -1)
	s.audit(r, "passkey_deleted", p.User.ID)
	w.WriteHeader(204)
}
