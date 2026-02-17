// Package copilot – workspace_containment.go implements workspace path containment
// and symlink escape protection for file operations.
//
// All file tools (read_file, write_file, edit_file, apply_patch) must call
// AssertSandboxPath before performing any I/O to ensure the resolved path
// is within the workspace root.
package copilot

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WorkspaceContainment enforces that file operations stay within the workspace root.
type WorkspaceContainment struct {
	// Root is the absolute path of the workspace root.
	Root string

	// Enabled toggles containment enforcement (default: true).
	Enabled bool
}

// NewWorkspaceContainment creates a containment checker for the given root directory.
func NewWorkspaceContainment(root string) *WorkspaceContainment {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	return &WorkspaceContainment{
		Root:    abs,
		Enabled: true,
	}
}

// AssertSandboxPath validates that the given path resolves to within the workspace root.
// It resolves symlinks to prevent escape attacks and checks for path traversal.
// Returns the resolved absolute path or an error if containment is violated.
func (wc *WorkspaceContainment) AssertSandboxPath(path string) (string, error) {
	if !wc.Enabled {
		return path, nil
	}

	// Convert to absolute path relative to workspace root.
	if !filepath.IsAbs(path) {
		path = filepath.Join(wc.Root, path)
	}

	// Clean the path to remove .. and . components.
	path = filepath.Clean(path)

	// Check for path traversal BEFORE resolving symlinks.
	if !strings.HasPrefix(path, wc.Root) {
		return "", fmt.Errorf("path %q escapes workspace root %q", path, wc.Root)
	}

	// Resolve symlinks to get the real path.
	resolved, err := resolveRealPath(path)
	if err != nil {
		// If the file doesn't exist yet, check the parent directory.
		parent := filepath.Dir(path)
		resolvedParent, parentErr := resolveRealPath(parent)
		if parentErr != nil {
			return "", fmt.Errorf("cannot resolve path %q: %w", path, err)
		}
		if !strings.HasPrefix(resolvedParent, wc.Root) {
			return "", fmt.Errorf("path %q resolves to %q which escapes workspace root %q",
				path, resolvedParent, wc.Root)
		}
		// Parent is safe, use the original cleaned path.
		return path, nil
	}

	// Verify the resolved path is still within the workspace.
	if !strings.HasPrefix(resolved, wc.Root) {
		return "", fmt.Errorf("path %q resolves to %q (symlink escape) which is outside workspace %q",
			path, resolved, wc.Root)
	}

	return resolved, nil
}

// AssertNoSymlinkEscape checks that a path does not traverse through a symlink
// that points outside the workspace. Unlike AssertSandboxPath, this allows
// symlinks WITHIN the workspace but blocks those that escape.
func (wc *WorkspaceContainment) AssertNoSymlinkEscape(path string) error {
	if !wc.Enabled {
		return nil
	}

	if !filepath.IsAbs(path) {
		path = filepath.Join(wc.Root, path)
	}
	path = filepath.Clean(path)

	// Walk each component of the path, checking for symlinks.
	components := strings.Split(strings.TrimPrefix(path, wc.Root+string(os.PathSeparator)), string(os.PathSeparator))
	current := wc.Root

	for _, component := range components {
		current = filepath.Join(current, component)

		info, err := os.Lstat(current)
		if err != nil {
			break // File doesn't exist yet — that's OK.
		}

		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(current)
			if err != nil {
				return fmt.Errorf("cannot read symlink %q: %w", current, err)
			}

			// Resolve relative symlinks.
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(current), target)
			}
			target = filepath.Clean(target)

			if !strings.HasPrefix(target, wc.Root) {
				return fmt.Errorf("symlink %q points to %q which escapes workspace %q",
					current, target, wc.Root)
			}
		}
	}

	return nil
}

// resolveRealPath resolves all symlinks in a path using filepath.EvalSymlinks.
func resolveRealPath(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}
