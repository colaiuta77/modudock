// SPA 경로와 누락된 빌드 안내가 API 경로를 가리지 않는지 검증한다.
package web

import (
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestFrontendFallbackBoundaries(t *testing.T) {
	files := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<h1>console</h1>")}, "assets/app.js": &fstest.MapFile{Data: []byte("alert(1)")}}
	for _, tc := range []struct {
		path     string
		want     int
		contains string
	}{{"/settings/users", 200, "console"}, {"/api/v1/missing", 404, ""}, {"/healthz/no", 404, ""}, {"/assets/missing.js", 404, ""}, {"/assets/app.js", 200, "alert"}} {
		w := httptest.NewRecorder()
		Handler(files).ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		if w.Code != tc.want || !strings.Contains(w.Body.String(), tc.contains) {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body)
		}
	}
}
func TestMissingBuildIsExplicit(t *testing.T) {
	w := httptest.NewRecorder()
	Handler(fstest.MapFS{}).ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 503 || !strings.Contains(w.Body.String(), "프론트엔드 빌드가 없습니다") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}
