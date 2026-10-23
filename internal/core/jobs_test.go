// 실제 SQLite에서 작업 상태·중복 요청·재시작·릴리스 고정을 검증한다.
package core

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func seedPlugin(t *testing.T, s *Server, exe string) string {
	t.Helper()
	var m map[string]any
	json.Unmarshal(manifestFixture(), &m)
	m["tasks"].([]any)[0].(map[string]any)["input_schema"] = map[string]any{"type": "object"}
	m["menus"] = []map[string]any{{"id": "home", "title": "Hello", "icon": "Puzzle"}}
	m["resources"] = []map[string]any{{"id": "items", "methods": []string{"GET", "POST"}, "input_schema": map[string]any{"type": "object"}, "timeout_seconds": 10}}
	data, _ := json.Marshal(m)
	release := randomID()
	if _, e := s.db.Exec("INSERT INTO releases(id,plugin_id,version,commit_sha,data_schema_version,created_at,manifest,executable) VALUES(?,'hello','1.0.0',?,1,?,?,?)", release, strings.Repeat("a", 40), nowString(), string(data), exe); e != nil {
		t.Fatal(e)
	}
	if _, e := s.db.Exec("INSERT INTO plugins(id,state,repository_url,active_release_id,settings) VALUES('hello','active','https://github.com/example/hello',?,'{}')", release); e != nil {
		t.Fatal(e)
	}
	manifest, _ := parseManifest(data)
	tx, e := s.db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	if e = syncMenus(tx, manifest); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	return release
}
func runFrom(t *testing.T, w *httptest.ResponseRecorder) Run {
	t.Helper()
	var run Run
	if e := json.Unmarshal(w.Body.Bytes(), &run); e != nil {
		t.Fatal(e)
	}
	return run
}
func waitRun(t *testing.T, s *Server, id, state string) Run {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		run, e := s.run(id)
		if e != nil {
			t.Fatal(e)
		}
		if run.State == state {
			return run
		}
		if run.State == "failed" && state != "failed" {
			t.Fatalf("unexpected failed run %+v", run)
		}
		time.Sleep(15 * time.Millisecond)
	}
	run, _ := s.run(id)
	t.Fatalf("run did not reach %s: %+v", state, run)
	return Run{}
}
func TestQueuePersistsUntilExplicitBackgroundStart(t *testing.T) {
	c := newClient(t)
	c.login("admin")
	release := seedPlugin(t, c.s, helperExecutable(t))
	w := c.request("POST", "/api/v1/plugins/hello/tasks/greet/runs", `{"input":{}}`)
	status(t, w, 201)
	run := runFrom(t, w)
	if run.State != "queued" || run.ReleaseID != release {
		t.Fatalf("uncommitted/pinned queue %+v", run)
	}
	time.Sleep(50 * time.Millisecond)
	run, _ = c.s.run(run.ID)
	if run.State != "queued" {
		t.Fatal("Open/admin path started executing jobs")
	}
	if e := c.s.StartBackground(); e != nil {
		t.Fatal(e)
	}
	result := waitRun(t, c.s, run.ID, "succeeded")
	if result.Progress != 100 || result.Result["message"] != "hello" {
		t.Fatalf("invalid result %+v", result)
	}
	status(t, c.request("GET", "/api/v1/status", ""), 200)
}
func TestQueueCancelDisableAndRestartRecovery(t *testing.T) {
	c := newClient(t)
	c.login("admin")
	seedPlugin(t, c.s, helperExecutable(t))
	first := c.request("POST", "/api/v1/plugins/hello/tasks/greet/runs", `{"input":{"mode":"hang"}}`)
	status(t, first, 201)
	firstRun := runFrom(t, first)
	if e := c.s.StartBackground(); e != nil {
		t.Fatal(e)
	}
	waitRun(t, c.s, firstRun.ID, "running")
	status(t, c.request("POST", "/api/v1/runs/"+firstRun.ID+"/cancel", ""), 200)
	waitRun(t, c.s, firstRun.ID, "cancelled")
	// Closing interrupts a real child; queued entries survive and Open itself leaves both untouched.
	second := c.request("POST", "/api/v1/plugins/hello/tasks/greet/runs", `{"input":{"mode":"hang"}}`)
	status(t, second, 201)
	id := runFrom(t, second).ID
	waitRun(t, c.s, id, "running")
	dir := c.s.dataDir
	if e := c.s.Close(); e != nil {
		t.Fatal(e)
	}
	s, e := Open(dir, Options{})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	r, e := s.run(id)
	if e != nil || r.State != "interrupted" {
		t.Fatalf("shutdown did not interrupt %+v %v", r, e)
	}
	// Simulate a process crash that left running state committed.
	if _, e = s.db.Exec("UPDATE runs SET state='running' WHERE id=?", id); e != nil {
		t.Fatal(e)
	}
	if e = s.StartBackground(); e != nil {
		t.Fatal(e)
	}
	waitRun(t, s, id, "interrupted")
}
func TestIdempotencyComparesPayloadAndQueuedReleaseStaysPinned(t *testing.T) {
	c := newClient(t)
	c.login("admin")
	release := seedPlugin(t, c.s, helperExecutable(t))
	submit := func(body, key string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://localhost/api/v1/plugins/hello/tasks/greet/runs", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", "http://localhost")
		r.Header.Set("X-CSRF-Token", c.csrf)
		r.Header.Set("Idempotency-Key", key)
		r.AddCookie(c.cookie)
		w := httptest.NewRecorder()
		c.s.Handler().ServeHTTP(w, r)
		return w
	}
	first := submit(`{"input":{"name":"Ada","mode":"env"}}`, "same")
	status(t, first, 201)
	again := submit(`{"input":{"mode":"env","name":"Ada"}}`, "same")
	status(t, again, 201)
	if runFrom(t, first).ID != runFrom(t, again).ID {
		t.Fatal("duplicate idempotency execution")
	}
	status(t, submit(`{"input":{"name":"Other"}}`, "same"), 409)
	var manifest, exe string
	c.s.db.QueryRow("SELECT manifest,executable FROM releases WHERE id=?", release).Scan(&manifest, &exe)
	next := randomID()
	if _, e := c.s.db.Exec("INSERT INTO releases(id,plugin_id,version,commit_sha,data_schema_version,created_at,manifest,executable) VALUES(?,'hello','2.0.0',?,1,?,?,?)", next, strings.Repeat("b", 40), nowString(), manifest, exe); e != nil {
		t.Fatal(e)
	}
	status(t, c.request("POST", "/api/v1/plugins/hello/activate", fmt.Sprintf(`{"release_id":%q}`, next)), 200)
	queued, _ := c.s.run(runFrom(t, first).ID)
	if queued.ReleaseID != release {
		t.Fatal("update rewrote queued release")
	}
	status(t, c.request("PUT", "/api/v1/plugins/hello/state", `{"enabled":false}`), 200)
	waitRun(t, c.s, queued.ID, "cancelled")
	status(t, c.request("POST", "/api/v1/plugins/hello/tasks/greet/runs", `{"input":{}}`), 409)
}
func TestSettingsMasksAndPreservesSecrets(t *testing.T) {
	c := newClient(t)
	c.login("admin")
	seedPlugin(t, c.s, helperExecutable(t))
	w := c.request("PUT", "/api/v1/plugins/hello/settings", `{"token":"very-secret"}`)
	status(t, w, 200)
	if strings.Contains(w.Body.String(), "very-secret") || !strings.Contains(w.Body.String(), "********") {
		t.Fatal("secret not masked")
	}
	for _, body := range []string{`{"token":"********"}`, `{"token":""}`} {
		status(t, c.request("PUT", "/api/v1/plugins/hello/settings", body), 200)
		settings, e := c.s.settings("hello")
		if e != nil || settings["token"] != "very-secret" {
			t.Fatalf("secret overwritten %v %v", settings, e)
		}
	}
	status(t, c.request("POST", "/api/v1/users", fmt.Sprintf(`{"username":"viewer","password":%q,"role":"viewer"}`, password)), 201)
	c.login("viewer")
	status(t, c.request("GET", "/api/v1/plugins/hello/settings", ""), 403)
	status(t, c.request("GET", "/api/v1/plugins/hello/resources/items", ""), 200)
}
func TestSettingsMasksLocalRefAndCompositionSecrets(t *testing.T) {
	c := newClient(t)
	c.login("admin")
	release := seedPlugin(t, c.s, helperExecutable(t))
	var raw string
	c.s.db.QueryRow("SELECT manifest FROM releases WHERE id=?", release).Scan(&raw)
	var m map[string]any
	json.Unmarshal([]byte(raw), &m)
	m["settings_schema"] = map[string]any{"type": "object", "$defs": map[string]any{"credential": map[string]any{"type": "string", "writeOnly": true}}, "properties": map[string]any{"token": map[string]any{"$ref": "#/$defs/credential"}, "nested": map[string]any{"type": "object", "allOf": []any{map[string]any{"properties": map[string]any{"key": map[string]any{"type": "string", "format": "password"}}}}}}, "additionalProperties": false}
	b, _ := json.Marshal(m)
	if _, e := c.s.db.Exec("UPDATE releases SET manifest=? WHERE id=?", string(b), release); e != nil {
		t.Fatal(e)
	}
	for _, method := range []string{"PUT", "GET"} {
		body := ""
		if method == "PUT" {
			body = `{"token":"secret-via-ref","nested":{"key":"secret-via-allof"}}`
		}
		w := c.request(method, "/api/v1/plugins/hello/settings", body)
		status(t, w, 200)
		if strings.Contains(w.Body.String(), "secret-via") {
			t.Fatalf("indirect schema secret leaked %s", w.Body)
		}
	}
	status(t, c.request("PUT", "/api/v1/plugins/hello/settings", `{"token":"********","nested":{"key":""}}`), 200)
	settings, e := c.s.settings("hello")
	if e != nil || settings["token"] != "secret-via-ref" || settings["nested"].(map[string]any)["key"] != "secret-via-allof" {
		t.Fatalf("indirect secret not preserved %v %v", settings, e)
	}
}
func TestPluginStderrRedactsSettings(t *testing.T) {
	c := newClient(t)
	c.login("admin")
	seedPlugin(t, c.s, helperExecutable(t))
	status(t, c.request("PUT", "/api/v1/plugins/hello/settings", `{"token":"do-not-log-this"}`), 200)
	w := c.request("POST", "/api/v1/plugins/hello/tasks/greet/runs", `{"input":{"mode":"secret_log"}}`)
	status(t, w, 201)
	run := runFrom(t, w)
	if e := c.s.StartBackground(); e != nil {
		t.Fatal(e)
	}
	waitRun(t, c.s, run.ID, "succeeded")
	w = c.request("GET", "/api/v1/logs?run_id="+run.ID, "")
	status(t, w, 200)
	if strings.Contains(w.Body.String(), "do-not-log-this") || !strings.Contains(w.Body.String(), "[REDACTED]") {
		t.Fatalf("stderr secret leaked %s", w.Body)
	}
}
func TestPermittedStderrKeepsLogAPIReadable(t *testing.T) {
	c := newClient(t)
	c.login("admin")
	seedPlugin(t, c.s, helperExecutable(t))
	w := c.request("POST", "/api/v1/plugins/hello/tasks/greet/runs", `{"input":{"mode":"quoted_log"}}`)
	status(t, w, 201)
	run := runFrom(t, w)
	if e := c.s.StartBackground(); e != nil {
		t.Fatal(e)
	}
	waitRun(t, c.s, run.ID, "succeeded")
	status(t, c.request("GET", "/api/v1/logs?run_id="+run.ID, ""), 200)
}
func TestJobsRetryOnlyIdempotentAtMostThreeAttempts(t *testing.T) {
	for _, idempotent := range []bool{true, false} {
		t.Run(fmt.Sprint(idempotent), func(t *testing.T) {
			c := newClient(t)
			c.login("admin")
			release := seedPlugin(t, c.s, helperExecutable(t))
			var raw string
			c.s.db.QueryRow("SELECT manifest FROM releases WHERE id=?", release).Scan(&raw)
			var m map[string]any
			json.Unmarshal([]byte(raw), &m)
			m["tasks"].([]any)[0].(map[string]any)["idempotent"] = idempotent
			b, _ := json.Marshal(m)
			if _, e := c.s.db.Exec("UPDATE releases SET manifest=? WHERE id=?", string(b), release); e != nil {
				t.Fatal(e)
			}
			w := c.request("POST", "/api/v1/plugins/hello/tasks/greet/runs", `{"input":{"mode":"fail"}}`)
			status(t, w, 201)
			id := runFrom(t, w).ID
			if e := c.s.StartBackground(); e != nil {
				t.Fatal(e)
			}
			waitRun(t, c.s, id, "failed")
			var attempts int
			c.s.db.QueryRow("SELECT attempts FROM runs WHERE id=?", id).Scan(&attempts)
			want := 1
			if idempotent {
				want = 3
			}
			if attempts != want {
				t.Fatalf("attempts=%d want=%d", attempts, want)
			}
		})
	}
}
func TestQueueTimeoutAndConcurrencyBound(t *testing.T) {
	c := newClient(t)
	c.login("admin")
	release := seedPlugin(t, c.s, helperExecutable(t))
	ids := []string{}
	for i := 0; i < 5; i++ {
		w := c.request("POST", "/api/v1/plugins/hello/tasks/greet/runs", `{"input":{"mode":"hang"}}`)
		status(t, w, 201)
		ids = append(ids, runFrom(t, w).ID)
	}
	if e := c.s.StartBackground(); e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(2 * time.Second)
	running, queued := 0, 0
	for time.Now().Before(deadline) {
		c.s.db.QueryRow("SELECT count(*) FROM runs WHERE state='running'").Scan(&running)
		if running == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	c.s.db.QueryRow("SELECT count(*) FROM runs WHERE state='queued'").Scan(&queued)
	if running != 2 || queued != 3 {
		t.Fatalf("concurrency running=%d queued=%d", running, queued)
	}
	status(t, c.request("PUT", "/api/v1/plugins/hello/state", `{"enabled":false}`), 200)
	for _, id := range ids {
		status(t, c.request("POST", "/api/v1/runs/"+id+"/cancel", ""), 200)
	}
	status(t, c.request("PUT", "/api/v1/plugins/hello/state", `{"enabled":true}`), 200)
	var raw string
	c.s.db.QueryRow("SELECT manifest FROM releases WHERE id=?", release).Scan(&raw)
	var m map[string]any
	json.Unmarshal([]byte(raw), &m)
	m["tasks"].([]any)[0].(map[string]any)["timeout_seconds"] = 1
	b, _ := json.Marshal(m)
	if _, e := c.s.db.Exec("UPDATE releases SET manifest=? WHERE id=?", string(b), release); e != nil {
		t.Fatal(e)
	}
	w := c.request("POST", "/api/v1/plugins/hello/tasks/greet/runs", `{"input":{"mode":"hang"}}`)
	status(t, w, 201)
	done := waitRun(t, c.s, runFrom(t, w).ID, "failed")
	if done.Error != "Task timeout" {
		t.Fatalf("timeout not persisted %+v", done)
	}
}
func TestViewerRunRedactionAndQueueBacklog(t *testing.T) {
	c := newClient(t)
	c.login("admin")
	seedPlugin(t, c.s, helperExecutable(t))
	w := c.request("POST", "/api/v1/plugins/hello/tasks/greet/runs", `{"input":{"name":"private-input"}}`)
	status(t, w, 201)
	run := runFrom(t, w)
	if e := c.s.StartBackground(); e != nil {
		t.Fatal(e)
	}
	waitRun(t, c.s, run.ID, "succeeded")
	status(t, c.request("POST", "/api/v1/users", fmt.Sprintf(`{"username":"viewer","password":%q,"role":"viewer"}`, password)), 201)
	c.login("viewer")
	w = c.request("GET", "/api/v1/runs/"+run.ID, "")
	status(t, w, 200)
	var visible map[string]any
	json.Unmarshal(w.Body.Bytes(), &visible)
	if visible["input"] != nil || visible["result"] != nil || visible["error"] != "" {
		t.Fatalf("viewer received private run fields %s", w.Body)
	}
	status(t, c.request("POST", "/api/v1/plugins/hello/tasks/greet/runs", `{"input":{}}`), 403)
	// Backlog capacity is tested without workers, against committed DB rows.
	c2 := newClient(t)
	c2.login("admin")
	seedPlugin(t, c2.s, helperExecutable(t))
	tx, e := c2.s.db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < maxQueuedJobs; i++ {
		if _, e = enqueueTx(tx, "hello", "greet", map[string]any{}, "test-user", "", "", ""); e != nil {
			t.Fatal(e)
		}
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	status(t, c2.request("POST", "/api/v1/plugins/hello/tasks/greet/runs", `{"input":{}}`), 429)
}
