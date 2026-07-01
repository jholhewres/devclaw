//go:build windows

package commands

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"syscall"

	"github.com/jholhewres/devclaw/pkg/devclaw/copilot"
)

func shutdownSignals() []os.Signal {
	return []os.Signal{syscall.SIGINT, syscall.SIGTERM}
}

func handleForceDreamSignal(_ os.Signal, _ *copilot.Assistant, _ *slog.Logger) bool {
	return false
}

func execReplace(executable string, args []string, env []string) error {
	cmd := exec.Command(executable, args...)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to spawn replacement process: %w", err)
	}
	os.Exit(0)
	return nil
}
