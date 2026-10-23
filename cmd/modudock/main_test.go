// CLI 비밀 입력과 상태 확인 명령의 실제 동작을 검증한다.
package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIAdminUsesStdinAndLocks(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	var output bytes.Buffer
	err := run([]string{"admin", "create", "--data-dir", dir, "--username", "admin", "--password-stdin"}, strings.NewReader("test-password-1234\n"), &output)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "test-password") {
		t.Fatal("password printed")
	}
	err = run([]string{"admin", "create", "--data-dir", dir, "--username", "admin", "--password", "exposed"}, strings.NewReader(""), &output)
	if err == nil {
		t.Fatal("password argv accepted")
	}
}
func TestHealthcheckRequiresExactHealth(t *testing.T) {
	for _, tc := range []struct {
		body   string
		status int
		ok     bool
	}{{`{"status":"ok"}`, 200, true}, {`{"status":"bad"}`, 200, false}, {`{"status":"ok"}`, 500, false}, {`<html>ok</html>`, 200, false}} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); w.Write([]byte(tc.body)) }))
		var out bytes.Buffer
		e := run([]string{"healthcheck", "--url", server.URL}, strings.NewReader(""), &out)
		server.Close()
		if (e == nil) != tc.ok {
			t.Fatalf("body %s error %v", tc.body, e)
		}
	}
}
