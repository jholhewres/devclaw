// Package sandbox – exec_docker.go implements the Docker-based executor
// for maximum script isolation.
//
// This executor provides:
//   - Full filesystem isolation (container image)
//   - Network isolation (configurable)
//   - CPU and memory limits via Docker
//   - Read-only workspace mount
//   - Writable temp directory mount
//   - Automatic sandbox image building
package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
)

// DockerExecutor runs scripts inside Docker containers.
type DockerExecutor struct {
	cfg    Config
	logger *slog.Logger
}

// NewDockerExecutor creates a new Docker executor.
func NewDockerExecutor(cfg Config, logger *slog.Logger) *DockerExecutor {
	return &DockerExecutor{cfg: cfg, logger: logger}
}

// Execute runs the script inside a Docker container.
func (e *DockerExecutor) Execute(ctx context.Context, req *ExecRequest) (*ExecResult, error) {
	if !e.Available() {
		return nil, fmt.Errorf("docker not available")
	}

	// Ensure the sandbox image exists.
	if err := e.ensureImage(ctx); err != nil {
		return nil, fmt.Errorf("ensuring sandbox image: %w", err)
	}

	args := e.buildDockerArgs(req)
	cmd := exec.CommandContext(ctx, "docker", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if req.Stdin != "" {
		cmd.Stdin = strings.NewReader(req.Stdin)
	}

	err := cmd.Run()

	result := &ExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			if ctx.Err() != nil {
				result.Killed = true
				result.KillReason = "timeout"
			}
			// Exit code 137 = SIGKILL (usually OOM killer).
			if result.ExitCode == 137 {
				result.Killed = true
				result.KillReason = "oom_killed"
			}
		} else {
			return result, fmt.Errorf("docker run: %w", err)
		}
	}

	return result, nil
}

// Available checks if Docker is installed and running.
func (e *DockerExecutor) Available() bool {
	cmd := exec.Command("docker", "info")
	return cmd.Run() == nil
}

// Name returns the executor name.
func (e *DockerExecutor) Name() string { return "docker" }

// Close is a no-op for Docker executor.
func (e *DockerExecutor) Close() error { return nil }

// BuildImage builds the sandbox Docker image.
func (e *DockerExecutor) BuildImage(ctx context.Context, dockerfilePath string) error {
	cmd := exec.CommandContext(ctx, "docker", "build",
		"-t", e.cfg.Docker.Image,
		"-f", dockerfilePath,
		".")
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

// ---------- Internal ----------

// buildDockerArgs constructs the docker run command arguments.
func (e *DockerExecutor) buildDockerArgs(req *ExecRequest) []string {
	args := []string{"run", "--rm"}

	// Security options.
	args = append(args, "--security-opt", "no-new-privileges")
	args = append(args, "--cap-drop", "ALL")
	args = append(args, "--read-only")

	// Resource limits.
	if e.cfg.MaxMemoryMB > 0 {
		args = append(args, "--memory", fmt.Sprintf("%dm", e.cfg.MaxMemoryMB))
		args = append(args, "--memory-swap", fmt.Sprintf("%dm", e.cfg.MaxMemoryMB))
	}
	if e.cfg.MaxCPUPercent > 0 {
		// Docker --cpus expects a float (e.g., 0.5 = 50%).
		cpus := float64(e.cfg.MaxCPUPercent) / 100.0
		args = append(args, "--cpus", strconv.FormatFloat(cpus, 'f', 2, 64))
	}

	// Network isolation.
	network := e.cfg.Docker.Network
	if network == "" {
		network = "none"
	}
	args = append(args, "--network", network)

	// Timeout via Docker's --stop-timeout.
	if req.Timeout > 0 {
		secs := int(req.Timeout.Seconds())
		args = append(args, "--stop-timeout", fmt.Sprintf("%d", secs))
	}

	// Mount skill directory as read-only.
	if req.SkillDir != "" {
		args = append(args, "-v", req.SkillDir+":/skill:ro")
	}

	// Mount temp directory as writable.
	if tmpDir, ok := req.Env["DEVCLAW_TMPDIR"]; ok {
		args = append(args, "-v", tmpDir+":/tmp:rw")
		args = append(args, "--tmpfs", "/run:rw,noexec,nosuid,size=64m")
	}

	// Mount working directory if specified.
	if req.WorkDir != "" {
		args = append(args, "-v", req.WorkDir+":/workspace:ro")
		args = append(args, "-w", "/workspace")
	}

	// Extra volumes from config.
	for _, vol := range e.cfg.Docker.ExtraVolumes {
		args = append(args, "-v", vol)
	}

	// Environment variables.
	for k, v := range req.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}
	// Override baseDir to container path.
	if req.SkillDir != "" {
		args = append(args, "-e", "DEVCLAW_SKILL_DIR=/skill")
	}

	// Enable stdin if needed.
	if req.Stdin != "" {
		args = append(args, "-i")
	}

	// Image.
	args = append(args, e.cfg.Docker.Image)

	// Command: interpreter + script + args.
	bin, cmdArgs := e.resolveContainerCommand(req)
	args = append(args, bin)
	args = append(args, cmdArgs...)

	return args
}

