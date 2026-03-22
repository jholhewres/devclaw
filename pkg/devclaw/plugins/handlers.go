package plugins

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"plugin"
	"strings"
	"time"
)

// ToolHandlerFunc is the signature for plugin tool handlers.
type ToolHandlerFunc func(ctx context.Context, args map[string]any) (any, error)

// BuildToolHandler creates a tool handler function for a ToolDef.
// It dispatches to the appropriate handler type (script, HTTP, or native).
func BuildToolHandler(def ToolDef, instance *PluginInstance) (ToolHandlerFunc, error) {
	switch {
	case def.Script != "":
		return buildScriptHandler(def, instance.Dir, instance.Config), nil
	case def.Endpoint != "":
		return buildHTTPHandler(def, instance.Config), nil
	case def.Handler != "":
		return buildNativeHandler(def, instance.nativeHandle)
	default:
		return nil, fmt.Errorf("tool %q has no handler (script, endpoint, or handler required)", def.Name)
	}
}

// buildScriptHandler creates a handler that executes a bash script.
// Plugin config values are injected as PLUGIN_CFG_* environment variables,
// mirroring the pattern used by buildHTTPHandler.
func buildScriptHandler(def ToolDef, pluginDir string, config map[string]any) ToolHandlerFunc {
	return func(ctx context.Context, args map[string]any) (any, error) {
		// Build environment with tool parameters, prefixed to avoid overriding system vars.
		env := os.Environ()
		for k, v := range args {
			env = append(env, fmt.Sprintf("PLUGIN_%s=%v", strings.ToUpper(k), v))
		}

		// Inject plugin config as PLUGIN_CFG_* env vars.
		for k, v := range config {
			env = append(env, fmt.Sprintf("PLUGIN_CFG_%s=%v", strings.ToUpper(k), v))
		}

		cmd := exec.CommandContext(ctx, "bash", "-c", def.Script)
		cmd.Dir = pluginDir
		cmd.Env = env

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			errMsg := stderr.String()
			if errMsg == "" {
				errMsg = err.Error()
			}
			return nil, fmt.Errorf("script error: %s", strings.TrimSpace(errMsg))
		}

		result := strings.TrimSpace(stdout.String())
		const maxLen = 6000
		if len(result) > maxLen {
			result = result[:maxLen] + "\n... (truncated)"
		}

		return result, nil
	}
}

// buildHTTPHandler creates a handler that calls an HTTP endpoint.
// Migrated from copilot/plugin_system.go.
func buildHTTPHandler(def ToolDef, config map[string]any) ToolHandlerFunc {
	return func(ctx context.Context, args map[string]any) (any, error) {
		url := def.Endpoint
		apiPath, _ := args["path"].(string)
		body, _ := args["body"].(string)

		if apiPath != "" {
			url = strings.TrimRight(url, "/") + "/" + strings.TrimLeft(apiPath, "/")
		}

		var bodyReader io.Reader
		method := def.Method
		if method == "" {
			method = "GET"
		}
		if body != "" {
			bodyReader = strings.NewReader(body)
			if method == "GET" {
				method = "POST"
			}
		}

		req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}

		// Add auth headers from config.
		if token, ok := config["token"].(string); ok && token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		req.Header.Set("Accept", "application/json")
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}

		// Add custom headers from tool def.
		for k, v := range def.Headers {
			req.Header.Set(k, v)
		}

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("request failed: %w", err)
		}
		defer resp.Body.Close()

		const maxResponseBytes = 1 << 20 // 1 MiB
		limitedReader := io.LimitReader(resp.Body, maxResponseBytes)
		respBody, _ := io.ReadAll(limitedReader)
		result := strings.TrimSpace(string(respBody))

		const maxLen = 6000
		if len(result) > maxLen {
			result = result[:maxLen] + "\n... (truncated)"
		}

		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, result)
		}

		return result, nil
	}
}

// buildNativeHandler creates a handler that calls a Go symbol from a native .so.
func buildNativeHandler(def ToolDef, native *nativePlugin) (ToolHandlerFunc, error) {
	if native == nil || native.raw == nil {
		return nil, fmt.Errorf("tool %q requires native_lib but no .so loaded", def.Name)
	}

	p, ok := native.raw.(*plugin.Plugin)
	if !ok {
		return nil, fmt.Errorf("tool %q: native handle is not a *plugin.Plugin", def.Name)
	}

	sym, err := p.Lookup(def.Handler)
	if err != nil {
		return nil, fmt.Errorf("tool %q: symbol %q not found in .so: %w", def.Name, def.Handler, err)
	}

	// The symbol must be a function matching ToolHandlerFunc signature.
	handler, ok := sym.(*func(context.Context, map[string]any) (any, error))
	if !ok {
		return nil, fmt.Errorf("tool %q: symbol %q has wrong type (expected func(context.Context, map[string]any) (any, error))", def.Name, def.Handler)
	}

	return *handler, nil
}

// resolveInstructionsPath resolves an agent's instructions to content.
// If the instructions look like a file path (ends in .md), it's read from disk
// relative to the plugin directory. Otherwise, it's returned as-is (inline).
func resolveInstructionsPath(instructions, pluginDir string) (string, error) {
	if instructions == "" {
		return "", nil
	}

	// If it looks like a file path, read it.
	if strings.HasSuffix(instructions, ".md") || strings.Contains(instructions, string(filepath.Separator)) {
		path := filepath.Join(pluginDir, instructions)
		if err := validatePathWithinDir(path, pluginDir); err != nil {
			return "", err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("reading instructions file %s: %w", path, err)
		}
		return string(data), nil
	}

	// Otherwise return as inline instructions.
	return instructions, nil
}

// validatePathWithinDir ensures that the resolved path stays within the plugin directory,
// preventing path traversal attacks via relative paths or symlinks.
func validatePathWithinDir(path, dir string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolving absolute path: %w", err)
	}
	// Resolve symlinks to prevent traversal via symlinked files.
	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return fmt.Errorf("resolving symlinks: %w", err)
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolving absolute dir: %w", err)
	}
	if !strings.HasPrefix(realPath, absDir+string(filepath.Separator)) {
		return fmt.Errorf("path %q escapes plugin directory %q", path, dir)
	}
	return nil
}
