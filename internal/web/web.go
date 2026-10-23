// 내장 프론트엔드와 개발 안내 화면을 안전하게 제공한다.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var assets embed.FS

func FS() fs.FS { f, _ := fs.Sub(assets, "dist"); return f }
func Handler(files fs.FS) http.Handler {
	if files == nil {
		files = FS()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api") || strings.HasPrefix(r.URL.Path, "/healthz") {
			http.NotFound(w, r)
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			w.WriteHeader(405)
			return
		}
		if _, e := fs.Stat(files, "index.html"); e != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(503)
			w.Write([]byte("<!doctype html><html lang=\"ko\"><meta charset=\"utf-8\"><title>ModuDock Go</title><h1>프론트엔드 빌드가 없습니다.</h1><p>개발자는 frontend를 빌드하여 internal/web/dist에 배치하세요. API는 사용할 수 있습니다.</p></html>"))
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}
		if info, e := fs.Stat(files, name); e != nil || info.IsDir() {
			if path.Ext(name) != "" {
				http.NotFound(w, r)
				return
			}
			name = "index.html"
		}
		copy := r.Clone(r.Context())
		copy.URL.Path = "/" + name
		if name == "index.html" {
			copy.URL.Path = "/"
		}
		http.FileServer(http.FS(files)).ServeHTTP(w, copy)
	})
}
