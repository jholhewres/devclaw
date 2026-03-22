// Package plugins – installer.go implements plugin installation from multiple
// sources: GitHub repositories and local paths.
//
// Supported sources:
//   - GitHub shorthand: "user/repo" or "github:user/repo"
//   - GitHub URL:       "https://github.com/user/repo"
//   - Local path:       "./path/to/plugin" or "/absolute/path"
package plugins

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PluginInstallResult holds the result of a plugin installation.
type PluginInstallResult struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Source  string `json:"source"`
	Path    string `json:"path"`
	IsNew   bool   `json:"is_new"`
}

// PluginInstaller handles plugin installation from various sources.
type PluginInstaller struct {
	pluginsDir string
	logger     *slog.Logger
}

// NewPluginInstaller creates a new plugin installer.
func NewPluginInstaller(pluginsDir string, logger *slog.Logger) *PluginInstaller {
	if logger == nil {
		logger = slog.Default()
	}
	return &PluginInstaller{
		pluginsDir: pluginsDir,
		logger:     logger.With("component", "plugin-installer"),
	}
}

// Install installs a plugin from the given source string.
// Supported formats:
//   - "user/repo" or "github:user/repo" — GitHub repository
//   - "https://github.com/user/repo" — GitHub URL
//   - "./path/to/plugin" or "/absolute/path" — local path
func (inst *PluginInstaller) Install(ctx context.Context, source string) (*PluginInstallResult, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return nil, fmt.Errorf("empty plugin source")
	}

	if err := os.MkdirAll(inst.pluginsDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating plugins directory: %w", err)
	}

	switch {
	case strings.HasPrefix(source, "github:"):
		repo := strings.TrimPrefix(source, "github:")
		return inst.installFromGitHub(ctx, repo)

	case strings.HasPrefix(source, "https://github.com/") || strings.HasPrefix(source, "http://github.com/"):
		repo := extractGitHubRepoURL(source)
		if repo == "" {
			return nil, fmt.Errorf("invalid GitHub URL: %s", source)
		}
		return inst.installFromGitHub(ctx, repo)

	case isLocalPluginPath(source):
		return inst.installFromLocal(source)

	default:
		// Treat as GitHub user/repo shorthand (e.g. "user/devclaw-plugin-foo").
		if strings.Contains(source, "/") && !strings.Contains(source, " ") {
			return inst.installFromGitHub(ctx, source)
		}
		return nil, fmt.Errorf("cannot determine source type for %q — use user/repo, github:<user/repo>, a GitHub URL, or a local path", source)
	}
}

// installFromGitHub clones a GitHub repository into the plugins directory.
func (inst *PluginInstaller) installFromGitHub(ctx context.Context, repo string) (*PluginInstallResult, error) {
	inst.logger.Info("installing plugin from GitHub", "repo", repo)

	parts := strings.Split(repo, "/")
	name := parts[len(parts)-1]
	name = strings.TrimSuffix(name, ".git")

	cloneURL := fmt.Sprintf("https://github.com/%s.git", repo)
	targetDir := filepath.Join(inst.pluginsDir, name)

	// Path traversal protection.
	absTarget, err := filepath.Abs(targetDir)
	if err != nil {
		return nil, fmt.Errorf("resolving path: %w", err)
	}
	absBase, err := filepath.Abs(inst.pluginsDir)
	if err != nil {
		return nil, fmt.Errorf("resolving plugins dir: %w", err)
	}
	if !strings.HasPrefix(absTarget, absBase+string(filepath.Separator)) {
		return nil, fmt.Errorf("plugin name %q escapes plugins directory", name)
	}

	isNew := !pluginDirExists(targetDir)

	if pluginDirExists(targetDir) {
		// Update: git pull.
		cmd := exec.CommandContext(ctx, "git", "-C", targetDir, "pull", "--ff-only")
		if out, err := cmd.CombinedOutput(); err != nil {
			inst.logger.Warn("git pull failed, re-cloning", "output", string(out), "error", err)
			_ = os.RemoveAll(targetDir)
			isNew = true
		} else {
			inst.logger.Info("plugin updated from GitHub", "name", name)
			return inst.validateAndResult(targetDir, "github:"+repo, isNew)
		}
	}

	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", cloneURL, targetDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("git clone failed: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return inst.validateAndResult(targetDir, "github:"+repo, isNew)
}

