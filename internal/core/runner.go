// 제한된 JSON 프로토콜과 최소 환경으로 신뢰한 플러그인을 실행한다.
package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const maxProtocolBytes = 1 << 20
const maxStderrBytes = 64 << 10

type pluginRequest struct {
	ProtocolVersion int            `json:"protocol_version"`
	Operation       string         `json:"operation"`
	TaskID          string         `json:"task_id,omitempty"`
	Resource        string         `json:"resource,omitempty"`
	Method          string         `json:"method,omitempty"`
	Input           map[string]any `json:"input"`
	Settings        map[string]any `json:"settings"`
	DataDir         string         `json:"data_dir"`
	RunID           string         `json:"run_id,omitempty"`
}
type protocolOutput struct {
	pending  []byte
	total    int
	result   map[string]any
	seen     bool
	err      error
	cancel   context.CancelFunc
	progress func(int)
}

func (o *protocolOutput) Write(b []byte) (int, error) {
	if o.err != nil {
		return 0, o.err
	}
	o.total += len(b)
	fail := func(e error) (int, error) { o.err = e; o.cancel(); return 0, e }
	if o.total > maxProtocolBytes {
		return fail(errors.New("stdout limit exceeded"))
	}
	o.pending = append(o.pending, b...)
	for {
		line, rest, ok := bytes.Cut(o.pending, []byte{'\n'})
		if !ok {
			break
		}
		o.pending = rest
		if o.seen {
			return fail(errors.New("output after final result"))
		}
		var event map[string]json.RawMessage
		if e := json.Unmarshal(line, &event); e != nil || event == nil {
			return fail(errors.New("malformed protocol event"))
		}
		var kind string
		if e := json.Unmarshal(event["type"], &kind); e != nil {
			return fail(errors.New("missing event type"))
		}
		switch kind {
		case "progress":
			var value float64
			if len(event) != 2 || bytes.Equal(event["progress"], []byte("null")) || json.Unmarshal(event["progress"], &value) != nil || value < 0 || value > 100 || value != float64(int(value)) {
				return fail(errors.New("invalid progress"))
			}
			if o.progress != nil {
				o.progress(int(value))
			}
		case "result":
			if len(event) != 2 || json.Unmarshal(event["result"], &o.result) != nil || o.result == nil {
				return fail(errors.New("result must be an object"))
			}
			o.seen = true
		default:
			return fail(errors.New("unknown protocol event"))
		}
	}
	return len(b), nil
}

type boundedStderr struct {
	data     []byte
	cancel   context.CancelFunc
	exceeded bool
}

func (b *boundedStderr) Write(p []byte) (int, error) {
	if len(b.data)+len(p) > maxStderrBytes {
		b.exceeded = true
		b.cancel()
		return 0, errors.New("stderr limit exceeded")
	}
	b.data = append(b.data, p...)
	return len(p), nil
}
func runPlugin(ctx context.Context, executable string, request pluginRequest, progress func(int), stderr func(string)) (map[string]any, error) {
	if !filepath.IsAbs(executable) {
		return nil, errors.New("executable must be absolute")
	}
	info, e := os.Lstat(executable)
	if e != nil {
		return nil, e
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("executable must be a regular file")
	}
	if request.ProtocolVersion != 1 || request.Input == nil {
		return nil, errors.New("invalid plugin request")
	}
	if request.Settings == nil {
		request.Settings = map[string]any{}
	}
	data, e := json.Marshal(request)
	if e != nil || len(data) > 128<<10 {
		return nil, errors.New("plugin request exceeds limit")
	}
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(child, executable)
	cmd.Dir = filepath.Dir(executable)
	cmd.Stdin = bytes.NewReader(append(data, '\n'))
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LANG=C.UTF-8", "TZ=UTC"}
	if runtime.GOOS == "windows" {
		root := os.Getenv("SystemRoot")
		cmd.Env = []string{"SystemRoot=" + root, "PATH=" + filepath.Join(root, "System32"), "TZ=UTC"}
	}
	output := &protocolOutput{cancel: cancel, progress: progress}
	errOutput := &boundedStderr{cancel: cancel}
	cmd.Stdout = output
	cmd.Stderr = errOutput
	cmd.WaitDelay = time.Second
	configureProcess(cmd)
	e = cmd.Run()
	// stderr may contain credentials; callers receive it only after applying settings redaction.
	if stderr != nil && len(errOutput.data) > 0 {
		stderr(strings.TrimSpace(string(errOutput.data)))
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if output.err != nil {
		return nil, output.err
	}
	if errOutput.exceeded {
		return nil, errors.New("stderr limit exceeded")
	}
	if e != nil {
		return nil, fmt.Errorf("plugin process failed: %w", e)
	}
	if len(output.pending) != 0 || !output.seen {
		return nil, errors.New("missing final newline or result")
	}
	return output.result, nil
}
