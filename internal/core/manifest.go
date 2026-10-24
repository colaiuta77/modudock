// 플러그인 manifest와 외부 참조 없는 JSON Schema를 검증한다.
package core

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type Manifest struct {
	SchemaVersion     int                 `json:"schema_version"`
	ProtocolVersion   int                 `json:"protocol_version"`
	CoreAPI           int                 `json:"core_api"`
	ID                string              `json:"id"`
	Name              string              `json:"name"`
	Version           string              `json:"version"`
	DataSchemaVersion int                 `json:"data_schema_version"`
	Artifacts         map[string]Artifact `json:"artifacts"`
	Tasks             []Task              `json:"tasks,omitempty"`
	Resources         []Resource          `json:"resources,omitempty"`
	Menus             []MenuDeclaration   `json:"menus,omitempty"`
	SettingsSchema    map[string]any      `json:"settings_schema,omitempty"`
}
type Artifact struct {
	URL        string `json:"url"`
	SHA256     string `json:"sha256"`
	Executable string `json:"executable"`
}
type Task struct {
	ID             string         `json:"id"`
	Title          string         `json:"title"`
	InputSchema    map[string]any `json:"input_schema"`
	TimeoutSeconds int            `json:"timeout_seconds"`
	Idempotent     bool           `json:"idempotent"`
}
type Resource struct {
	ID             string         `json:"id"`
	Methods        []string       `json:"methods"`
	InputSchema    map[string]any `json:"input_schema"`
	TimeoutSeconds int            `json:"timeout_seconds"`
}
type MenuDeclaration struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Icon     string `json:"icon"`
	Resource string `json:"resource,omitempty"`
}

var pluginIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)
var platformPattern = regexp.MustCompile(`^(linux|windows|darwin)-(amd64|arm64)$`)

