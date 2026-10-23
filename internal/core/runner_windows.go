// Windows 개발 실행에서 직접 자식 프로세스를 취소한다.
package core

import "os/exec"

func configureProcess(cmd *exec.Cmd) { cmd.Cancel = func() error { return cmd.Process.Kill() } }
