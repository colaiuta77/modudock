// ModuDock 실행과 관리자 생성 및 상태 확인 명령을 제공한다.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"golang.org/x/term"
	"io"
	"modudock/internal/core"
	"modudock/internal/web"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata"
)

var version = "dev"

func main() {
	if e := run(os.Args[1:], os.Stdin, os.Stdout); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run(args []string, in io.Reader, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: modudock serve|admin create|admin reset-mfa|healthcheck|version")
	}
	switch args[0] {
	case "version":
		if len(args) != 1 {
			return errors.New("version takes no arguments")
		}
		fmt.Fprintln(out, version)
		return nil
	case "serve":
		f := flag.NewFlagSet("serve", flag.ContinueOnError)
		f.SetOutput(out)
		config, path := configFlags(f)
		if e := f.Parse(args[1:]); e != nil {
			return e
		}
		if f.NArg() != 0 {
			return errors.New("unexpected serve arguments")
		}
		if e := resolveConfig(f, config, *path); e != nil {
			return e
		}
		var logOutput io.Writer
		if config.LogFile != "" {
			file, e := os.OpenFile(config.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
			if e != nil {
				return e
			}
			defer file.Close()
			logOutput = file
		}
		s, e := core.Open(config.DataDir, core.Options{Version: version, PublicURL: config.PublicURL, LogOutput: logOutput, WebFS: web.FS()})
		if e != nil {
			return e
		}
		defer s.Close()
		if e = s.StartBackground(); e != nil {
			return e
		}
		server := &http.Server{Addr: config.Listen, Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		server.BaseContext = func(net.Listener) context.Context { return ctx }
		result := make(chan error, 1)
		go func() { result <- server.ListenAndServe() }()
		select {
		case e = <-result:
			if errors.Is(e, http.ErrServerClosed) {
				return nil
			}
			return e
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if e = server.Shutdown(shutdown); e != nil {
				server.Close()
				return e
			}
			return nil
		}
	case "admin":
		if len(args) > 1 && args[1] == "reset-mfa" {
			f := flag.NewFlagSet("admin reset-mfa", flag.ContinueOnError)
			f.SetOutput(out)
			dir := f.String("data-dir", "data", "Stopped server data directory")
			username := f.String("username", "", "Account username")
			confirmation := f.String("confirm-username", "", "Repeat the username to revoke all factors, sessions and API tokens")
			if e := f.Parse(args[2:]); e != nil {
				return e
			}
			if f.NArg() != 0 {
				return errors.New("unexpected reset-mfa arguments")
			}
			if *username == "" || *confirmation != *username {
				return errors.New("--confirm-username must exactly match --username")
			}
			s, e := core.Open(*dir, core.Options{Version: version})
			if e != nil {
				return e
			}
			defer s.Close()
			if e = s.ResetMFA(*username); e != nil {
				return e
			}
			fmt.Fprintln(out, "MFA, Passkeys, sessions and API tokens revoked. Password and role preserved.")
			return nil
		}
		if len(args) < 2 || args[1] != "create" {
			return errors.New("usage: modudock admin create|reset-mfa --username NAME")
		}
		f := flag.NewFlagSet("admin create", flag.ContinueOnError)
		f.SetOutput(out)
		dir := f.String("data-dir", "data", "Local data directory")
		username := f.String("username", "", "Administrator username")
		stdin := f.Bool("password-stdin", false, "Read password from stdin")
		if e := f.Parse(args[2:]); e != nil {
			return e
		}
		if f.NArg() != 0 {
			return errors.New("unexpected admin arguments")
		}
		if *username == "" {
			return errors.New("username required")
		}
		password, e := readPassword(in, out, *stdin)
		if e != nil {
			return e
		}
		s, e := core.Open(*dir, core.Options{Version: version})
		if e != nil {
			return e
		}
		defer s.Close()
		if e = s.CreateAdmin(*username, password); e != nil {
			return e
		}
		fmt.Fprintln(out, "Administrator created.")
		return nil
	case "config":
		if len(args) < 2 || args[1] != "check" {
			return errors.New("usage: modudock config check --config PATH [--field data_dir|listen|public_url|log_file]")
		}
		f := flag.NewFlagSet("config check", flag.ContinueOnError)
		f.SetOutput(out)
		config, path := configFlags(f)
		field := f.String("field", "", "Print one effective configuration field")
		if e := f.Parse(args[2:]); e != nil {
			return e
		}
		if f.NArg() != 0 {
			return errors.New("unexpected config arguments")
		}
		if e := resolveConfig(f, config, *path); e != nil {
			return e
		}
		values := map[string]string{"data_dir": config.DataDir, "listen": config.Listen, "public_url": config.PublicURL, "log_file": config.LogFile}
		if *field == "" {
			fmt.Fprintln(out, "Configuration valid.")
			return nil
		}
		value, ok := values[*field]
		if !ok {
			return errors.New("unknown config field")
		}
		fmt.Fprintln(out, value)
		return nil
	case "healthcheck":
		f := flag.NewFlagSet("healthcheck", flag.ContinueOnError)
		f.SetOutput(out)
		url := f.String("url", "http://127.0.0.1:7438", "Server origin")
		config, path := configFlags(f)
		if e := f.Parse(args[1:]); e != nil {
			return e
		}
		if f.NArg() != 0 {
			return errors.New("unexpected healthcheck arguments")
		}
		if e := resolveConfig(f, config, *path); e != nil {
			return e
		}
		explicitURL := false
		f.Visit(func(v *flag.Flag) {
			if v.Name == "url" {
				explicitURL = true
			}
		})
		if !explicitURL {
			*url = config.healthURL()
		}
		client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
		response, e := client.Get(strings.TrimRight(*url, "/") + "/healthz")
		if e != nil {
			return e
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			return fmt.Errorf("healthcheck returned HTTP %d", response.StatusCode)
		}
		var body struct {
			Status string `json:"status"`
		}
		d := json.NewDecoder(io.LimitReader(response.Body, 1024))
		d.DisallowUnknownFields()
		if e = d.Decode(&body); e != nil {
			return e
		}
		if body.Status != "ok" {
			return errors.New("server unhealthy")
		}
		var extra any
		if d.Decode(&extra) != io.EOF {
			return errors.New("invalid health response")
		}
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}
func readPassword(in io.Reader, out io.Writer, forceStdin bool) (string, error) {
	if file, ok := in.(*os.File); ok && !forceStdin && term.IsTerminal(int(file.Fd())) {
		fmt.Fprint(out, "Password: ")
		value, e := term.ReadPassword(int(file.Fd()))
		fmt.Fprintln(out)
		return string(value), e
	}
	reader := bufio.NewReader(io.LimitReader(in, 1026))
	line, e := reader.ReadString('\n')
	if e != nil && e != io.EOF {
		return "", e
	}
	line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	if len(line) > 1024 {
		return "", errors.New("password exceeds 1024 bytes")
	}
	return line, nil
}
