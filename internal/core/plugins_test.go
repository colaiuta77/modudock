// HTTP 핸들러와 실제 ZIP·자식 프로세스로 GitHub 설치 전 과정을 검증한다.
package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func responseFixture(r *http.Request, status int, body []byte) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body)), Request: r}
}
func githubFixture(t *testing.T, s *Server) (map[string]any, []byte) {
	t.Helper()
	exe := helperExecutable(t)
	binary, e := os.ReadFile(exe)
	if e != nil {
		t.Fatal(e)
	}
	archive := zipFixture(t, filepath.Base(exe), 0700, binary)
	digest := sha256.Sum256(archive)
	var manifest map[string]any
	json.Unmarshal(manifestFixture(), &manifest)
	manifest["artifacts"] = map[string]any{runtime.GOOS + "-" + runtime.GOARCH: map[string]any{"url": "https://github.com/example/hello/releases/download/v1/hello.zip", "sha256": hex.EncodeToString(digest[:]), "executable": filepath.Base(exe)}}
	manifest["tasks"].([]any)[0].(map[string]any)["input_schema"] = map[string]any{"type": "object"}
	manifest["menus"] = []map[string]any{{"id": "home", "title": "Hello", "icon": "Puzzle", "resource": "items"}}
	manifest["resources"] = []map[string]any{{"id": "items", "methods": []string{"GET", "POST"}, "input_schema": map[string]any{"type": "object"}, "timeout_seconds": 10}}
	s.pluginHTTP = &http.Client{Timeout: time.Second, Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.String() {
		case "https://api.github.com/repos/example/hello":
			return responseFixture(r, 200, []byte(`{"private":false,"full_name":"example/hello"}`)), nil
		case "https://api.github.com/repos/example/hello/commits/v1", "https://api.github.com/repos/example/hello/commits/v2":
			return responseFixture(r, 200, []byte(`{"sha":"`+strings.Repeat("a", 40)+`"}`)), nil
		case "https://api.github.com/repos/example/hello/contents/modudock.json?ref=" + strings.Repeat("a", 40):
			b, _ := json.Marshal(manifest)
			return responseFixture(r, 200, b), nil
		case "https://github.com/example/hello/releases/download/v1/hello.zip":
			return responseFixture(r, 200, archive), nil
		default:
			return responseFixture(r, 404, []byte("unexpected GitHub request")), nil
		}
	})}
	return manifest, archive
}
func TestGitHubHTTPInstallRunSettingsScheduleUpdateRollbackRecovery(t *testing.T) {
	c := newClient(t)
	c.login("admin")
	m, _ := githubFixture(t, c.s)
	w := c.request("POST", "/api/v1/plugins/install", `{"repository_url":"https://github.com/example/hello","ref":"v1"}`)
	status(t, w, 201)
	var initial Plugin
	if e := json.Unmarshal(w.Body.Bytes(), &initial); e != nil {
		t.Fatal(e)
	}
	status(t, c.request("PUT", "/api/v1/plugins/hello/settings", `{"token":"keep-me"}`), 200)
	status(t, c.request("GET", "/api/v1/plugins/hello/resources/items", ""), 200)
	status(t, c.request("PUT", "/api/v1/menus/hello/home", `{"title":"Customized","position":7,"enabled":true}`), 200)
	w = c.request("POST", "/api/v1/plugins/hello/tasks/greet/runs", `{"input":{}}`)
	status(t, w, 201)
	queued := runFrom(t, w)
	status(t, c.request("POST", "/api/v1/schedules", `{"name":"Hourly","plugin_id":"hello","task_id":"greet","cron":"0 * * * *","timezone":"Asia/Seoul","input":{},"enabled":true}`), 201)
	m["version"] = "1.1.0"
	w = c.request("POST", "/api/v1/plugins/hello/update", `{"ref":"v2"}`)
	status(t, w, 200)
	var updated Plugin
	json.Unmarshal(w.Body.Bytes(), &updated)
	if updated.ActiveReleaseID == initial.ActiveReleaseID {
		t.Fatal("update did not create immutable release")
	}
	oldRun, _ := c.s.run(queued.ID)
	if oldRun.ReleaseID != initial.ActiveReleaseID {
		t.Fatal("queued run lost release pin")
	}
	status(t, c.request("POST", "/api/v1/plugins/hello/activate", fmt.Sprintf(`{"release_id":%q}`, initial.ActiveReleaseID)), 200)
	settings, e := c.s.settings("hello")
	if e != nil || settings["token"] != "keep-me" {
		t.Fatalf("rollback lost settings %v %v", settings, e)
	}
	menus := c.request("GET", "/api/v1/menus", "")
	status(t, menus, 200)
	if !strings.Contains(menus.Body.String(), "Customized") {
		t.Fatal("update lost menu customization")
	}
	// Reopen does not start jobs. Explicit serve startup resumes the committed queue.
	dir := c.s.dataDir
	if e = c.s.Close(); e != nil {
		t.Fatal(e)
	}
	s, e := Open(dir, Options{})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	before, e := s.run(queued.ID)
	if e != nil || before.State != "queued" {
		t.Fatalf("queue not durable %+v %v", before, e)
	}
	if e = s.StartBackground(); e != nil {
		t.Fatal(e)
	}
	done := waitRun(t, s, queued.ID, "succeeded")
	if done.ReleaseID != initial.ActiveReleaseID {
		t.Fatal("recovered job used new release")
	}
}
func TestFailedUpdateLeavesActiveReleaseAndDataUntouched(t *testing.T) {
	c := newClient(t)
	c.login("admin")
	m, _ := githubFixture(t, c.s)
	status(t, c.request("POST", "/api/v1/plugins/install", `{"repository_url":"https://github.com/example/hello","ref":"v1"}`), 201)
	initial, _ := c.s.plugin("hello")
	dataPath := filepath.Join(c.s.dataDir, "plugin-data", "hello", "persistent.txt")
	if e := os.WriteFile(dataPath, []byte("preserve"), 0600); e != nil {
		t.Fatal(e)
	}
	m["data_schema_version"] = 2
	status(t, c.request("POST", "/api/v1/plugins/hello/update", `{"ref":"v2"}`), 400)
	m["data_schema_version"] = 1
	a := m["artifacts"].(map[string]any)[runtime.GOOS+"-"+runtime.GOARCH].(map[string]any)
	good := a["sha256"]
	a["sha256"] = strings.Repeat("0", 64)
	status(t, c.request("POST", "/api/v1/plugins/hello/update", `{"ref":"v2"}`), 400)
	a["sha256"] = good
	a["url"] = "https://github.com/attacker/other/releases/download/v1/hello.zip"
	status(t, c.request("POST", "/api/v1/plugins/hello/update", `{"ref":"v2"}`), 400)
	current, _ := c.s.plugin("hello")
	if current.ActiveReleaseID != initial.ActiveReleaseID {
		t.Fatal("failed update switched active release")
	}
	data, e := os.ReadFile(dataPath)
	if e != nil || string(data) != "preserve" {
		t.Fatalf("plugin data changed %q %v", data, e)
	}
	var releases int
	c.s.db.QueryRow("SELECT count(*) FROM releases").Scan(&releases)
	if releases != 1 {
		t.Fatal("failed release became visible")
	}
}
func TestGitHubURLAndRedirectBoundaries(t *testing.T) {
	for _, raw := range []string{"http://github.com/a/b", "https://github.com:443/a/b", "https://github.com@127.0.0.1/a/b", "https://github.com/a/b?x=1", "https://github.com/a/%2e%2e", "https://localhost/a/b", "https://github.com/a/b/extra"} {
		if _, e := githubRepository(raw); e == nil {
			t.Fatalf("unsafe repo accepted %s", raw)
		}
	}
	c := newClient(t)
	for _, target := range []string{"http://127.0.0.1/private", "https://github.com/other/repo/releases/download/v1/x.zip", "https://api.github.com/repos/other/repo/secret", "https://release-assets.githubusercontent.com.evil.test/x"} {
		c.s.pluginHTTP = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
			response := responseFixture(r, 302, nil)
			response.Header.Set("Location", target)
			return response, nil
		})}
		if _, e := c.s.download(t.Context(), "https://github.com/example/hello/releases/download/v1/hello.zip", "example/hello", true, 1024); e == nil {
			t.Fatalf("unsafe redirect accepted %s", target)
		}
	}
	c.s.pluginHTTP = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "github.com" {
			response := responseFixture(r, 302, nil)
			response.Header.Set("Location", "https://release-assets.githubusercontent.com/github-production-release-asset/x.zip?sig=fixture")
			return response, nil
		}
		return responseFixture(r, 200, []byte("ok")), nil
	})}
	data, e := c.s.download(t.Context(), "https://github.com/example/hello/releases/download/v1/hello.zip", "example/hello", true, 10)
	if e != nil || string(data) != "ok" {
		t.Fatalf("legitimate GitHub release redirect rejected %q %v", data, e)
	}
}
func TestPluginDownloadRejectsNonPublicAddresses(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "10.0.0.1", "192.168.1.1", "169.254.169.254", "100.64.0.1", "192.0.2.1", "198.18.0.1", "::1", "fc00::1", "fe80::1", "2001:db8::1", "::ffff:127.0.0.1"} {
		if publicPluginIP(net.ParseIP(raw)) {
			t.Errorf("nonpublic address accepted %s", raw)
		}
	}
	for _, raw := range []string{"140.82.112.3", "185.199.108.133", "2606:50c0:8000::154"} {
		if !publicPluginIP(net.ParseIP(raw)) {
			t.Errorf("public GitHub address rejected %s", raw)
		}
	}
}
