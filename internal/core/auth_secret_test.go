// TOTP 비밀 암호화와 키 손상 시 인증 우회 방지를 검증한다.
package core

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAuthSecretAADIsolation(t *testing.T) {
	c := newClient(t)
	userID := c.login("admin")["id"].(string)
	sealed, err := c.s.sealAuthSecret(userID, "totp", []byte("private-totp-seed"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("private-totp-seed")) {
		t.Fatal("plaintext persisted")
	}
	plain, err := c.s.openAuthSecret(userID, "totp", sealed)
	if err != nil || string(plain) != "private-totp-seed" {
		t.Fatalf("decrypt: %s %v", plain, err)
	}
	if _, err = c.s.openAuthSecret("other", "totp", sealed); err == nil {
		t.Fatal("other user decrypted secret")
	}
	if _, err = c.s.openAuthSecret(userID, "pending", sealed); err == nil {
		t.Fatal("other purpose decrypted secret")
	}
	sealed[len(sealed)-1] ^= 1
	if _, err = c.s.openAuthSecret(userID, "totp", sealed); err == nil {
		t.Fatal("tampered secret accepted")
	}
}

func TestAuthSecretMissingOrCorruptKey(t *testing.T) {
	for _, kind := range []string{"missing", "corrupt"} {
		t.Run(kind, func(t *testing.T) {
			c := newClient(t)
			userID := c.login("admin")["id"].(string)
			sealed, err := c.s.sealAuthSecret(userID, "totp", []byte("private-totp-seed"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err = c.s.db.Exec("INSERT INTO user_totp(user_id,secret,last_step) VALUES(?,?,-1)", userID, sealed); err != nil {
				t.Fatal(err)
			}
			keyPath := filepath.Join(c.s.dataDir, "auth.key")
			if kind == "missing" {
				err = os.Remove(keyPath)
			} else {
				err = os.WriteFile(keyPath, []byte("bad"), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = c.s.openAuthSecret(userID, "totp", sealed); err == nil {
				t.Fatal("broken key decrypted")
			}
			if _, err = c.s.sealAuthSecret(userID, "totp", []byte("new")); err == nil {
				t.Fatal("broken key silently replaced")
			}
			if kind == "missing" {
				if _, err = os.Stat(keyPath); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("missing key was recreated")
				}
			}
		})
	}
}
