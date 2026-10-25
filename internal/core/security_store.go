// 인증 상태의 저장과 단회 소비 및 계정 보안 버전에 따른 세션 발급을 관리한다.
package core

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"
)

const securitySchema = `ALTER TABLE users ADD COLUMN auth_version INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN webauthn_handle BLOB;
CREATE TABLE auth_challenges(hash TEXT PRIMARY KEY,purpose TEXT NOT NULL,user_id TEXT REFERENCES users(id) ON DELETE CASCADE,session_hash TEXT NOT NULL,binding_hash TEXT NOT NULL,auth_version INTEGER NOT NULL,expires_at INTEGER NOT NULL,attempts INTEGER NOT NULL DEFAULT 0,payload BLOB NOT NULL);
CREATE INDEX auth_challenges_user ON auth_challenges(user_id);
CREATE INDEX auth_challenges_expiry ON auth_challenges(expires_at);
CREATE TABLE user_totp(user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,secret BLOB NOT NULL,last_step INTEGER NOT NULL DEFAULT -1);
CREATE TABLE recovery_codes(user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,hash TEXT NOT NULL,PRIMARY KEY(user_id,hash));
CREATE TABLE passkeys(id TEXT PRIMARY KEY,user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,name TEXT NOT NULL,credential BLOB NOT NULL,created_at TEXT NOT NULL,last_used_at TEXT NOT NULL DEFAULT '');
CREATE INDEX passkeys_user ON passkeys(user_id);
CREATE TABLE auth_settings(key TEXT PRIMARY KEY,value TEXT NOT NULL);
PRAGMA user_version=3;`

var errAuthState = errors.New("authentication state expired or changed")

type authAccount struct {
	User
	PasswordHash string
	AuthVersion  int64
}
type authSession struct {
	User      User   `json:"user"`
	CSRF      string `json:"csrf_token"`
	RawCookie string `json:"-"`
}
type authChallenge struct {
	Purpose, UserID, SessionHash, BindingHash string
	AuthVersion                               int64
	ExpiresAt                                 time.Time
	Attempts                                  int
	Payload                                   json.RawMessage
}

