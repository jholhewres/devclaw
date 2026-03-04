// Package copilot – provider_discovery.go implements optional, non-blocking
// discovery of locally-hosted LLM providers (Ollama, vLLM).
//
// During startup, the assistant can probe known endpoints to discover
// available models and their context window sizes. This information
// supplements the static getModelContextWindowByName() lookup, allowing
// DevClaw to use correct settings for custom or fine-tuned local models.
package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

// ProviderDiscoveryConfig configures dynamic model discovery for local providers.
type ProviderDiscoveryConfig struct {
	// Enabled turns discovery on (default: false).
	Enabled bool `yaml:"enabled"`

	// OllamaURL is the base URL for the Ollama API (default: http://localhost:11434).
	OllamaURL string `yaml:"ollama_url"`

	// VLLMURL is the base URL for the vLLM API (default: http://localhost:8000).
	VLLMURL string `yaml:"vllm_url"`

	// VLLMAPIKey is the optional API key for vLLM (some deployments require auth).
	VLLMAPIKey string `yaml:"vllm_api_key"`

	// TimeoutSeconds is the per-probe HTTP timeout (default: 5).
	TimeoutSeconds int `yaml:"timeout_seconds"`
}

// effective returns a copy with defaults filled in for zero fields.
func (c ProviderDiscoveryConfig) effective() ProviderDiscoveryConfig {
	out := c
	if out.OllamaURL == "" {
		out.OllamaURL = "http://localhost:11434"
	}
	if out.VLLMURL == "" {
		out.VLLMURL = "http://localhost:8000"
	}
	if out.TimeoutSeconds <= 0 {
		out.TimeoutSeconds = 5
	}
	out.OllamaURL = strings.TrimRight(out.OllamaURL, "/")
	out.VLLMURL = strings.TrimRight(out.VLLMURL, "/")
	return out
}

// ---------------------------------------------------------------------------
// Discovered model info
// ---------------------------------------------------------------------------

// DiscoveredModel holds metadata about a model found via provider discovery.
type DiscoveredModel struct {
	// Name is the model identifier (e.g. "llama3:latest").
	Name string `json:"name"`

	// Provider is the source provider ("ollama", "vllm").
	Provider string `json:"provider"`

	// ContextWindow is the context window size in tokens (0 = unknown).
	ContextWindow int `json:"context_window,omitempty"`

	// ParameterSize is a human-readable parameter count (e.g. "7B", "70B").
	ParameterSize string `json:"parameter_size,omitempty"`
}

// ---------------------------------------------------------------------------
// ProviderDiscovery manages background model discovery
// ---------------------------------------------------------------------------

// ProviderDiscovery probes local LLM providers to discover available models.
type ProviderDiscovery struct {
	config ProviderDiscoveryConfig
	logger *slog.Logger
	client *http.Client

	mu     sync.RWMutex
	models map[string]DiscoveredModel // key: lowercase model name
}

// NewProviderDiscovery creates a new discovery instance.
func NewProviderDiscovery(cfg ProviderDiscoveryConfig, logger *slog.Logger) *ProviderDiscovery {
	eff := cfg.effective()
	return &ProviderDiscovery{
		config: eff,
		logger: logger.With("component", "provider-discovery"),
		client: &http.Client{
			Timeout: time.Duration(eff.TimeoutSeconds) * time.Second,
		},
		models: make(map[string]DiscoveredModel),
	}
}

// DiscoverAll probes all configured providers and populates the model cache.
// Safe to call from a goroutine; does not block on failure.
func (pd *ProviderDiscovery) DiscoverAll(ctx context.Context) {
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		pd.discoverOllama(ctx)
	}()
	go func() {
		defer wg.Done()
		pd.discoverVLLM(ctx)
	}()

	wg.Wait()

	pd.mu.RLock()
	count := len(pd.models)
	pd.mu.RUnlock()

	if count > 0 {
		pd.logger.Info("provider discovery complete", "models_found", count)
	}
}

// GetContextWindow returns the discovered context window for a model.
// Returns 0 if the model was not discovered or context window is unknown.
func (pd *ProviderDiscovery) GetContextWindow(model string) int {
	pd.mu.RLock()
	defer pd.mu.RUnlock()
	if m, ok := pd.models[strings.ToLower(model)]; ok {
		return m.ContextWindow
	}
	return 0
}

// ListModels returns a copy of all discovered models.
func (pd *ProviderDiscovery) ListModels() []DiscoveredModel {
	pd.mu.RLock()
	defer pd.mu.RUnlock()
	out := make([]DiscoveredModel, 0, len(pd.models))
	for _, m := range pd.models {
		out = append(out, m)
	}
	return out
}

// ---------------------------------------------------------------------------
// Ollama discovery
// ---------------------------------------------------------------------------

// ollamaTagsResponse is the JSON shape of GET /api/tags.
type ollamaTagsResponse struct {
	Models []ollamaModelEntry `json:"models"`
}

