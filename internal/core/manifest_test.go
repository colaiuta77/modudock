// 플러그인 선언과 아카이브의 신뢰 경계를 검증한다.
package core

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func manifestFixture() []byte {
	return []byte(`{"schema_version":1,"protocol_version":1,"core_api":1,"id":"hello","name":"Hello","version":"1.0.0","data_schema_version":1,"artifacts":{"linux-amd64":{"url":"https://github.com/example/hello/releases/download/v1/hello.zip","sha256":"` + strings.Repeat("a", 64) + `","executable":"hello"}},"tasks":[{"id":"greet","title":"Greet","input_schema":{"type":"object","properties":{"name":{"type":"string"}},"additionalProperties":false},"timeout_seconds":10,"idempotent":true}],"settings_schema":{"type":"object","properties":{"token":{"type":"string","format":"password"}},"additionalProperties":false}}`)
}
func TestManifestRejectsUnsafeDeclarations(t *testing.T) {
	for _, change := range []func(map[string]any){
		func(m map[string]any) { m["id"] = "../escape" },
		func(m map[string]any) { m["runtime"] = "python" },
		func(m map[string]any) { m["permissions"] = []string{"host:root"} },
		func(m map[string]any) { m["schema_version"] = 2 },
		func(m map[string]any) { m["settings_schema"] = map[string]any{"$ref": "http://127.0.0.1/private"} },
		func(m map[string]any) { m["settings_schema"] = map[string]any{"type": "nonsense"} },
		func(m map[string]any) { m["tasks"].([]any)[0].(map[string]any)["timeout_seconds"] = 0 },
	} {
		var m map[string]any
		json.Unmarshal(manifestFixture(), &m)
		change(m)
		b, _ := json.Marshal(m)
		if _, e := parseManifest(b); e == nil {
			t.Fatalf("accepted unsafe manifest %s", b)
		}
	}
	m, e := parseManifest(manifestFixture())
	if e != nil {
		t.Fatal(e)
	}
	if e = validateInput(m.Tasks[0].InputSchema, map[string]any{"name": 42}); e == nil {
		t.Fatal("schema did not reject wrong type")
	}
	if e = validateInput(m.Tasks[0].InputSchema, map[string]any{"name": "Ada"}); e != nil {
		t.Fatal(e)
	}
}
func TestSchemaRejectsUnsupportedDialectsAndReferences(t *testing.T) {
	for _, tc := range []struct{ name, schema string }{
		{"draft07_tuple", `{"type":"object","$schema":"http://json-schema.org/draft-07/schema#","properties":{"tokens":{"type":"array","items":[{"type":"string","writeOnly":true}]}}}`},
		{"named_anchor", `{"type":"object","$defs":{"token":{"$anchor":"token","type":"string","writeOnly":true}},"default":{"$anchor":"token","type":"string"},"properties":{"token":{"$ref":"#token"}}}`},
		{"encoded_named_anchor", `{"type":"object","$defs":{"token":{"$anchor":"token","type":"string","writeOnly":true}},"properties":{"token":{"$ref":"#%74oken"}}}`},
		{"dynamic_pointer", `{"type":"object","$defs":{"token":{"type":"string","writeOnly":true}},"properties":{"token":{"$dynamicRef":"#/$defs/token"}}}`},
		{"recursive_pointer", `{"type":"object","$defs":{"token":{"type":"string","writeOnly":true}},"properties":{"token":{"$recursiveRef":"#/$defs/token"}}}`},
		{"nested_dialect", `{"type":"object","$defs":{"token":{"$schema":"http://json-schema.org/draft-07/schema#","type":"string","writeOnly":true}},"properties":{"token":{"$ref":"#/$defs/token"}}}`},
		{"annotation_dialect_pointer", `{"type":"object","default":{"$schema":"http://json-schema.org/draft-07/schema#","type":"string","writeOnly":true},"properties":{"token":{"$ref":"#/%64efault"}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var schema map[string]any
			if e := json.Unmarshal([]byte(tc.schema), &schema); e != nil {
				t.Fatal(e)
			}
			if _, e := compileSchema(schema); e == nil {
				t.Fatal("accepted unsupported schema dialect or reference")
			}
		})
	}
}
func zipFixture(t *testing.T, name string, mode os.FileMode, content []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	h := &zip.FileHeader{Name: name, Method: zip.Deflate}
	h.SetMode(mode)
	w, e := z.CreateHeader(h)
	if e != nil {
		t.Fatal(e)
	}
	w.Write(content)
	if e = z.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestArchiveRejectsTraversalLinksAndBomb(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mode    os.FileMode
		content []byte
	}{
		{"../outside", 0600, []byte("bad")}, {"/absolute", 0600, nil}, {"C:/absolute", 0600, nil}, {"dir\\escape", 0600, nil}, {"link", os.ModeSymlink | 0777, []byte("../outside")}, {"big", 0600, bytes.Repeat([]byte("a"), int(maxExpandedBytes)+1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if e := extractArchive(zipFixture(t, tc.name, tc.mode, tc.content), t.TempDir()); e == nil {
				t.Fatal("unsafe archive accepted")
			}
		})
	}
	dir := t.TempDir()
	if e := extractArchive(zipFixture(t, "bin/hello", 0755, []byte("binary")), dir); e != nil {
		t.Fatal(e)
	}
	if b, e := os.ReadFile(filepath.Join(dir, "bin", "hello")); e != nil || string(b) != "binary" {
		t.Fatalf("bad extraction %q %v", b, e)
	}
}