func randomAuthID() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
func (s *Server) loadAuthAccount(ctx context.Context, userID string) (authAccount, error) {
	var a authAccount
	e := s.db.QueryRowContext(ctx, `SELECT id,username,role,is_active,created_at,password_hash,auth_version FROM users WHERE id=?`, userID).Scan(&a.ID, &a.Username, &a.Role, &a.IsActive, &a.CreatedAt, &a.PasswordHash, &a.AuthVersion)
	return a, e
}
func (s *Server) issueSession(ctx context.Context, tx *sql.Tx, userID string, version int64) (authSession, error) {
	var session authSession
	e := tx.QueryRowContext(ctx, `SELECT id,username,role,is_active,created_at FROM users WHERE id=? AND auth_version=? AND is_active=1`, userID, version).Scan(&session.User.ID, &session.User.Username, &session.User.Role, &session.User.IsActive, &session.User.CreatedAt)
	if e != nil {
		return session, errAuthState
	}
	now := time.Now().UTC()
	if _, e = tx.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at<=?`, now.Format(time.RFC3339)); e != nil {
		return session, e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM sessions WHERE hash IN (SELECT hash FROM sessions WHERE user_id=? ORDER BY expires_at DESC LIMIT -1 OFFSET 19)`, userID); e != nil {
		return session, e
	}
	session.RawCookie, session.CSRF = randomAuthID(), randomAuthID()
	_, e = tx.ExecContext(ctx, `INSERT INTO sessions(hash,user_id,csrf,expires_at) VALUES(?,?,?,?)`, hashToken(session.RawCookie), userID, session.CSRF, now.Add(24*time.Hour).Format(time.RFC3339))
	return session, e
}
func revokeWebCredentials(tx *sql.Tx, userID string) error {
	for _, query := range []string{`UPDATE users SET auth_version=auth_version+1 WHERE id=?`, `DELETE FROM sessions WHERE user_id=?`, `DELETE FROM auth_challenges WHERE user_id=? OR (user_id IS NULL AND purpose='passkey-login')`} {
		if _, e := tx.Exec(query, userID); e != nil {
			return e
		}
	}
	return nil
}
func (s *Server) putAuthChallenge(ctx context.Context, c authChallenge) (string, error) {
	if !c.ExpiresAt.After(time.Now()) || c.Purpose == "" || !json.Valid(c.Payload) {
		return "", errAuthState
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return "", e
	}
	defer tx.Rollback()
	raw, e := insertAuthChallenge(ctx, tx, c)
	if e != nil {
		return "", e
	}
	return raw, tx.Commit()
}
func insertAuthChallenge(ctx context.Context, tx *sql.Tx, c authChallenge) (string, error) {
	var e error
	if _, e = tx.ExecContext(ctx, `DELETE FROM auth_challenges WHERE expires_at<=?`, time.Now().UnixMilli()); e != nil {
		return "", e
	}
	var total, own int
	if e = tx.QueryRowContext(ctx, `SELECT count(*),coalesce(sum(CASE WHEN user_id=? THEN 1 ELSE 0 END),0) FROM auth_challenges`, c.UserID).Scan(&total, &own); e != nil {
		return "", e
	}
	if total >= 4096 || (c.UserID != "" && own >= 8) {
		return "", errors.New("too many pending authentication requests")
	}
	if c.UserID != "" {
		var n int
		if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE id=? AND auth_version=? AND is_active=1`, c.UserID, c.AuthVersion).Scan(&n); e != nil {
			return "", e
		}
		if n != 1 {
			return "", errAuthState
		}
	}
	raw := randomAuthID()
	_, e = tx.ExecContext(ctx, `INSERT INTO auth_challenges(hash,purpose,user_id,session_hash,binding_hash,auth_version,expires_at,attempts,payload) VALUES(?,?,nullif(?,''),?,?,?,?,?,?)`, hashToken(raw), c.Purpose, c.UserID, c.SessionHash, c.BindingHash, c.AuthVersion, c.ExpiresAt.UnixMilli(), c.Attempts, []byte(c.Payload))
	if e != nil {
		return "", e
	}
	return raw, nil
}
func (s *Server) readAuthChallenge(ctx context.Context, raw string) (authChallenge, error) {
	var c authChallenge
	var expires int64
	if len(raw) != 43 {
		return c, errAuthState
	}
	e := s.db.QueryRowContext(ctx, `SELECT purpose,coalesce(user_id,''),session_hash,binding_hash,auth_version,expires_at,attempts,payload FROM auth_challenges WHERE hash=? AND expires_at>? AND attempts<5`, hashToken(raw), time.Now().UnixMilli()).Scan(&c.Purpose, &c.UserID, &c.SessionHash, &c.BindingHash, &c.AuthVersion, &expires, &c.Attempts, &c.Payload)
	c.ExpiresAt = time.UnixMilli(expires)
	return c, e
}
func claimAuthChallenge(tx *sql.Tx, raw, purpose, userID, sessionHash, bindingHash string, version int64, now time.Time) (json.RawMessage, error) {
	var payload json.RawMessage
	if len(raw) != 43 {
		return nil, errAuthState
	}
	e := tx.QueryRow(`DELETE FROM auth_challenges WHERE hash=? AND purpose=? AND coalesce(user_id,'')=? AND session_hash=? AND binding_hash=? AND auth_version=? AND expires_at>? AND attempts<5
 AND (user_id IS NULL OR EXISTS(SELECT 1 FROM users WHERE id=user_id AND auth_version=? AND is_active=1))
 AND (session_hash='' OR EXISTS(SELECT 1 FROM sessions WHERE hash=session_hash AND user_id=? AND expires_at>?)) RETURNING payload`, hashToken(raw), purpose, userID, sessionHash, bindingHash, version, now.UnixMilli(), version, userID, now.UTC().Format(time.RFC3339)).Scan(&payload)
	if errors.Is(e, sql.ErrNoRows) {
		e = errAuthState
	}
	return payload, e
}
