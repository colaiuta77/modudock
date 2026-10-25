// 테스트용 ES256 인증기로 Passkey 등록과 서명 및 소유권 경계를 검증한다.
package core

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/go-webauthn/webauthn/protocol/webauthncbor"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

type testPasskey struct {
	key        *ecdsa.PrivateKey
	id, handle []byte
	count      uint32
	backup     bool
}

func newTestPasskey(t *testing.T) *testPasskey {
	t.Helper()
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	id := make([]byte, 32)
	rand.Read(id)
	return &testPasskey{key: key, id: id}
}
func encodeB64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
func jsonText(v any) string     { b, _ := json.Marshal(v); return string(b) }
func passkeyRequest(c *client, path string, body any, cookies []*http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "http://localhost/api/v1/auth/"+path, strings.NewReader(jsonText(body)))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://localhost")
	if c.cookie != nil {
		r.AddCookie(c.cookie)
		r.Header.Set("X-CSRF-Token", c.csrf)
	}
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	c.s.Handler().ServeHTTP(w, r)
	return w
}
func (k *testPasskey) registration(t *testing.T, options map[string]any, flags byte) map[string]any {
	t.Helper()
	pub := options["publicKey"].(map[string]any)
	handle, e := base64.RawURLEncoding.DecodeString(pub["user"].(map[string]any)["id"].(string))
	if e != nil {
		t.Fatal(e)
	}
	k.handle = handle
	rp := pub["rp"].(map[string]any)["id"].(string)
	hash := sha256.Sum256([]byte(rp))
	data := append([]byte{}, hash[:]...)
	data = append(data, flags|0x40)
	data = append(data, 0, 0, 0, 0)
	data = append(data, make([]byte, 16)...)
	data = binary.BigEndian.AppendUint16(data, uint16(len(k.id)))
	data = append(data, k.id...)
	cose, e := webauthncbor.Marshal(map[int]any{1: 2, 3: -7, -1: 1, -2: k.key.X.FillBytes(make([]byte, 32)), -3: k.key.Y.FillBytes(make([]byte, 32))})
	if e != nil {
		t.Fatal(e)
	}
	data = append(data, cose...)
	att, e := webauthncbor.Marshal(map[string]any{"fmt": "none", "authData": data, "attStmt": map[string]any{}})
	if e != nil {
		t.Fatal(e)
	}
	cd := jsonText(map[string]any{"type": "webauthn.create", "challenge": pub["challenge"], "origin": "http://localhost", "crossOrigin": false})
	return map[string]any{"id": encodeB64(k.id), "rawId": encodeB64(k.id), "type": "public-key", "response": map[string]any{"clientDataJSON": encodeB64([]byte(cd)), "attestationObject": encodeB64(att), "transports": []string{"internal"}}, "clientExtensionResults": map[string]any{}}
}
func (k *testPasskey) assertion(t *testing.T, options map[string]any, mutation string) map[string]any {
	t.Helper()
	pub := options["publicKey"].(map[string]any)
	rp := pub["rpId"].(string)
	if mutation == "rp" {
		rp = "evil.example"
	}
	hash := sha256.Sum256([]byte(rp))
	data := append([]byte{}, hash[:]...)
	flags := byte(5)
	if k.backup {
		flags |= 8
	}
	if mutation == "uv" {
		flags &^= 4
	}
	data = append(data, flags)
	k.count++
	data = binary.BigEndian.AppendUint32(data, k.count)
	origin := "http://localhost"
	if mutation == "origin" {
		origin = "https://evil.example"
	}
	challenge := pub["challenge"]
	if mutation == "challenge" {
		challenge = encodeB64(make([]byte, 32))
	}
	cd := []byte(jsonText(map[string]any{"type": "webauthn.get", "challenge": challenge, "origin": origin, "crossOrigin": false}))
	cdHash := sha256.Sum256(cd)
	signed := append(data, cdHash[:]...)
	digest := sha256.Sum256(signed)
	sig, e := ecdsa.SignASN1(rand.Reader, k.key, digest[:])
	if e != nil {
		t.Fatal(e)
	}
	if mutation == "signature" {
		sig[len(sig)-1] ^= 1
	}
	handle := k.handle
	if mutation == "handle" {
		handle = []byte("other")
	}
	return map[string]any{"id": encodeB64(k.id), "rawId": encodeB64(k.id), "type": "public-key", "response": map[string]any{"clientDataJSON": encodeB64(cd), "authenticatorData": encodeB64(data), "signature": encodeB64(sig), "userHandle": encodeB64(handle)}, "clientExtensionResults": map[string]any{}}
}
func passkeyClient(t *testing.T) *client {
	c := newClient(t)
	c.s.options.PublicURL = "http://localhost"
	c.login("admin")
	return c
}
func registerTestPasskey(t *testing.T, c *client) *testPasskey {
	t.Helper()
	proof := reauth(t, c, "passkey-register", "", "")
	begin := passkeyRequest(c, "passkeys/register/begin", map[string]any{"proof": proof}, nil)
	options := authJSON(t, begin, 200)
	key := newTestPasskey(t)
	body := map[string]any{"proof": proof, "name": "test device", "credential": key.registration(t, options, 5)}
	status(t, passkeyRequest(c, "passkeys/register/finish", body, begin.Result().Cookies()), 200)
	status(t, c.request("GET", "/api/v1/auth/me", ""), 401)
	return key
}
func TestPasskeyPublicOriginPolicy(t *testing.T) {
	c := newClient(t)
	for _, tc := range []struct {
		url       string
		available bool
	}{{"", false}, {"http://192.168.1.100:7438", false}, {"http://example.com", false}, {"http://localhost:7438", true}, {"https://modudock.example.com", true}} {
		c.s.options.PublicURL = tc.url
		body := authJSON(t, c.request("GET", "/api/v1/auth/options", ""), 200)
		if body["passkey_available"] != tc.available {
			t.Errorf("%s availability %v", tc.url, body)
		}
	}
	c.s.options.PublicURL = "http://localhost"
	c.login("admin")
	registerTestPasskey(t, c)
	c.s.options.PublicURL = "https://new.example.com"
	available, _ := c.s.passkeyAvailability()
	if available {
		t.Fatal("changed RP accepted")
	}
	c.s.db.Exec("DELETE FROM passkeys")
	available, _ = c.s.passkeyAvailability()
	if !available {
		t.Fatal("empty RP could not be replaced")
	}
}
func TestPasskeyRegistrationAndDiscoverableLogin(t *testing.T) {
	c := passkeyClient(t)
	key := registerTestPasskey(t, c)
	c.cookie = nil
	begin := passkeyRequest(c, "passkey/begin", map[string]any{}, nil)
	options := authJSON(t, begin, 200)
	if options["publicKey"].(map[string]any)["userVerification"] != "required" {
		t.Fatal("UV not requested")
	}
	acceptMFASession(t, c, passkeyRequest(c, "passkey/finish", map[string]any{"credential": key.assertion(t, options, "")}, begin.Result().Cookies()))
	status(t, c.request("GET", "/api/v1/auth/me", ""), 200)
	security := authJSON(t, c.request("GET", "/api/v1/auth/security", ""), 200)
	if len(security["passkeys"].([]any)) != 1 {
		t.Fatal(security)
	}
}
func TestPasskeyRejectsInvalidAssertion(t *testing.T) {
	for _, mutation := range []string{"uv", "origin", "rp", "challenge", "handle", "signature"} {
		t.Run(mutation, func(t *testing.T) {
			c := passkeyClient(t)
			key := registerTestPasskey(t, c)
			c.cookie = nil
			begin := passkeyRequest(c, "passkey/begin", map[string]any{}, nil)
			options := authJSON(t, begin, 200)
			status(t, passkeyRequest(c, "passkey/finish", map[string]any{"credential": key.assertion(t, options, mutation)}, begin.Result().Cookies()), 401)
		})
	}
}
func TestPasskeyDeletedOrDisabledDuringFinish(t *testing.T) {
	for _, mutation := range []string{"deleted", "disabled"} {
		t.Run(mutation, func(t *testing.T) {
			c := passkeyClient(t)
			key := registerTestPasskey(t, c)
			c.cookie = nil
			begin := passkeyRequest(c, "passkey/begin", map[string]any{}, nil)
			options := authJSON(t, begin, 200)
			if mutation == "deleted" {
				c.s.db.Exec("DELETE FROM passkeys")
			} else {
				c.s.db.Exec("UPDATE users SET is_active=0")
			}
			status(t, passkeyRequest(c, "passkey/finish", map[string]any{"credential": key.assertion(t, options, "")}, begin.Result().Cookies()), 401)
		})
	}
}
func TestPasskeyReauthOwnerAndPurpose(t *testing.T) {
	c := passkeyClient(t)
	key := registerTestPasskey(t, c)
	c.login("admin")
	begin := passkeyRequest(c, "passkeys/reauth/begin", map[string]any{"purpose": "totp-setup"}, nil)
	options := authJSON(t, begin, 200)
	proof := authJSON(t, passkeyRequest(c, "passkeys/reauth/finish", map[string]any{"credential": key.assertion(t, options, "")}, begin.Result().Cookies()), 200)["proof"].(string)
	status(t, c.request("DELETE", "/api/v1/auth/passkeys/"+encodeB64(key.id), fmt.Sprintf(`{"proof":%q}`, proof)), 403)
	setupTOTP(t, c, proof)
	other := &client{t: t, s: c.s}
	other.login("admin")
	status(t, other.request("POST", "/api/v1/auth/totp/setup", fmt.Sprintf(`{"proof":%q}`, proof)), 403)
	status(t, c.request("PUT", "/api/v1/auth/passkeys/"+encodeB64(key.id), `{"name":"renamed"}`), 200)
	status(t, c.request("GET", "/api/v1/auth/me", ""), 200)
}
func TestPasskeyLimitAndConcurrentFinish(t *testing.T) {
	c := passkeyClient(t)
	key := registerTestPasskey(t, c)
	c.cookie = nil
	begin := passkeyRequest(c, "passkey/begin", map[string]any{}, nil)
	options := authJSON(t, begin, 200)
	credential := key.assertion(t, options, "")
	var wins atomic.Int32
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			if passkeyRequest(c, "passkey/finish", map[string]any{"credential": credential}, begin.Result().Cookies()).Code == 200 {
				wins.Add(1)
			}
		})
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("finish accepted %d times", wins.Load())
	}
	c.login("admin")
	for i := 1; i < 10; i++ {
		if _, e := c.s.db.Exec(`INSERT INTO passkeys(id,user_id,name,credential,created_at) SELECT ?,user_id,name,credential,created_at FROM passkeys LIMIT 1`, fmt.Sprintf("test-%d", i)); e != nil {
			t.Fatal(e)
		}
	}
	proof := reauth(t, c, "passkey-register", "", "")
	status(t, passkeyRequest(c, "passkeys/register/begin", map[string]any{"proof": proof}, nil), 409)
}
func TestPasskeyCounterAndBackupPolicy(t *testing.T) {
	for _, backup := range []bool{false, true} {
		t.Run(fmt.Sprint(backup), func(t *testing.T) {
			c := passkeyClient(t)
			proof := reauth(t, c, "passkey-register", "", "")
			begin := passkeyRequest(c, "passkeys/register/begin", map[string]any{"proof": proof}, nil)
			key := newTestPasskey(t)
			key.backup = backup
			flags := byte(5)
			if backup {
				flags |= 8
			}
			status(t, passkeyRequest(c, "passkeys/register/finish", map[string]any{"proof": proof, "name": "counter test", "credential": key.registration(t, authJSON(t, begin, 200), flags)}, begin.Result().Cookies()), 200)
			c.cookie = nil
			for attempt := 0; attempt < 2; attempt++ {
				begin = passkeyRequest(c, "passkey/begin", map[string]any{}, nil)
				options := authJSON(t, begin, 200)
				key.count = 0
				want := 200
				if attempt == 1 && !backup {
					want = 401
				}
				status(t, passkeyRequest(c, "passkey/finish", map[string]any{"credential": key.assertion(t, options, "")}, begin.Result().Cookies()), want)
			}
		})
	}
}
func TestPasskeyRegistrationRequiresUVAndFreshAccount(t *testing.T) {
	for _, mutation := range []string{"uv", "version"} {
		t.Run(mutation, func(t *testing.T) {
			c := passkeyClient(t)
			proof := reauth(t, c, "passkey-register", "", "")
			begin := passkeyRequest(c, "passkeys/register/begin", map[string]any{"proof": proof}, nil)
			options := authJSON(t, begin, 200)
			key := newTestPasskey(t)
			flags := byte(5)
			want := 401
			if mutation == "uv" {
				flags = 1
			} else {
				c.s.db.Exec("UPDATE users SET auth_version=auth_version+1")
				want = 403
			}
			status(t, passkeyRequest(c, "passkeys/register/finish", map[string]any{"proof": proof, "name": "test", "credential": key.registration(t, options, flags)}, begin.Result().Cookies()), want)
		})
	}
}
