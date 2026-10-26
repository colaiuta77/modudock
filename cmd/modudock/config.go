// YAML과 명시적 CLI 옵션을 병합하고 파일 생성 없이 실행 설정을 검증한다.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

type serverConfig struct {
	DataDir   string
	Listen    string
	PublicURL string
	LogFile   string
}

func configFlags(f *flag.FlagSet) (*serverConfig, *string) {
	c := &serverConfig{}
	path := f.String("config", "", "YAML configuration file")
	f.StringVar(&c.DataDir, "data-dir", "data", "Local data directory")
	f.StringVar(&c.Listen, "listen", "127.0.0.1:7438", "HTTP listen address")
	f.StringVar(&c.PublicURL, "public-url", "", "External HTTP origin")
	f.StringVar(&c.LogFile, "log-file", "", "Optional operational log (requires external rotation)")
	return c, path
}

func resolveConfig(f *flag.FlagSet, c *serverConfig, path string) error {
	if path != "" {
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("config must be a regular file")
		}
		if info.Size() > 1<<20 {
			return errors.New("config exceeds 1 MiB")
		}
		d := yaml.NewDecoder(io.LimitReader(file, 1<<20))
		var doc yaml.Node
		if err = d.Decode(&doc); err != nil {
			return fmt.Errorf("config: %w", err)
		}
		if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
			return errors.New("config must be a YAML mapping")
		}
		loaded := serverConfig{DataDir: "data", Listen: "127.0.0.1:7438"}
		fields := map[string]*string{"data_dir": &loaded.DataDir, "listen": &loaded.Listen, "public_url": &loaded.PublicURL, "log_file": &loaded.LogFile}
		seen := map[string]bool{}
		nodes := doc.Content[0].Content
		for i := 0; i < len(nodes); i += 2 {
			key, value := nodes[i], nodes[i+1]
			target, ok := fields[key.Value]
			if !ok || key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
				return fmt.Errorf("unknown config field %q", key.Value)
			}
			if seen[key.Value] {
				return fmt.Errorf("duplicate config field %q", key.Value)
			}
			if value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
				return fmt.Errorf("config field %s must be a string", key.Value)
			}
			seen[key.Value] = true
			*target = value.Value
		}
		var extra yaml.Node
		if err = d.Decode(&extra); err != io.EOF {
			return errors.New("config must contain exactly one YAML document")
		}
		f.Visit(func(v *flag.Flag) {
			switch v.Name {
			case "data-dir":
				loaded.DataDir = c.DataDir
			case "listen":
				loaded.Listen = c.Listen
			case "public-url":
				loaded.PublicURL = c.PublicURL
			case "log-file":
				loaded.LogFile = c.LogFile
			}
		})
		*c = loaded
	}
	if c.DataDir == "" || strings.ContainsAny(c.DataDir+c.LogFile, "\x00\r\n") {
		return errors.New("invalid data-dir or log-file")
	}
	host, port, err := net.SplitHostPort(c.Listen)
	if err != nil || !validHost(host, true) || !validPort(port) {
		return errors.New("listen must be a valid host:port address")
	}
	if c.PublicURL != "" {
		u, err := url.Parse(c.PublicURL)
		// 기존 CLI는 루트 슬래시를 허용했으므로 명시적 플래그만 호환 정규화한다.
		f.Visit(func(v *flag.Flag) {
			if v.Name == "public-url" && err == nil && u.Path == "/" {
				c.PublicURL = strings.TrimSuffix(c.PublicURL, "/")
				u.Path = ""
			}
		})
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || !validHost(u.Hostname(), false) || u.User != nil || u.Path != "" || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" || strings.Contains(c.PublicURL, "#") || (u.Port() != "" && !validPort(u.Port())) || strings.HasSuffix(u.Host, ":") {
			return errors.New("public-url must be an exact HTTP(S) origin without a path, credentials, query or fragment")
		}
	}
	return nil
}

func validHost(host string, empty bool) bool {
	if host == "" {
		return empty
	}
	if net.ParseIP(host) != nil {
		return true
	}
	if len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}

func validPort(port string) bool {
	for _, c := range port {
		if c < '0' || c > '9' {
			return false
		}
	}
	value, err := strconv.Atoi(port)
	return err == nil && value > 0 && value <= 65535
}

func (c serverConfig) healthURL() string {
	host, port, _ := net.SplitHostPort(c.Listen)
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	if host == "::" {
		host = "::1"
	}
	return "http://" + net.JoinHostPort(host, port)
}
