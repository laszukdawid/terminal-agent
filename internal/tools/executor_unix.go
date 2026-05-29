//go:build !windows

package tools

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func configureCommandCancellation(cmd *exec.Cmd) {
	// Run bash in its own process group so context cancellation kills the shell
	// and child processes such as sleep, not just the immediate shell process.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}
