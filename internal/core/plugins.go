// GitHub 플러그인 릴리스 설치와 설정·메뉴·리소스 API를 제공한다.
package core

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

const maxArchiveBytes = 32 << 20
const maxExpandedBytes = 64 << 20
const secretMask = "********"

var installGate = make(chan struct{}, 1)
var githubName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$`)
var commitPattern = regexp.MustCompile(`^[a-f0-9]{40}$`)
var nonPublicPluginNetworks = func() []*net.IPNet {
	networks := []*net.IPNet{}
	for _, raw := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32"} {
		_, network, _ := net.ParseCIDR(raw)
		networks = append(networks, network)
	}
	return networks
}()

func publicPluginIP(ip net.IP) bool {
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, network := range nonPublicPluginNetworks {
		if network.Contains(ip) {
			return false
		}
	}
	return true
}

type Plugin struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Version         string   `json:"version"`
	State           string   `json:"state"`
	RepositoryURL   string   `json:"repository_url"`
	ActiveReleaseID string   `json:"active_release_id"`
	Manifest        Manifest `json:"manifest"`
}
type Release struct {
	ID                string `json:"id"`
	Version           string `json:"version"`
	CommitSHA         string `json:"commit_sha"`
	DataSchemaVersion int    `json:"data_schema_version"`
	CreatedAt         string `json:"created_at"`
}
type Menu struct {
	PluginID   string `json:"plugin_id"`
	PluginName string `json:"plugin_name"`
	ID         string `json:"id"`
	Title      string `json:"title"`
	Icon       string `json:"icon"`
	Position   int    `json:"position"`
	Enabled    bool   `json:"enabled"`
}

func githubRepository(raw string) (string, error) {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" {
		return "", errors.New("expected public HTTPS GitHub repository")
	}
	parts := strings.Split(strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git"), "/")
	if len(parts) != 2 || !githubName.MatchString(parts[0]) || !githubName.MatchString(parts[1]) {
		return "", errors.New("invalid GitHub repository")
	}
	return strings.ToLower(strings.Join(parts, "/")), nil
}
func repositoryFromArtifact(raw string) (string, error) {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" {
		return "", errors.New("artifact must be a matching GitHub release ZIP")
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) != 6 || parts[2] != "releases" || parts[3] != "download" || parts[4] == "" || parts[5] == "" || !strings.HasSuffix(strings.ToLower(parts[5]), ".zip") {
		return "", errors.New("artifact must be a release ZIP")
	}
	return githubRepository("https://github.com/" + parts[0] + "/" + parts[1])
}
func githubClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	transport := &http.Transport{Proxy: nil, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 15 * time.Second, MaxIdleConns: 4, IdleConnTimeout: 30 * time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(address)
		if e != nil {
			return nil, e
		}
		ips, e := net.DefaultResolver.LookupIPAddr(ctx, host)
		if e != nil {
			return nil, e
		}
		if len(ips) == 0 {
			return nil, errors.New("DNS has no public address")
		}
		for _, ip := range ips {
			if !publicPluginIP(ip.IP) {
				return nil, errors.New("private download address forbidden")
			}
		}
		// Dial the validated address, not the hostname, to close DNS-rebinding races.
		return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
	}
	return &http.Client{Transport: transport, Timeout: 60 * time.Second}
}
func (s *Server) download(ctx context.Context, raw, repo string, artifact bool, limit int64) ([]byte, error) {
	validate := func(u *url.URL, redirect bool) bool {
		if u.Scheme != "https" || u.User != nil || u.Fragment != "" || u.Port() != "" {
			return false
		}
		if artifact {
			if u.Host == "github.com" {
				r, e := repositoryFromArtifact(u.String())
				return e == nil && r == repo
			}
			return redirect && u.Host == "release-assets.githubusercontent.com"
		}
		root := "/repos/" + repo
		path := strings.ToLower(u.EscapedPath())
		return u.Host == "api.github.com" && (path == root || strings.HasPrefix(path, root+"/"))
	}
	u, e := url.Parse(raw)
	if e != nil || !validate(u, false) {
		return nil, errors.New("download URL forbidden")
	}
	client := *s.pluginHTTP
	client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) >= 3 || !validate(r.URL, true) {
			return errors.New("download redirect forbidden")
		}
		return nil
	}
	request, e := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if e != nil {
		return nil, e
	}
	request.Header.Set("Accept", "application/vnd.github.raw+json")
	request.Header.Set("User-Agent", "ModuDock-Go")
	response, e := client.Do(request)
	if e != nil {
		return nil, e
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength > limit {
		return nil, errors.New("download exceeds limit")
	}
	data, e := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if e != nil {
		return nil, e
	}
	if int64(len(data)) > limit {
		return nil, errors.New("download exceeds limit")
	}
	return data, nil
}
func extractArchive(data []byte, dir string) error {
	z, e := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if e != nil {
		return e
	}
	if len(z.File) == 0 || len(z.File) > 256 {
		return errors.New("invalid ZIP entry count")
	}
	seen := map[string]bool{}
	var total uint64
	for _, f := range z.File {
		key := strings.ToLower(strings.TrimSuffix(f.Name, "/"))
		mode := f.Mode()
		if !safeArchivePath(f.Name) || seen[key] || (!mode.IsRegular() && !mode.IsDir()) {
			return errors.New("unsafe ZIP entry")
		}
		seen[key] = true
		if f.UncompressedSize64 > maxExpandedBytes || total+f.UncompressedSize64 > maxExpandedBytes || (f.UncompressedSize64 > 1<<20 && f.UncompressedSize64/(f.CompressedSize64+1) > 200) {
			return errors.New("ZIP expansion limit exceeded")
		}
		total += f.UncompressedSize64
	}
	for _, f := range z.File {
		target := filepath.Join(dir, filepath.FromSlash(f.Name))
		if f.FileInfo().IsDir() {
			if e = os.MkdirAll(target, 0700); e != nil {
				return e
			}
			continue
		}
		if e = os.MkdirAll(filepath.Dir(target), 0700); e != nil {
			return e
		}
		input, e := f.Open()
		if e != nil {
			return e
		}
		output, e := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			input.Close()
			return e
		}
		n, copyErr := io.Copy(output, io.LimitReader(input, int64(f.UncompressedSize64)+1))
		closeErr := output.Close()
		input.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if n != int64(f.UncompressedSize64) {
			return errors.New("invalid ZIP expanded length")
		}
	}
	return nil
}
func (s *Server) plugin(id string) (Plugin, error) {
	var p Plugin
	var manifest string
	e := s.db.QueryRow(`SELECT p.id,p.state,p.repository_url,p.active_release_id,r.manifest FROM plugins p JOIN releases r ON r.id=p.active_release_id WHERE p.id=?`, id).Scan(&p.ID, &p.State, &p.RepositoryURL, &p.ActiveReleaseID, &manifest)
	if e != nil {
		return p, e
	}
	if e = json.Unmarshal([]byte(manifest), &p.Manifest); e != nil {
		return p, e
	}
	p.Name = p.Manifest.Name
	p.Version = p.Manifest.Version
	return p, nil
}
func (s *Server) release(id string) (Manifest, string, error) {
	var manifest, executable string
	e := s.db.QueryRow("SELECT manifest,executable FROM releases WHERE id=?", id).Scan(&manifest, &executable)
	var m Manifest
	if e == nil {
		e = json.Unmarshal([]byte(manifest), &m)
	}
	return m, executable, e
}
func (s *Server) settings(id string) (map[string]any, error) {
	var data string
	if e := s.db.QueryRow("SELECT settings FROM plugins WHERE id=?", id).Scan(&data); e != nil {
		return nil, e
	}
	settings := map[string]any{}
	e := json.Unmarshal([]byte(data), &settings)
	return settings, e
}
func (s *Server) install(ctx context.Context, repository, ref, expectedID string) (Plugin, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	select {
	case installGate <- struct{}{}:
		defer func() { <-installGate }()
	case <-ctx.Done():
		return Plugin{}, ctx.Err()
	}
	repo, e := githubRepository(repository)
	if e != nil {
		return Plugin{}, e
	}
	if ref == "" || len(ref) > 200 || strings.ContainsAny(ref, "\x00\r\n") {
		return Plugin{}, errors.New("invalid ref")
	}
	root := "https://api.github.com/repos/" + repo
	// Explicit public metadata avoids silently installing a private repository if credentials are ever added.
	metadata, e := s.download(ctx, root, repo, false, 64<<10)
	if e != nil {
		return Plugin{}, e
	}
	var info struct {
		Private  *bool  `json:"private"`
		FullName string `json:"full_name"`
	}
	if json.Unmarshal(metadata, &info) != nil || info.Private == nil || *info.Private || strings.ToLower(info.FullName) != repo {
		return Plugin{}, errors.New("repository must be public")
	}
	commitData, e := s.download(ctx, root+"/commits/"+url.PathEscape(ref), repo, false, 1<<20)
	if e != nil {
		return Plugin{}, e
	}
	var commit struct {
		SHA string `json:"sha"`
	}
	if json.Unmarshal(commitData, &commit) != nil || !commitPattern.MatchString(commit.SHA) {
		return Plugin{}, errors.New("invalid commit SHA")
	}
	data, e := s.download(ctx, root+"/contents/modudock.json?ref="+commit.SHA, repo, false, 64<<10)
	if e != nil {
		return Plugin{}, e
	}
	m, e := parseManifest(data)
	if e != nil {
		return Plugin{}, e
	}
	if expectedID != "" && m.ID != expectedID {
		return Plugin{}, errors.New("update changed plugin ID")
	}
	for _, a := range m.Artifacts {
		r, e := repositoryFromArtifact(a.URL)
		if e != nil || r != repo {
			return Plugin{}, errors.New("artifact belongs to another repository")
		}
	}
	old, e := s.plugin(m.ID)
	exists := e == nil
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return Plugin{}, e
	}
	settings := map[string]any{}
	if exists {
		if old.RepositoryURL != "https://github.com/"+repo || old.Manifest.DataSchemaVersion != m.DataSchemaVersion {
			return Plugin{}, errors.New("repository or data schema mismatch")
		}
		settings, e = s.settings(m.ID)
		if e != nil {
			return Plugin{}, e
		}
		if e = validateInput(m.SettingsSchema, settings); e != nil {
			return Plugin{}, errors.New("stored settings incompatible with release")
		}
	}
	a, ok := m.Artifacts[runtime.GOOS+"-"+runtime.GOARCH]
	if !ok {
		return Plugin{}, errors.New("no artifact for this platform")
	}
	archive, e := s.download(ctx, a.URL, repo, true, maxArchiveBytes)
	if e != nil {
		return Plugin{}, e
	}
	digest := sha256.Sum256(archive)
	if !strings.EqualFold(hex.EncodeToString(digest[:]), a.SHA256) {
		return Plugin{}, errors.New("artifact SHA-256 mismatch")
	}
	releaseID := randomID()
	parent := filepath.Join(s.dataDir, "plugins", m.ID, "releases")
	if e = os.MkdirAll(parent, 0700); e != nil {
		return Plugin{}, e
	}
	dir, e := os.MkdirTemp(parent, ".install-")
	if e != nil {
		return Plugin{}, e
	}
	defer func() { os.RemoveAll(dir) }()
	if e = extractArchive(archive, dir); e != nil {
		return Plugin{}, e
	}
	executable := filepath.Join(dir, filepath.FromSlash(a.Executable))
	stat, e := os.Lstat(executable)
	if e != nil || !stat.Mode().IsRegular() {
		return Plugin{}, errors.New("artifact executable missing")
	}
	if e = os.Chmod(executable, 0700); e != nil {
		return Plugin{}, e
	}
	stable := filepath.Join(parent, releaseID)
	if e = os.Rename(dir, stable); e != nil {
		return Plugin{}, e
	}
	dir = stable
	executable = filepath.Join(stable, filepath.FromSlash(a.Executable))
	dataDir := filepath.Join(s.dataDir, "plugin-data", m.ID)
	if e = os.MkdirAll(dataDir, 0700); e != nil {
		return Plugin{}, e
	}
	healthCtx, healthCancel := context.WithTimeout(ctx, 10*time.Second)
	result, e := runPlugin(healthCtx, executable, pluginRequest{ProtocolVersion: 1, Operation: "health", Input: map[string]any{}, Settings: settings, DataDir: dataDir}, nil, nil)
	healthCancel()
	if e != nil || result["healthy"] != true || result["protocol_version"] != float64(1) {
		return Plugin{}, errors.New("plugin health/protocol check failed")
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return Plugin{}, e
	}
	defer tx.Rollback()
	if _, e = tx.Exec(`INSERT INTO releases(id,plugin_id,version,commit_sha,data_schema_version,created_at,manifest,executable) VALUES(?,?,?,?,?,?,?,?)`, releaseID, m.ID, m.Version, commit.SHA, m.DataSchemaVersion, nowString(), string(data), executable); e != nil {
		return Plugin{}, e
	}
	if exists {
		_, e = tx.Exec("UPDATE plugins SET active_release_id=? WHERE id=?", releaseID, m.ID)
	} else {
		_, e = tx.Exec("INSERT INTO plugins(id,state,repository_url,active_release_id,settings) VALUES(?,'active',?,?,'{}')", m.ID, "https://github.com/"+repo, releaseID)
	}
	if e != nil {
		return Plugin{}, e
	}
	if e = syncMenus(tx, m); e != nil {
		return Plugin{}, e
	}
	if e = tx.Commit(); e != nil {
		return Plugin{}, e
	}
	dir = ""
	s.notify()
	return s.plugin(m.ID)
}
func syncMenus(tx *sql.Tx, m Manifest) error {
	for i, menu := range m.Menus {
		_, e := tx.Exec(`INSERT INTO menus(plugin_id,id,title,icon,position,enabled) VALUES(?,?,?,?,?,1) ON CONFLICT(plugin_id,id) DO UPDATE SET icon=excluded.icon`, m.ID, menu.ID, menu.Title, menu.Icon, i)
		if e != nil {
			return e
		}
	}
	return nil
}
func (s *Server) registerPluginRoutes() {
	for _, r := range []struct {
		method, path, role, scope string
		handler                   endpoint
	}{
		{"GET", "/plugins", "viewer", "plugins:read", s.listPlugins}, {"GET", "/plugins/{id}", "viewer", "plugins:read", s.getPlugin},
		{"POST", "/plugins/install", "admin", "plugins:write", s.installPlugin}, {"POST", "/plugins/{id}/update", "admin", "plugins:write", s.updatePlugin},
		{"GET", "/plugins/{id}/releases", "admin", "plugins:read", s.listReleases}, {"POST", "/plugins/{id}/activate", "admin", "plugins:write", s.activatePlugin},
		{"PUT", "/plugins/{id}/state", "admin", "plugins:write", s.pluginState}, {"GET", "/plugins/{id}/settings", "admin", "plugins:read", s.getSettings}, {"PUT", "/plugins/{id}/settings", "admin", "plugins:write", s.putSettings},
		{"GET", "/menus", "viewer", "plugins:read", s.listMenus}, {"PUT", "/menus/{plugin_id}/{menu_id}", "admin", "plugins:write", s.putMenu},
	} {
		s.route(r.method+" /api/v1"+r.path, r.role, r.scope, r.handler)
	}
	for _, method := range []string{"GET", "POST", "PUT", "DELETE"} {
		role, scope := "operator", "plugins:write"
		if method == "GET" {
			role, scope = "viewer", "plugins:read"
		}
		s.route(method+" /api/v1/plugins/{id}/resources/{resource}", role, scope, s.resource)
	}
}
func (s *Server) getPlugin(w http.ResponseWriter, r *http.Request, p principal) {
	plugin, e := s.plugin(r.PathValue("id"))
	if e != nil {
		pluginError(w, e)
		return
	}
	writeJSON(w, 200, plugin)
}
func pluginError(w http.ResponseWriter, e error) {
	if errors.Is(e, sql.ErrNoRows) {
		apiError(w, 404, "not_found", "Plugin or release not found")
	} else {
		internalError(w)
	}
}
func (s *Server) listPlugins(w http.ResponseWriter, r *http.Request, p principal) {
	limit, offset, ok := pagination(w, r)
	if !ok {
		return
	}
	rows, e := s.db.Query("SELECT id FROM plugins ORDER BY id")
	if e != nil {
		internalError(w)
		return
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			break
		}
		ids = append(ids, id)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		internalError(w)
		return
	}
	items := []Plugin{}
	for i, id := range ids {
		if i < offset || len(items) >= limit {
			continue
		}
		plugin, e := s.plugin(id)
		if e != nil {
			internalError(w)
			return
		}
		items = append(items, plugin)
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": len(ids)})
}
func (s *Server) installPlugin(w http.ResponseWriter, r *http.Request, p principal) {
	var body struct {
		RepositoryURL string `json:"repository_url"`
		Ref           string `json:"ref"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	plugin, e := s.install(r.Context(), body.RepositoryURL, body.Ref, "")
	if e != nil {
		apiError(w, 400, "install_failed", e.Error())
		return
	}
	s.audit(r, "plugin.install", p.User.ID)
	writeJSON(w, 201, plugin)
}
func (s *Server) updatePlugin(w http.ResponseWriter, r *http.Request, p principal) {
	var body struct {
		Ref string `json:"ref"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	old, e := s.plugin(r.PathValue("id"))
	if e != nil {
		pluginError(w, e)
		return
	}
	plugin, e := s.install(r.Context(), old.RepositoryURL, body.Ref, old.ID)
	if e != nil {
		apiError(w, 400, "install_failed", e.Error())
		return
	}
	s.audit(r, "plugin.update", p.User.ID)
	writeJSON(w, 200, plugin)
}
func (s *Server) listReleases(w http.ResponseWriter, r *http.Request, p principal) {
	if _, e := s.plugin(r.PathValue("id")); e != nil {
		pluginError(w, e)
		return
	}
	limit, offset, ok := pagination(w, r)
	if !ok {
		return
	}
	var total int
	if e := s.db.QueryRow("SELECT count(*) FROM releases WHERE plugin_id=?", r.PathValue("id")).Scan(&total); e != nil {
		internalError(w)
		return
	}
	rows, e := s.db.Query("SELECT id,version,commit_sha,data_schema_version,created_at FROM releases WHERE plugin_id=? ORDER BY created_at DESC,id LIMIT ? OFFSET ?", r.PathValue("id"), limit, offset)
	if e != nil {
		internalError(w)
		return
	}
	defer rows.Close()
	items := []Release{}
	for rows.Next() {
		var release Release
		if e = rows.Scan(&release.ID, &release.Version, &release.CommitSHA, &release.DataSchemaVersion, &release.CreatedAt); e != nil {
			internalError(w)
			return
		}
		items = append(items, release)
	}
	if rows.Err() != nil {
		internalError(w)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": total})
}
func (s *Server) activatePlugin(w http.ResponseWriter, r *http.Request, p principal) {
	var body struct {
		ReleaseID string `json:"release_id"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	select {
	case installGate <- struct{}{}:
		defer func() { <-installGate }()
	default:
		apiError(w, 409, "install_busy", "Installation in progress")
		return
	}
	old, e := s.plugin(r.PathValue("id"))
	if e != nil {
		pluginError(w, e)
		return
	}
	manifest, _, e := s.release(body.ReleaseID)
	if e != nil {
		pluginError(w, e)
		return
	}
	if manifest.ID != old.ID || manifest.DataSchemaVersion != old.Manifest.DataSchemaVersion {
		apiError(w, 409, "schema_mismatch", "Release data schema is incompatible")
		return
	}
	settings, e := s.settings(old.ID)
	if e != nil {
		internalError(w)
		return
	}
	if validateInput(manifest.SettingsSchema, settings) != nil {
		apiError(w, 409, "settings_mismatch", "Stored settings incompatible with release")
		return
	}
	tx, e := s.db.Begin()
	if e != nil {
		internalError(w)
		return
	}
	defer tx.Rollback()
	if _, e = tx.Exec("UPDATE plugins SET active_release_id=? WHERE id=?", body.ReleaseID, old.ID); e == nil {
		e = syncMenus(tx, manifest)
	}
	if e == nil {
		e = tx.Commit()
	}
	if e != nil {
		internalError(w)
		return
	}
	s.notify()
	s.audit(r, "plugin.activate", p.User.ID)
	plugin, _ := s.plugin(old.ID)
	writeJSON(w, 200, plugin)
}
func (s *Server) pluginState(w http.ResponseWriter, r *http.Request, p principal) {
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Enabled == nil {
		apiError(w, 400, "invalid_input", "enabled is required")
		return
	}
	plugin, e := s.plugin(r.PathValue("id"))
	if e != nil {
		pluginError(w, e)
		return
	}
	state := "disabled"
	if *body.Enabled {
		state = "active"
	}
	tx, e := s.db.Begin()
	if e != nil {
		internalError(w)
		return
	}
	defer tx.Rollback()
	if _, e = tx.Exec("UPDATE plugins SET state=? WHERE id=?", state, plugin.ID); e == nil && !*body.Enabled {
		_, e = tx.Exec("UPDATE runs SET state='cancelled',finished_at=?,error='Plugin disabled' WHERE plugin_id=? AND state='queued'", nowString(), plugin.ID)
	}
	if e == nil {
		e = tx.Commit()
	}
	if e != nil {
		internalError(w)
		return
	}
	plugin.State = state
	s.notify()
	s.audit(r, "plugin.state", p.User.ID)
	writeJSON(w, 200, plugin)
}
func secretSchema(schema map[string]any) bool {
	return schema["writeOnly"] == true || schema["format"] == "password"
}
func transformSecrets(schema map[string]any, value, old any, mask bool) any {
	return transformSecretNodes([]map[string]any{schema}, schema, value, old, mask)
}
func localSchemaRef(root map[string]any, ref string) map[string]any {
	u, e := url.Parse(ref)
	if e != nil {
		return nil
	}
	fragment := u.Fragment
	if fragment == "" {
		return root
	}
	if strings.HasPrefix(fragment, "/") {
		var node any = root
		for _, part := range strings.Split(strings.TrimPrefix(fragment, "/"), "/") {
			key := strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
			switch value := node.(type) {
			case map[string]any:
				node = value[key]
			case []any:
				index, e := strconv.Atoi(key)
				if e != nil || index < 0 || index >= len(value) || strconv.Itoa(index) != key {
					return nil
				}
				node = value[index]
			default:
				return nil
			}
		}
		m, _ := node.(map[string]any)
		return m
	}
	return nil
}
func secretBranches(nodes []map[string]any, root map[string]any) []map[string]any {
	branches := []map[string]any{}
	seenRefs := map[string]bool{}
	var visit func(map[string]any)
	visit = func(node map[string]any) {
		if node == nil {
			return
		}
		branches = append(branches, node)
		if ref, ok := node["$ref"].(string); ok && !seenRefs[ref] {
			seenRefs[ref] = true
			visit(localSchemaRef(root, ref))
		}
		for _, key := range []string{"allOf", "anyOf", "oneOf"} {
			if list, ok := node[key].([]any); ok {
				for _, child := range list {
					m, _ := child.(map[string]any)
					visit(m)
				}
			}
		}
		for _, key := range []string{"if", "then", "else"} {
			m, _ := node[key].(map[string]any)
			visit(m)
		}
		if dependencies, ok := node["dependentSchemas"].(map[string]any); ok {
			for _, child := range dependencies {
				m, _ := child.(map[string]any)
				visit(m)
			}
		}
	}
	for _, node := range nodes {
		visit(node)
	}
	return branches
}
func transformSecretNodes(nodes []map[string]any, root map[string]any, value, old any, mask bool) any {
	branches := secretBranches(nodes, root)
	if slices.ContainsFunc(branches, secretSchema) {
		if mask {
			return secretMask
		}
		if value == "" || value == secretMask {
			return old
		}
		return value
	}
	if object, ok := value.(map[string]any); ok {
		previous, _ := old.(map[string]any)
		for key, field := range object {
			children := []map[string]any{}
			for _, branch := range branches {
				properties, _ := branch["properties"].(map[string]any)
				if sub, ok := properties[key].(map[string]any); ok {
					children = append(children, sub)
				}
				patterns, _ := branch["patternProperties"].(map[string]any)
				matched := false
				for pattern, raw := range patterns {
					re, e := regexp.Compile(pattern)
					if e == nil && re.MatchString(key) {
						matched = true
						if sub, ok := raw.(map[string]any); ok {
							children = append(children, sub)
						}
					}
				}
				if properties[key] == nil && !matched {
					if sub, ok := branch["additionalProperties"].(map[string]any); ok {
						children = append(children, sub)
					}
				}
				if sub, ok := branch["unevaluatedProperties"].(map[string]any); ok {
					children = append(children, sub)
				}
			}
			object[key] = transformSecretNodes(children, root, field, previous[key], mask)
		}
	}
	if array, ok := value.([]any); ok {
		previous, _ := old.([]any)
		for i, field := range array {
			children := []map[string]any{}
			for _, branch := range branches {
				if sub, ok := branch["items"].(map[string]any); ok {
					children = append(children, sub)
				}
				if prefix, ok := branch["prefixItems"].([]any); ok && i < len(prefix) {
					if sub, ok := prefix[i].(map[string]any); ok {
						children = append(children, sub)
					}
				}
				if sub, ok := branch["contains"].(map[string]any); ok {
					children = append(children, sub)
				}
				if sub, ok := branch["unevaluatedItems"].(map[string]any); ok {
					children = append(children, sub)
				}
			}
			var before any
			if i < len(previous) {
				before = previous[i]
			}
			array[i] = transformSecretNodes(children, root, field, before, mask)
		}
	}
	return value
}
func (s *Server) getSettings(w http.ResponseWriter, r *http.Request, p principal) {
	plugin, e := s.plugin(r.PathValue("id"))
	if e != nil {
		pluginError(w, e)
		return
	}
	settings, e := s.settings(plugin.ID)
	if e != nil {
		internalError(w)
		return
	}
	writeJSON(w, 200, transformSecrets(plugin.Manifest.SettingsSchema, settings, nil, true))
}
func (s *Server) putSettings(w http.ResponseWriter, r *http.Request, p principal) {
	var settings map[string]any
	if !decodeJSON(w, r, &settings) {
		return
	}
	if settings == nil {
		apiError(w, 400, "invalid_input", "Settings must be an object")
		return
	}
	select {
	case installGate <- struct{}{}:
		defer func() { <-installGate }()
	default:
		apiError(w, 409, "install_busy", "Installation in progress")
		return
	}
	plugin, e := s.plugin(r.PathValue("id"))
	if e != nil {
		pluginError(w, e)
		return
	}
	old, e := s.settings(plugin.ID)
	if e != nil {
		internalError(w)
		return
	}
	transformSecrets(plugin.Manifest.SettingsSchema, settings, old, false)
	if e = validateInput(plugin.Manifest.SettingsSchema, settings); e != nil {
		apiError(w, 400, "invalid_input", "Settings do not match schema")
		return
	}
	data, _ := json.Marshal(settings)
	if _, e = s.db.Exec("UPDATE plugins SET settings=? WHERE id=?", string(data), plugin.ID); e != nil {
		internalError(w)
		return
	}
	s.notify()
	s.audit(r, "plugin.settings", p.User.ID)
	writeJSON(w, 200, transformSecrets(plugin.Manifest.SettingsSchema, settings, nil, true))
}
func (s *Server) listMenus(w http.ResponseWriter, r *http.Request, p principal) {
	limit, offset, ok := pagination(w, r)
	if !ok {
		return
	}
	rows, e := s.db.Query("SELECT m.plugin_id,m.id,m.title,m.icon,m.position,m.enabled,p.state,r.manifest FROM menus m JOIN plugins p ON p.id=m.plugin_id JOIN releases r ON r.id=p.active_release_id ORDER BY m.position,m.plugin_id,m.id")
	if e != nil {
		internalError(w)
		return
	}
	defer rows.Close()
	items := []Menu{}
	total := 0
	for rows.Next() {
		var menu Menu
		var state, raw string
		if e = rows.Scan(&menu.PluginID, &menu.ID, &menu.Title, &menu.Icon, &menu.Position, &menu.Enabled, &state, &raw); e != nil {
			internalError(w)
			return
		}
		var m Manifest
		if json.Unmarshal([]byte(raw), &m) != nil {
			internalError(w)
			return
		}
		if !slices.ContainsFunc(m.Menus, func(d MenuDeclaration) bool { return d.ID == menu.ID }) {
			continue
		}
		menu.PluginName = m.Name
		menu.Enabled = menu.Enabled && state == "active"
		if p.User.Role != "admin" && !menu.Enabled {
			continue
		}
		if total >= offset && len(items) < limit {
			items = append(items, menu)
		}
		total++
	}
	if rows.Err() != nil {
		internalError(w)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": total})
}
func (s *Server) putMenu(w http.ResponseWriter, r *http.Request, p principal) {
	var body struct {
		Title    string `json:"title"`
		Position *int   `json:"position"`
		Enabled  *bool  `json:"enabled"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Title) == "" || len(body.Title) > 100 || body.Position == nil || *body.Position < 0 || *body.Position > 10000 || body.Enabled == nil {
		apiError(w, 400, "invalid_input", "Invalid menu fields")
		return
	}
	plugin, e := s.plugin(r.PathValue("plugin_id"))
	if e != nil {
		pluginError(w, e)
		return
	}
	var declaration *MenuDeclaration
	for _, m := range plugin.Manifest.Menus {
		if m.ID == r.PathValue("menu_id") {
			copy := m
			declaration = &copy
			break
		}
	}
	if declaration == nil {
		apiError(w, 404, "not_found", "Menu not declared")
		return
	}
	if _, e = s.db.Exec("UPDATE menus SET title=?,position=?,enabled=? WHERE plugin_id=? AND id=?", body.Title, *body.Position, *body.Enabled, plugin.ID, declaration.ID); e != nil {
		internalError(w)
		return
	}
	s.notify()
	writeJSON(w, 200, Menu{PluginID: plugin.ID, PluginName: plugin.Name, ID: declaration.ID, Title: body.Title, Icon: declaration.Icon, Position: *body.Position, Enabled: *body.Enabled && plugin.State == "active"})
}
func (s *Server) resource(w http.ResponseWriter, r *http.Request, p principal) {
	plugin, e := s.plugin(r.PathValue("id"))
	if e != nil {
		pluginError(w, e)
		return
	}
	if plugin.State != "active" {
		apiError(w, 409, "plugin_disabled", "Plugin is disabled")
		return
	}
	var resource *Resource
	for _, d := range plugin.Manifest.Resources {
		if d.ID == r.PathValue("resource") {
			copy := d
			resource = &copy
			break
		}
	}
	if resource == nil {
		apiError(w, 404, "not_found", "Resource not declared")
		return
	}
	if !slices.Contains(resource.Methods, r.Method) {
		apiError(w, 405, "method_not_allowed", "Resource method not declared")
		return
	}
	input := map[string]any{}
	if r.Method == "GET" {
		for key, values := range r.URL.Query() {
			if len(values) != 1 {
				apiError(w, 400, "invalid_input", "Query fields must be singular")
				return
			}
			input[key] = values[0]
		}
	} else if !decodeJSON(w, r, &input) {
		return
	}
	if validateInput(resource.InputSchema, input) != nil {
		apiError(w, 400, "invalid_input", "Resource input does not match schema")
		return
	}
	select {
	case s.resourceSlots <- struct{}{}:
		defer func() { <-s.resourceSlots }()
	default:
		apiError(w, 429, "resource_busy", "Too many resource calls")
		return
	}
	settings, e := s.settings(plugin.ID)
	if e != nil {
		internalError(w)
		return
	}
	_, executable, e := s.release(plugin.ActiveReleaseID)
	if e != nil {
		internalError(w)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(resource.TimeoutSeconds)*time.Second)
	defer cancel()
	result, e := runPlugin(ctx, executable, pluginRequest{ProtocolVersion: 1, Operation: "resource", Resource: resource.ID, Method: r.Method, Input: input, Settings: settings, DataDir: filepath.Join(s.dataDir, "plugin-data", plugin.ID)}, nil, nil)
	if e != nil {
		apiError(w, 502, "plugin_failed", "Resource process failed")
		return
	}
	writeJSON(w, 200, result)
}

const timestampFormat = "2006-01-02T15:04:05.000000000Z07:00"

func nowString() string { return time.Now().UTC().Format(timestampFormat) }
