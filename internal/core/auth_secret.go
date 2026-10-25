// 별도 파일 키로 TOTP 비밀을 암호화하고 손상된 키의 자동 재생성을 차단한다.
package core

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
)

func (s *Server) authSecretKey(create bool) ([]byte, error) {
	// 복호화는 DB 트랜잭션 안에서도 호출되므로 DB를 조회하는 생성 잠금을 기다리지 않는다.
	if create {
		s.authKeyMu.Lock()
		defer s.authKeyMu.Unlock()
	}
	path := filepath.Join(s.dataDir, "auth.key")
	info, e := os.Lstat(path)
	if e == nil && !info.Mode().IsRegular() {
		return nil, errors.New("auth.key must be a regular file")
	}
	if e == nil {
		key, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if len(key) != 32 {
			return nil, errors.New("auth.key is corrupt; restore its backup")
		}
		return key, nil
	}
	if !errors.Is(e, os.ErrNotExist) || !create {
		return nil, e
	}
	var count int
	if e = s.db.QueryRow(`SELECT (SELECT count(*) FROM user_totp)+(SELECT count(*) FROM auth_challenges WHERE purpose='totp-setup')`).Scan(&count); e != nil {
		return nil, e
	}
	if count != 0 {
		return nil, errors.New("auth.key is missing; restore its backup")
	}
	key := make([]byte, 32)
	rand.Read(key)
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return nil, e
	}
	_, e = f.Write(key)
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil {
		return nil, e
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return key, nil
}
func (s *Server) sealAuthSecret(userID, purpose string, plain []byte) ([]byte, error) {
	key, e := s.authSecretKey(true)
	if e != nil {
		return nil, e
	}
	block, e := aes.NewCipher(key)
	if e != nil {
		return nil, e
	}
	gcm, e := cipher.NewGCM(block)
	if e != nil {
		return nil, e
	}
	nonce := make([]byte, gcm.NonceSize())
	rand.Read(nonce)
	return gcm.Seal(nonce, nonce, plain, []byte(userID+"\x00"+purpose)), nil
}
func (s *Server) openAuthSecret(userID, purpose string, sealed []byte) ([]byte, error) {
	key, e := s.authSecretKey(false)
	if e != nil {
		return nil, e
	}
	block, e := aes.NewCipher(key)
	if e != nil {
		return nil, e
	}
	gcm, e := cipher.NewGCM(block)
	if e != nil {
		return nil, e
	}
	if len(sealed) < gcm.NonceSize() {
		return nil, errors.New("invalid encrypted authentication secret")
	}
	return gcm.Open(nil, sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():], []byte(userID+"\x00"+purpose))
}
