// 실제 자식 프로세스로 프로토콜 검증과 취소를 검사한다.
package core

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func init() {
	if len(os.Args) > 0 && strings.Contains(filepath.Base(os.Args[0]), "plugin-helper") {
		TestRunnerProcessHelper(nil)
	}
}
func TestRunnerProcessHelper(t *testing.T) {
	if len(os.Args) < 1 || !strings.Contains(filepath.Base(os.Args[0]), "plugin-helper") {
		return
	}
	var request pluginRequest
	if e := json.NewDecoder(bufio.NewReader(os.Stdin)).Decode(&request); e != nil {
		os.Exit(2)
	}
	switch request.Input["mode"] {
	case "hang":
		time.Sleep(time.Minute)
	case "group_parent":
		child := exec.Command(os.Args[0])
		b, _ := json.Marshal(pluginRequest{ProtocolVersion: 1, Operation: "task", Input: map[string]any{"mode": "group_child", "marker": request.Input["marker"]}})
		child.Stdin = bytes.NewReader(b)
		if e := child.Start(); e != nil {
			os.Exit(2)
		}
		fmt.Println(`{"type":"progress","progress":99}`)
		time.Sleep(time.Minute)
	case "group_child":
		time.Sleep(500 * time.Millisecond)
		marker, _ := request.Input["marker"].(string)
		if e := os.WriteFile(marker, []byte("child survived"), 0600); e != nil {
			os.Exit(2)
		}
	case "secret_log":
		fmt.Fprintln(os.Stderr, request.Settings["token"])
		fmt.Println(`{"type":"result","result":{"message":"logged"}}`)
	case "quoted_log":
		fmt.Fprint(os.Stderr, strings.Repeat("\"", 40000))
		fmt.Println(`{"type":"result","result":{}}`)
	case "bad":
		fmt.Println(`{"type":"unknown"}`)
	case "null_progress":
		fmt.Println("{\"type\":\"progress\",\"progress\":null}\n{\"type\":\"result\",\"result\":{}}")
	case "empty_result":
		fmt.Println(`{"type":"result","result":null}`)
	case "no_newline":
		fmt.Print(`{"type":"result","result":{}}`)
	case "duplicate":
		fmt.Println("{\"type\":\"result\",\"result\":{}}\n{\"type\":\"result\",\"result\":{}}")
	case "after":
		fmt.Println("{\"type\":\"result\",\"result\":{}}\n{\"type\":\"progress\",\"progress\":20}")
	case "huge":
		fmt.Println(strings.Repeat("x", int(maxProtocolBytes)+1))
	case "stderr":
		fmt.Fprint(os.Stderr, strings.Repeat("s", int(maxStderrBytes)+1))
		fmt.Println(`{"type":"result","result":{}}`)
	case "env":
		fmt.Printf("{\"type\":\"result\",\"result\":{\"secret\":%q}}\n", os.Getenv("MODUDOCK_TEST_SECRET"))
	case "fail":
		os.Exit(7)
	default:
		fmt.Println(`{"type":"progress","progress":30}`)
		if request.Operation == "health" {
			fmt.Println(`{"type":"result","result":{"healthy":true,"protocol_version":1}}`)
		} else {
			fmt.Println(`{"type":"result","result":{"message":"hello"}}`)
		}
	}
	os.Exit(0)
}
func helperExecutable(t *testing.T) string {
	t.Helper()
	source, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(source)
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), "plugin-helper")
	if strings.HasSuffix(source, ".exe") {
		path += ".exe"
	}
	if e = os.WriteFile(path, b, 0700); e != nil {
		t.Fatal(e)
	}
	return path
}
func TestRunnerProtocolAndCancellation(t *testing.T) {
	exe := helperExecutable(t)
	t.Setenv("MODUDOCK_TEST_SECRET", "must-not-inherit")
	for _, mode := range []string{"bad", "null_progress", "empty_result", "no_newline", "duplicate", "after", "huge", "stderr", "fail", "hang"} {
		t.Run(mode, func(t *testing.T) {
			timeout := 5 * time.Second
			if mode == "hang" {
				timeout = 300 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			start := time.Now()
			_, e := runPlugin(ctx, exe, pluginRequest{ProtocolVersion: 1, Operation: "task", Input: map[string]any{"mode": mode}}, nil, nil)
			if e == nil {
				t.Fatal("invalid process succeeded")
			}
			if mode != "hang" && errors.Is(e, context.DeadlineExceeded) {
				t.Fatalf("validation was not reached: %v", e)
			}
			if mode == "hang" && time.Since(start) > 3*time.Second {
				t.Fatal("child did not terminate")
			}
		})
	}
	result, e := runPlugin(context.Background(), exe, pluginRequest{ProtocolVersion: 1, Operation: "task", Input: map[string]any{"mode": "env"}}, nil, nil)
	if e != nil || result["secret"] != "" {
		t.Fatalf("environment leaked %v %v", result, e)
	}
	progress := 0
	result, e = runPlugin(context.Background(), exe, pluginRequest{ProtocolVersion: 1, Operation: "task", Input: map[string]any{}}, func(v int) { progress = v }, nil)
	if e != nil || result["message"] != "hello" || progress != 30 {
		t.Fatalf("bad protocol result %v %v progress=%d", result, e, progress)
	}
}
