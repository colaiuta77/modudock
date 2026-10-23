// 예제 실행파일의 health·task·resource JSON 프로토콜을 검증한다.
package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestHelloProtocol(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{`{"protocol_version":1,"operation":"health","input":{},"settings":{},"data_dir":"unused"}`, `"healthy":true`},
		{`{"protocol_version":1,"operation":"task","task_id":"greet","input":{"name":"Ada"},"settings":{"greeting":"Welcome"},"data_dir":"unused"}`, `Welcome, Ada!`},
		{`{"protocol_version":1,"operation":"resource","resource":"greetings","method":"GET","input":{},"settings":{},"data_dir":"unused"}`, `"items"`},
	} {
		var output bytes.Buffer
		if e := run(strings.NewReader(tc.input+"\n"), &output); e != nil {
			t.Fatal(e)
		}
		if !strings.Contains(output.String(), tc.want) {
			t.Fatalf("bad output %s", &output)
		}
		for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
			var event map[string]any
			if e := json.Unmarshal([]byte(line), &event); e != nil {
				t.Fatal(e)
			}
		}
	}
	for _, input := range []string{`{"protocol_version":2,"operation":"health","input":{},"settings":{}}`, `{"protocol_version":1,"operation":"task","task_id":"missing","input":{},"settings":{}}`, `{"protocol_version":1,"operation":"task","task_id":"greet","input":{"name":42},"settings":{}}`} {
		if e := run(strings.NewReader(input), &bytes.Buffer{}); e == nil {
			t.Fatalf("invalid request accepted %s", input)
		}
	}
}