// resolveContainerCommand determines the command to run inside the container.
func (e *DockerExecutor) resolveContainerCommand(req *ExecRequest) (string, []string) {
	// Inside the container, skill scripts are at /skill/scripts/...
	script := req.Script
	if req.SkillDir != "" && strings.HasPrefix(script, req.SkillDir) {
		script = "/skill" + strings.TrimPrefix(script, req.SkillDir)
	}

	switch req.Runtime {
	case RuntimePython:
		return "python3", append([]string{"-u", script}, req.Args...)
	case RuntimeNode:
		return "node", append([]string{script}, req.Args...)
	case RuntimeShell:
		return "/bin/sh", append([]string{script}, req.Args...)
	default:
		return script, req.Args
	}
}

// ensureImage checks if the sandbox image exists, builds if needed.
func (e *DockerExecutor) ensureImage(ctx context.Context) error {
	// Check if image exists.
	cmd := exec.CommandContext(ctx, "docker", "image", "inspect", e.cfg.Docker.Image)
	if cmd.Run() == nil {
		return nil // Image exists.
	}

	if !e.cfg.Docker.BuildOnStart {
		return fmt.Errorf("sandbox image %q not found (set docker.build_on_start to auto-build)", e.cfg.Docker.Image)
	}

	e.logger.Info("sandbox: building Docker image", "image", e.cfg.Docker.Image)

	// Build from inline Dockerfile.
	dockerfile := generateSandboxDockerfile()
	buildCmd := exec.CommandContext(ctx, "docker", "build",
		"-t", e.cfg.Docker.Image,
		"-f", "-", // Read Dockerfile from stdin.
		".")
	buildCmd.Stdin = strings.NewReader(dockerfile)

	var buildOut bytes.Buffer
	buildCmd.Stdout = &buildOut
	buildCmd.Stderr = &buildOut

	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("building sandbox image: %w\n%s", err, buildOut.String())
	}

	e.logger.Info("sandbox: Docker image built successfully")
	return nil
}

// generateSandboxDockerfile creates a Dockerfile for the sandbox image.
// Includes Python, Node.js, and common CLI tools.
func generateSandboxDockerfile() string {
	return `FROM debian:bookworm-slim

# Install runtimes and common tools.
RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 python3-pip python3-venv \
    nodejs npm \
    bash curl wget jq \
    git ca-certificates \
    ffmpeg \
    && rm -rf /var/lib/apt/lists/*

# Install uv (fast Python package manager).
RUN curl -LsSf https://astral.sh/uv/install.sh | sh

# Create non-root user for script execution.
RUN groupadd -r sandbox && useradd -r -g sandbox -d /tmp -s /bin/bash sandbox

# Default working directory.
WORKDIR /workspace

# Run as non-root.
USER sandbox

# Default entrypoint (overridden per execution).
ENTRYPOINT ["/bin/sh", "-c"]
`
}
