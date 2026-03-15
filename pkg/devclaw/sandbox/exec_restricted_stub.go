//go:build !linux && !windows

package sandbox

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
)

// RestrictedExecutor is a no-op stub for non-Linux platforms.
// Linux namespace isolation is only available on Linux.
type RestrictedExecutor struct {
	cfg    Config
	logger *slog.Logger
}

func NewRestrictedExecutor(cfg Config, logger *slog.Logger) *RestrictedExecutor {
	return &RestrictedExecutor{cfg: cfg, logger: logger}
}

func (e *RestrictedExecutor) Execute(_ context.Context, _ *ExecRequest) (*ExecResult, error) {
	return nil, fmt.Errorf("restricted executor requires Linux, current OS: %s", runtime.GOOS)
}

func (e *RestrictedExecutor) Available() bool { return false }
func (e *RestrictedExecutor) Name() string    { return "restricted" }
func (e *RestrictedExecutor) Close() error    { return nil }
