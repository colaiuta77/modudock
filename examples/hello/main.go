// ModuDock JSON 프로토콜로 인사 작업과 읽기 전용 리소스를 제공한다.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

type request struct {
	ProtocolVersion int            `json:"protocol_version"`
	Operation       string         `json:"operation"`
	TaskID          string         `json:"task_id"`
	Resource        string         `json:"resource"`
	Method          string         `json:"method"`
	Input           map[string]any `json:"input"`
	Settings        map[string]any `json:"settings"`
	DataDir         string         `json:"data_dir"`
	RunID           string         `json:"run_id"`
}

func main() {
	if e := run(os.Stdin, os.Stdout); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run(input io.Reader, output io.Writer) error {
	var r request
	decoder := json.NewDecoder(io.LimitReader(input, 128<<10))
	decoder.DisallowUnknownFields()
	if e := decoder.Decode(&r); e != nil {
		return e
	}
	if r.ProtocolVersion != 1 || r.Input == nil || r.Settings == nil {
		return errors.New("invalid protocol request")
	}
	encoder := json.NewEncoder(output)
	var result map[string]any
	switch r.Operation {
	case "health":
		result = map[string]any{"healthy": true, "protocol_version": 1}
	case "task":
		if r.TaskID != "greet" {
			return errors.New("unknown task")
		}
		name := "World"
		if value, ok := r.Input["name"]; ok {
			var valid bool
			name, valid = value.(string)
			if !valid || len(name) > 100 {
				return errors.New("invalid name")
			}
		}
		greeting := "Hello"
		if value, ok := r.Settings["greeting"].(string); ok && value != "" {
			greeting = value
		}
		if e := encoder.Encode(map[string]any{"type": "progress", "progress": 50}); e != nil {
			return e
		}
		result = map[string]any{"message": greeting + ", " + name + "!"}
	case "resource":
		if r.Resource != "greetings" || r.Method != "GET" {
			return errors.New("unknown resource method")
		}
		result = map[string]any{"items": []map[string]any{{"message": "Hello from ModuDock Go"}}, "total": 1}
	default:
		return errors.New("unknown operation")
	}
	return encoder.Encode(map[string]any{"type": "result", "result": result})
}
