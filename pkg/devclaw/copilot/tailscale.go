// Package copilot – tailscale.go implements Tailscale Serve/Funnel integration
// for secure remote access to DevClaw's web services.
//
// Tailscale Serve proxies HTTPS traffic from the Tailscale network to a local
// port. Tailscale Funnel extends this to the public internet with automatic
// TLS certificates.
//
// Provides secure remote access without manual
// port forwarding, dynamic DNS, or certificate management.
//
// Architecture:
//
//	Internet ──HTTPS──▶ Tailscale Funnel ──▶ Local DevClaw (e.g. :8080)
//	Tailnet  ──HTTPS──▶ Tailscale Serve  ──▶ Local DevClaw (e.g. :8080)
package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// TailscaleConfig configures Tailscale Serve/Funnel integration.
type TailscaleConfig struct {
	// Enabled turns Tailscale integration on/off.
	Enabled bool `yaml:"enabled"`

	// Serve enables Tailscale Serve (accessible within your Tailnet).
	Serve bool `yaml:"serve"`

	// Funnel enables Tailscale Funnel (accessible from the public internet).
	// Requires Tailscale Funnel to be enabled in your Tailscale admin console.
	Funnel bool `yaml:"funnel"`

	// Port is the local port to proxy (default: 8080).
	Port int `yaml:"port"`

	// Hostname is the Tailscale hostname to use (empty = auto from `tailscale status`).
	Hostname string `yaml:"hostname"`
}

// TailscaleManager manages Tailscale Serve/Funnel lifecycle.
type TailscaleManager struct {
	cfg    TailscaleConfig
	logger *slog.Logger

	mu       sync.Mutex
	serving  bool
	funneled bool
	hostname string
}

// NewTailscaleManager creates a new Tailscale manager.
func NewTailscaleManager(cfg TailscaleConfig, logger *slog.Logger) *TailscaleManager {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.Port <= 0 {
		cfg.Port = 8080
	}
	return &TailscaleManager{
		cfg:    cfg,
		logger: logger.With("component", "tailscale"),
	}
}

// Start sets up Tailscale Serve and/or Funnel based on config.
func (tm *TailscaleManager) Start(ctx context.Context) error {
	if !tm.cfg.Enabled {
		return nil
	}

	// Check if Tailscale is available and connected.
	if !tm.isAvailable() {
		return fmt.Errorf("tailscale CLI not found or not connected; install from https://tailscale.com/download")
	}

	// Get hostname.
	hostname := tm.cfg.Hostname
	if hostname == "" {
		var err error
		hostname, err = tm.getHostname()
		if err != nil {
			return fmt.Errorf("failed to get Tailscale hostname: %w", err)
		}
	}

	tm.mu.Lock()
	tm.hostname = hostname
	tm.mu.Unlock()

	// Set up Tailscale Serve.
	if tm.cfg.Serve || tm.cfg.Funnel {
		if err := tm.setupServe(); err != nil {
			return fmt.Errorf("tailscale serve setup failed: %w", err)
		}
	}

	// Set up Tailscale Funnel if requested.
	if tm.cfg.Funnel {
		if err := tm.setupFunnel(); err != nil {
			tm.logger.Warn("tailscale funnel setup failed (serve still active)", "error", err)
		}
	}

	return nil
}

// Stop tears down Tailscale Serve/Funnel.
func (tm *TailscaleManager) Stop() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.funneled {
		tm.runTailscale("funnel", "--https=443", "off")
		tm.funneled = false
		tm.logger.Info("tailscale funnel stopped")
	}
	if tm.serving {
		tm.runTailscale("serve", "--https=443", "off")
		tm.serving = false
		tm.logger.Info("tailscale serve stopped")
	}
}

// Status returns the current Tailscale integration status.
func (tm *TailscaleManager) Status() map[string]any {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	status := map[string]any{
		"enabled":  tm.cfg.Enabled,
		"serving":  tm.serving,
		"funneled": tm.funneled,
		"hostname": tm.hostname,
		"port":     tm.cfg.Port,
	}

	if tm.serving {
		status["serve_url"] = fmt.Sprintf("https://%s/", tm.hostname)
	}
	if tm.funneled {
		status["funnel_url"] = fmt.Sprintf("https://%s/", tm.hostname)
	}

	return status
}

// URL returns the public-facing URL if available, or empty string.
func (tm *TailscaleManager) URL() string {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if tm.hostname == "" {
		return ""
	}
	return fmt.Sprintf("https://%s", tm.hostname)
}

// ─── Internal ───

func (tm *TailscaleManager) isAvailable() bool {
	out, err := tm.runTailscaleOutput("status", "--json")
	if err != nil {
		return false
	}
	return strings.Contains(out, "BackendState")
}

func (tm *TailscaleManager) getHostname() (string, error) {
	out, err := tm.runTailscaleOutput("status", "--json")
	if err != nil {
		return "", err
	}

	// Parse Self.DNSName from the tailscale status JSON.
	var status struct {
		Self struct {
			DNSName string `json:"DNSName"`
		} `json:"Self"`
	}
	if err := json.Unmarshal([]byte(out), &status); err != nil {
		return "", fmt.Errorf("parsing tailscale status: %w", err)
	}
	if status.Self.DNSName == "" {
		return "", fmt.Errorf("no DNSName found in tailscale status")
	}

	return strings.TrimSuffix(status.Self.DNSName, "."), nil
}

func (tm *TailscaleManager) setupServe() error {
	target := fmt.Sprintf("http://127.0.0.1:%d", tm.cfg.Port)
	_, err := tm.runTailscaleOutput("serve", "--https=443", target)
	if err != nil {
		return err
	}

	tm.mu.Lock()
	tm.serving = true
	tm.mu.Unlock()

	tm.logger.Info("tailscale serve started",
		"hostname", tm.hostname,
		"target", target,
		"url", fmt.Sprintf("https://%s/", tm.hostname),
	)
	return nil
}

func (tm *TailscaleManager) setupFunnel() error {
	_, err := tm.runTailscaleOutput("funnel", "--https=443", "on")
	if err != nil {
		return err
	}

	tm.mu.Lock()
	tm.funneled = true
	tm.mu.Unlock()

	tm.logger.Info("tailscale funnel enabled",
		"hostname", tm.hostname,
		"public_url", fmt.Sprintf("https://%s/", tm.hostname),
	)
	return nil
}

func (tm *TailscaleManager) runTailscale(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "tailscale", args...)
	return cmd.Run()
}

func (tm *TailscaleManager) runTailscaleOutput(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "tailscale", args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
