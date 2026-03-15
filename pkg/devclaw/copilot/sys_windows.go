//go:build windows

package copilot

import (
	"os"
	"os/exec"
)

func setSysProcAttr(cmd *exec.Cmd) {}

func killProcGroup(cmd *exec.Cmd) error {
	if cmd.Process != nil {
		return cmd.Process.Kill()
	}
	return nil
}

func killProcessGroupByCmd(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	cmd.Process.Kill()
}

func flockExclusive(f *os.File) error {
	return nil // advisory locking not supported on Windows
}

func flockUnlock(f *os.File) error {
	return nil
}

func processIsAlive(cmd *exec.Cmd) bool {
	// On Windows, signal 0 is not supported. Check ProcessState instead.
	return cmd.ProcessState == nil
}