// installFromLocal copies a local plugin directory to the plugins dir.
func (inst *PluginInstaller) installFromLocal(source string) (*PluginInstallResult, error) {
	inst.logger.Info("installing plugin from local path", "path", source)

	absSource, err := filepath.Abs(source)
	if err != nil {
		return nil, err
	}

	if !pluginDirExists(absSource) {
		return nil, fmt.Errorf("local path not found: %s", absSource)
	}

	// Validate manifest exists in source.
	if _, err := os.Stat(filepath.Join(absSource, "plugin.yaml")); err != nil {
		return nil, fmt.Errorf("no plugin.yaml found in %s", absSource)
	}

	name := filepath.Base(absSource)
	targetDir := filepath.Join(inst.pluginsDir, name)

	// Don't copy if source IS the target.
	if absSource == targetDir {
		return inst.validateAndResult(targetDir, absSource, false)
	}

	isNew := !pluginDirExists(targetDir)
	_ = os.RemoveAll(targetDir)

	if err := copyPluginDir(absSource, targetDir); err != nil {
		return nil, fmt.Errorf("copying plugin: %w", err)
	}

	return inst.validateAndResult(targetDir, absSource, isNew)
}

// validateAndResult validates the plugin.yaml and returns the install result.
func (inst *PluginInstaller) validateAndResult(dir, source string, isNew bool) (*PluginInstallResult, error) {
	manifestPath := filepath.Join(dir, "plugin.yaml")
	manifest, err := ParseManifest(manifestPath)
	if err != nil {
		// Clean up on validation failure for new installs.
		if isNew {
			_ = os.RemoveAll(dir)
		}
		return nil, fmt.Errorf("invalid plugin: %w", err)
	}

	inst.logger.Info("plugin installed",
		"id", manifest.ID, "name", manifest.Name, "version", manifest.Version, "path", dir)

	return &PluginInstallResult{
		ID:      manifest.ID,
		Name:    manifest.Name,
		Version: manifest.Version,
		Source:  source,
		Path:    dir,
		IsNew:   isNew,
	}, nil
}

// Remove removes an installed plugin by ID or directory name.
func (inst *PluginInstaller) Remove(name string) error {
	targetDir := filepath.Join(inst.pluginsDir, name)

	// Path traversal protection: ensure target stays within plugins dir.
	absTarget, err := filepath.Abs(targetDir)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}
	absBase, err := filepath.Abs(inst.pluginsDir)
	if err != nil {
		return fmt.Errorf("resolving plugins dir: %w", err)
	}
	if !strings.HasPrefix(absTarget, absBase+string(filepath.Separator)) {
		return fmt.Errorf("plugin name %q escapes plugins directory", name)
	}

	if !pluginDirExists(targetDir) {
		return fmt.Errorf("plugin %q not found in %s", name, inst.pluginsDir)
	}

	if err := os.RemoveAll(targetDir); err != nil {
		return fmt.Errorf("removing plugin: %w", err)
	}

	inst.logger.Info("plugin removed", "name", name)
	return nil
}

// ---------- Helpers ----------

// extractGitHubRepoURL extracts "user/repo" from a GitHub URL.
func extractGitHubRepoURL(u string) string {
	for _, prefix := range []string{"https://github.com/", "http://github.com/"} {
		if strings.HasPrefix(u, prefix) {
			path := strings.TrimPrefix(u, prefix)
			path = strings.TrimSuffix(path, "/")
			path = strings.TrimSuffix(path, ".git")
			// Remove /tree/... or /blob/... suffixes.
			if idx := strings.Index(path, "/tree/"); idx >= 0 {
				path = path[:idx]
			}
			if idx := strings.Index(path, "/blob/"); idx >= 0 {
				path = path[:idx]
			}
			return path
		}
	}
	return ""
}

// isLocalPluginPath checks if a string looks like a local filesystem path.
func isLocalPluginPath(s string) bool {
	return strings.HasPrefix(s, "/") || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") || strings.HasPrefix(s, "~")
}

// pluginDirExists checks if a directory exists.
func pluginDirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// copyPluginDir recursively copies a directory, skipping symlinks.
func copyPluginDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		linfo, lerr := os.Lstat(path)
		if lerr != nil {
			return lerr
		}
		if linfo.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		relPath, _ := filepath.Rel(src, path)
		targetPath := filepath.Join(dst, relPath)

		if d.IsDir() {
			return os.MkdirAll(targetPath, linfo.Mode())
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(targetPath, data, linfo.Mode())
	})
}
