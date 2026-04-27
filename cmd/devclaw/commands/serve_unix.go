//go:build !windows

package commands

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"syscall"

	"github.com/jholhewres/devclaw/pkg/devclaw/copilot"
)

func shutdownSignals() []os.Signal {
	return []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1}
}

func handleForceDreamSignal(sig os.Signal, assistant *copilot.Assistant, logger *slog.Logger) bool {
	if sig == syscall.SIGUSR1 {
		logger.Info("SIGUSR1 received — forcing dream cycle")
		go assistant.ForceDream(context.Background())
		return true
	}
	return false
}

func execReplace(executable string, args []string, env []string) error {
	if err := syscall.Exec(executable, append([]string{executable}, args...), env); err != nil {
		return fmt.Errorf("failed to reload process: %w", err)
	}
	return nil
}
