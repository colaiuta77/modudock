// JSONL 회전 보존과 로그 권한 및 민감정보 경계를 검증한다.
package core

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOperationalLogReceivesCoreEvents(t *testing.T) {
	var operational bytes.Buffer
	s, err := Open(t.TempDir(), Options{LogOutput: &operational})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.log(LogEntry{Message: "operational event", Component: "installer"})
	entries, err := s.logs.read()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Message != "operational event" || !strings.Contains(operational.String(), `"message":"operational event"`) {
		t.Fatalf("log not delivered: %q %+v", operational.String(), entries)
	}
}

func TestLogSeverityAndUTC(t *testing.T) {
	c := newClient(t)
	c.s.log(LogEntry{Level: "ERROR", Message: "task failed", Component: "runner", RunID: "run-1"})
	entries, e := c.s.logs.read()
	if e != nil {
		t.Fatal(e)
	}
	if len(entries) != 1 || entries[0].Level != "ERROR" || entries[0].RunID != "run-1" {
		t.Fatalf("incorrect entries %+v", entries)
	}
	parsed, e := time.Parse(time.RFC3339, entries[0].Timestamp)
	if e != nil {
		t.Fatal(e)
	}
	_, offset := parsed.Zone()
	if offset != 0 {
		t.Fatalf("non-UTC timestamp %s", entries[0].Timestamp)
	}
}
func TestLogRotationBounded(t *testing.T) {
	l, e := openLogs(t.TempDir(), nil)
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	line := []byte(strings.Repeat("x", int(logMaxBytes)-1) + "\n")
	for i := 0; i < 6; i++ {
		if _, e = l.Write(line); e != nil {
			t.Fatal(e)
		}
	}
	for _, name := range []string{"core.jsonl", "core.1.jsonl", "core.2.jsonl", "core.3.jsonl"} {
		info, e := os.Stat(filepath.Join(l.dir, name))
		if e != nil || info.Size() > logMaxBytes {
			t.Fatalf("unbounded log %s: %v", name, e)
		}
	}
	if _, e = os.Stat(filepath.Join(l.dir, "core.4.jsonl")); !os.IsNotExist(e) {
		t.Fatalf("too many backups: %v", e)
	}
}
