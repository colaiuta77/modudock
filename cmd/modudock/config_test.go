// YAML 설정의 엄격한 검증과 CLI 우선순위 및 부작용 없는 검사를 검증한다.
package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConfigCheckStrictAndNoDatabase(t *testing.T) {
	data := filepath.Join(t.TempDir(), "untouched")
	path := writeConfig(t, fmt.Sprintf("# native config\ndata_dir: %q\nlisten: '[::]:7439'\npublic_url: https://panel.example.test\n", filepath.ToSlash(data)))
	var out bytes.Buffer
	if err := run([]string{"config", "check", "--config", path, "--field", "data_dir"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != filepath.ToSlash(data) {
		t.Fatalf("unexpected data dir %q", out.String())
	}
	if _, err := os.Stat(data); !os.IsNotExist(err) {
		t.Fatalf("validation created data: %v", err)
	}
	for _, text := range []string{
		"unknown: value", "listen: 7438", "data_dir: null", "listen: []", "listen: '127.0.0.1:0'", "listen: '127.0.0.1:65536'", "listen: 'bad address:7438'",
		"listen: '127.0.0.1:7438'\nlisten: '0.0.0.0:7438'", "{}\n---\n{}", "public_url: https://example.test/path", "public_url: https://user@example.test", "public_url: https://example.test?", "public_url: https://example.test#", "public_url: https://example.test:99999", "public_url: https://example.test/", "- item", "",
	} {
		out.Reset()
		if err := run([]string{"config", "check", "--config", writeConfig(t, text)}, strings.NewReader(""), &out); err == nil {
			t.Errorf("accepted %q", text)
		}
	}
}

func TestConfigExplicitFlagsOverride(t *testing.T) {
	path := writeConfig(t, "data_dir: /native/data\nlisten: '0.0.0.0:7439'\npublic_url: https://panel.example.test\nlog_file: /native/logs/service.log\n")
	for _, tc := range []struct{ field, flag, value string }{
		{"data_dir", "data-dir", "override"}, {"listen", "listen", "127.0.0.1:7440"}, {"public_url", "public-url", ""}, {"log_file", "log-file", ""},
	} {
		var out bytes.Buffer
		if err := run([]string{"config", "check", "--config", path, "--" + tc.flag, tc.value, "--field", tc.field}, strings.NewReader(""), &out); err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(out.String()) != tc.value {
			t.Fatalf("%s override: %q", tc.field, out.String())
		}
	}
}

func TestConfigPreservesLegacyCLIOriginSlash(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"config", "check", "--public-url", "https://panel.example.test/", "--field", "public_url"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "https://panel.example.test" {
		t.Fatalf("origin not normalized: %q", out.String())
	}
}

func TestHealthcheckUsesConfigListenerNotPublicURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			t.Errorf("path %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"status":"ok"}`)
	}))
	defer server.Close()
	listen := strings.TrimPrefix(server.URL, "http://")
	path := writeConfig(t, "listen: '0.0.0.0:"+strings.Split(listen, ":")[1]+"'\npublic_url: https://not-used.invalid\n")
	var out bytes.Buffer
	if err := run([]string{"healthcheck", "--config", path}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	bad := writeConfig(t, "listen: '127.0.0.1:1'")
	if err := run([]string{"healthcheck", "--config", bad, "--url", server.URL}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
}

func TestServeInvalidConfigDoesNotCreateDatabase(t *testing.T) {
	data := filepath.Join(t.TempDir(), "untouched")
	path := writeConfig(t, "listen: 'bad address'")
	var out bytes.Buffer
	if err := run([]string{"serve", "--config", path, "--data-dir", data}, strings.NewReader(""), &out); err == nil {
		t.Fatal("invalid config accepted")
	}
	if _, err := os.Stat(data); !os.IsNotExist(err) {
		t.Fatalf("invalid config created database: %v", err)
	}
}