type ollamaModelEntry struct {
	Name       string `json:"name"`
	Model      string `json:"model"`
	ModifiedAt string `json:"modified_at"`
	Size       int64  `json:"size"`
}

// ollamaShowResponse is the JSON shape of POST /api/show (partial).
type ollamaShowResponse struct {
	ModelInfo map[string]any `json:"model_info"`
	Details   struct {
		ParameterSize string `json:"parameter_size"`
		Family        string `json:"family"`
	} `json:"details"`
}

func (pd *ProviderDiscovery) discoverOllama(ctx context.Context) {
	url := pd.config.OllamaURL + "/api/tags"

	body, err := pd.httpGet(ctx, url, "")
	if err != nil {
		pd.logger.Info("ollama discovery failed (is Ollama running?)", "error", err)
		return
	}

	var resp ollamaTagsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		pd.logger.Debug("ollama tags parse failed", "error", err)
		return
	}

	pd.logger.Debug("ollama models found", "count", len(resp.Models))

	for _, m := range resp.Models {
		name := m.Name
		if name == "" {
			name = m.Model
		}
		if name == "" {
			continue
		}

		dm := DiscoveredModel{
			Name:     name,
			Provider: "ollama",
		}

		// Query /api/show for context window and parameter details.
		if ctxWin, paramSize := pd.ollamaShow(ctx, name); ctxWin > 0 {
			dm.ContextWindow = ctxWin
			dm.ParameterSize = paramSize
		}

		pd.mu.Lock()
		pd.models[strings.ToLower(name)] = dm
		pd.mu.Unlock()
	}
}

// ollamaShow queries POST /api/show for model details.
// Returns (contextWindow, parameterSize).
func (pd *ProviderDiscovery) ollamaShow(ctx context.Context, model string) (int, string) {
	url := pd.config.OllamaURL + "/api/show"
	payloadBytes, _ := json.Marshal(map[string]string{"name": model})

	body, err := pd.httpPost(ctx, url, "", string(payloadBytes))
	if err != nil {
		return 0, ""
	}

	var resp ollamaShowResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, ""
	}

	ctxWindow := 0
	// model_info can contain context_length or num_ctx as various types.
	for _, key := range []string{
		"context_length",
		"llama.context_length",
		"general.context_length",
	} {
		if v, ok := resp.ModelInfo[key]; ok {
			switch n := v.(type) {
			case float64:
				ctxWindow = int(n)
			case json.Number:
				if i, err := n.Int64(); err == nil {
					ctxWindow = int(i)
				}
			case string:
				if i, err := strconv.Atoi(n); err == nil {
					ctxWindow = i
				}
			}
			if ctxWindow > 0 {
				break
			}
		}
	}

	return ctxWindow, resp.Details.ParameterSize
}

// ---------------------------------------------------------------------------
// vLLM discovery
// ---------------------------------------------------------------------------

// vllmModelsResponse is the JSON shape of GET /v1/models.
type vllmModelsResponse struct {
	Data []vllmModelEntry `json:"data"`
}

type vllmModelEntry struct {
	ID         string `json:"id"`
	Object     string `json:"object"`
	OwnedBy   string `json:"owned_by"`
	MaxModelLen int   `json:"max_model_len,omitempty"`
}

func (pd *ProviderDiscovery) discoverVLLM(ctx context.Context) {
	url := pd.config.VLLMURL + "/v1/models"

	body, err := pd.httpGet(ctx, url, pd.config.VLLMAPIKey)
	if err != nil {
		pd.logger.Info("vllm discovery failed (is vLLM running?)", "error", err)
		return
	}

	var resp vllmModelsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		pd.logger.Debug("vllm models parse failed", "error", err)
		return
	}

	pd.logger.Debug("vllm models found", "count", len(resp.Data))

	for _, m := range resp.Data {
		if m.ID == "" {
			continue
		}

		dm := DiscoveredModel{
			Name:          m.ID,
			Provider:      "vllm",
			ContextWindow: m.MaxModelLen,
		}

		pd.mu.Lock()
		pd.models[strings.ToLower(m.ID)] = dm
		pd.mu.Unlock()
	}
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

func (pd *ProviderDiscovery) httpGet(ctx context.Context, url, apiKey string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("provider discovery: new request: %w", err)
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	return pd.doRequest(req)
}

func (pd *ProviderDiscovery) httpPost(ctx context.Context, url, apiKey, body string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("provider discovery: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	return pd.doRequest(req)
}

func (pd *ProviderDiscovery) doRequest(req *http.Request) ([]byte, error) {
	resp, err := pd.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("provider discovery: %s %s: %w", req.Method, req.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("provider discovery: %s %s: status %d", req.Method, req.URL, resp.StatusCode)
	}

	// Cap body read to 10MB to handle large model lists while avoiding memory issues.
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("provider discovery: read body: %w", err)
	}
	return data, nil
}
