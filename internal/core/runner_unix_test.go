// Unix 취소가 손자 프로세스의 부수효과도 멈추는지 검증한다.
//go:build !windows

package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunnerCancellationKillsProcessGroup(t *testing.T) {
	exe := helperExecutable(t)
	marker := filepath.Join(t.TempDir(), "child-survived")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ready := false
	_, e := runPlugin(ctx, exe, pluginRequest{ProtocolVersion: 1, Operation: "task", Input: map[string]any{"mode": "group_parent", "marker": marker}}, func(progress int) {
		if progress == 99 {
			ready = true
			cancel()
		}
	}, nil)
	if !ready || !errors.Is(e, context.Canceled) {
		t.Fatalf("process never reached cancellation point ready=%v err=%v", ready, e)
	}
	time.Sleep(800 * time.Millisecond)
	if _, e = os.Stat(marker); !os.IsNotExist(e) {
		t.Fatalf("grandchild survived process-group cancellation %v", e)
	}
}
