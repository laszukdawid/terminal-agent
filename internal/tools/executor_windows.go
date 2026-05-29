//go:build windows

package tools

import "os/exec"

func configureCommandCancellation(_ *exec.Cmd) {}
