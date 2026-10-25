// 정지된 데이터 디렉터리에서 계정의 MFA와 모든 자격증명을 비상 초기화한다.
package core

import (
	"errors"
	"time"
)

func (s *Server) ResetMFA(username string) error {
	if !validUsername(username) {
		return errors.New("valid username required")
	}
	tx, e := s.db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var userID string
	if e = tx.QueryRow(`SELECT id FROM users WHERE username=?`, username).Scan(&userID); e != nil {
		return e
	}
	for _, query := range []string{`DELETE FROM user_totp WHERE user_id=?`, `DELETE FROM recovery_codes WHERE user_id=?`, `DELETE FROM passkeys WHERE user_id=?`} {
		if _, e = tx.Exec(query, userID); e != nil {
			return e
		}
	}
	if e = revokeCredentials(tx, userID); e != nil {
		return e
	}
	if _, e = tx.Exec(`INSERT INTO audit(id,timestamp,action,user_id,request_id) VALUES(?,?,'mfa_reset_cli',?,'cli')`, randomID(), time.Now().UTC().Format(time.RFC3339), userID); e != nil {
		return e
	}
	if e = tx.Commit(); e != nil {
		return e
	}
	s.log(LogEntry{Message: "mfa_reset_cli", Component: "auth", UserID: userID, RequestID: "cli"})
	return nil
}