func strictJSON(data []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return e
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("expected one JSON value")
	}
	return nil
}
func parseManifest(data []byte) (Manifest, error) {
	var m Manifest
	if len(data) > 64<<10 {
		return m, errors.New("manifest exceeds 64 KiB")
	}
	if e := strictJSON(data, &m); e != nil {
		return m, e
	}
	if m.SchemaVersion != 1 || m.ProtocolVersion != 1 || m.CoreAPI != 1 || m.DataSchemaVersion < 1 || !pluginIDPattern.MatchString(m.ID) || strings.TrimSpace(m.Name) == "" || len(m.Name) > 100 || len(m.Version) > 100 || !versionPattern.MatchString(m.Version) || len(m.Artifacts) == 0 || len(m.Artifacts) > 6 {
		return m, errors.New("invalid manifest metadata")
	}
	for platform, a := range m.Artifacts {
		digest, e := hex.DecodeString(a.SHA256)
		if !platformPattern.MatchString(platform) || e != nil || len(digest) != 32 || !safeArchivePath(a.Executable) || strings.HasSuffix(a.Executable, "/") {
			return m, errors.New("invalid artifact declaration")
		}
		if _, e = repositoryFromArtifact(a.URL); e != nil {
			return m, e
		}
	}
	if len(m.Tasks) > 100 || len(m.Resources) > 100 || len(m.Menus) > 100 {
		return m, errors.New("too many declarations")
	}
	seen := map[string]bool{}
	for _, task := range m.Tasks {
		if !pluginIDPattern.MatchString(task.ID) || seen[task.ID] || strings.TrimSpace(task.Title) == "" || len(task.Title) > 100 || task.TimeoutSeconds < 1 || task.TimeoutSeconds > 3600 {
			return m, errors.New("invalid task")
		}
		seen[task.ID] = true
		if _, e := compileSchema(task.InputSchema); e != nil {
			return m, fmt.Errorf("task schema: %w", e)
		}
	}
	seen = map[string]bool{}
	for _, resource := range m.Resources {
		if !pluginIDPattern.MatchString(resource.ID) || seen[resource.ID] || len(resource.Methods) == 0 || len(resource.Methods) > 4 || resource.TimeoutSeconds < 1 || resource.TimeoutSeconds > 60 {
			return m, errors.New("invalid resource")
		}
		seen[resource.ID] = true
		methods := map[string]bool{}
		for _, method := range resource.Methods {
			if methods[method] || (method != "GET" && method != "POST" && method != "PUT" && method != "DELETE") {
				return m, errors.New("invalid resource method")
			}
			methods[method] = true
		}
		if _, e := compileSchema(resource.InputSchema); e != nil {
			return m, fmt.Errorf("resource schema: %w", e)
		}
	}
	menus := map[string]bool{}
	for _, menu := range m.Menus {
		if !pluginIDPattern.MatchString(menu.ID) || menus[menu.ID] || strings.TrimSpace(menu.Title) == "" || len(menu.Title) > 100 || len(menu.Icon) > 64 || (menu.Resource != "" && !seen[menu.Resource]) {
			return m, errors.New("invalid menu")
		}
		menus[menu.ID] = true
	}
	if m.SettingsSchema != nil {
		if _, e := compileSchema(m.SettingsSchema); e != nil {
			return m, fmt.Errorf("settings schema: %w", e)
		}
	}
	return m, nil
}
func compileSchema(schema map[string]any) (*jsonschema.Schema, error) {
	if schema == nil || schema["type"] != "object" {
		return nil, errors.New("schema must declare type object")
	}
	var localRefs func(any) error
	localRefs = func(v any) error {
		switch x := v.(type) {
		case map[string]any:
			for key, value := range x {
				if key == "$dynamicRef" || key == "$recursiveRef" {
					return errors.New("dynamic and recursive schema refs are forbidden")
				}
				if key == "$ref" {
					s, ok := value.(string)
					if !ok || !strings.HasPrefix(s, "#") {
						return errors.New("external schema refs are forbidden")
					}
					u, e := url.Parse(s)
					if e != nil || (u.Fragment != "" && !strings.HasPrefix(u.Fragment, "/")) {
						return errors.New("schema refs must be local JSON Pointers")
					}
				}
				if key == "$id" {
					return errors.New("schema identifiers are forbidden")
				}
				if key == "$schema" && value != "https://json-schema.org/draft/2020-12/schema" && value != "https://json-schema.org/draft/2020-12/schema#" {
					return errors.New("only JSON Schema Draft 2020-12 is supported")
				}
				if e := localRefs(value); e != nil {
					return e
				}
			}
		case []any:
			for _, v := range x {
				if e := localRefs(v); e != nil {
					return e
				}
			}
		}
		return nil
	}
	if e := localRefs(schema); e != nil {
		return nil, e
	}
	c := jsonschema.NewCompiler()
	c.UseLoader(jsonschema.SchemeURLLoader{})
	c.DefaultDraft(jsonschema.Draft2020)
	if e := c.AddResource("https://modudock.invalid/schema", schema); e != nil {
		return nil, e
	}
	return c.Compile("https://modudock.invalid/schema")
}
func validateInput(schema map[string]any, input map[string]any) error {
	if input == nil {
		return errors.New("input must be an object")
	}
	if schema == nil {
		return nil
	}
	compiled, e := compileSchema(schema)
	if e != nil {
		return e
	}
	return compiled.Validate(input)
}
func safeArchivePath(name string) bool {
	if name == "" || len(name) > 240 || strings.ContainsAny(name, "\\:\x00") || strings.HasPrefix(name, "/") {
		return false
	}
	for _, part := range strings.Split(strings.TrimSuffix(name, "/"), "/") {
		if part == "" || part == "." || part == ".." || strings.TrimRight(part, ". ") != part {
			return false
		}
		base := strings.ToUpper(strings.Split(part, ".")[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || regexp.MustCompile(`^(COM|LPT)[1-9]$`).MatchString(base) {
			return false
		}
	}
	return path.Clean(strings.TrimSuffix(name, "/")) == strings.TrimSuffix(name, "/")
}
