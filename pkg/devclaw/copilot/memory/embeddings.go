// Package memory – embeddings.go implements embedding generation for semantic search.
// Supports multiple providers: OpenAI, and a zero-cost local fallback.
// Embeddings are cached by content hash + provider + model to avoid redundant API calls.
package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// EmbeddingProvider generates vector embeddings from text.
type EmbeddingProvider interface {
	// Embed generates embeddings for a batch of texts.
	// Returns one float32 vector per input text.
	Embed(ctx context.Context, texts []string) ([][]float32, error)

	// Dimensions returns the dimensionality of the output vectors.
	Dimensions() int

	// Name returns the provider name (for cache key derivation).
	Name() string

	// Model returns the model name (for cache key derivation).
	Model() string
}

// EmbeddingConfig configures the embedding provider.
type EmbeddingConfig struct {
	// Provider is the embedding provider ("openai", "none").
	Provider string `yaml:"provider"`

	// Model is the embedding model name (e.g. "text-embedding-3-small").
	Model string `yaml:"model"`

	// Dimensions is the output vector dimensionality (default: auto from model).
	Dimensions int `yaml:"dimensions"`

	// APIKey is the API key for the embedding provider. If empty, falls back to
	// the main LLM API key.
	APIKey string `yaml:"api_key"`

	// BaseURL is the API base URL. If empty, uses the provider default.
	BaseURL string `yaml:"base_url"`

	// Cache enables embedding caching in SQLite (default: true).
	Cache bool `yaml:"cache"`
}

// DefaultEmbeddingConfig returns sensible defaults.
func DefaultEmbeddingConfig() EmbeddingConfig {
	return EmbeddingConfig{
		Provider:   "none",
		Model:      "text-embedding-3-small",
		Dimensions: 1536,
		Cache:      true,
	}
}

// ---------- OpenAI Embedding Provider ----------

// OpenAIEmbedder generates embeddings using the OpenAI Embeddings API.
type OpenAIEmbedder struct {
	apiKey     string
	model      string
	dimensions int
	baseURL    string
	client     *http.Client
}

// NewOpenAIEmbedder creates an OpenAI embedding provider.
func NewOpenAIEmbedder(cfg EmbeddingConfig) *OpenAIEmbedder {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	dims := cfg.Dimensions
	if dims <= 0 {
		dims = 1536
	}
	model := cfg.Model
	if model == "" {
		model = "text-embedding-3-small"
	}
	return &OpenAIEmbedder{
		apiKey:     cfg.APIKey,
		model:      model,
		dimensions: dims,
		baseURL:    baseURL,
		client:     &http.Client{Timeout: 30 * time.Second},
	}
}

// openaiEmbedRequest is the OpenAI embeddings API request.
type openaiEmbedRequest struct {
	Model      string   `json:"model"`
	Input      []string `json:"input"`
	Dimensions int      `json:"dimensions,omitempty"`
}

// openaiEmbedResponse is the OpenAI embeddings API response.
type openaiEmbedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Embed generates embeddings for a batch of texts.
func (e *OpenAIEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	reqBody := openaiEmbedRequest{
		Model: e.model,
		Input: texts,
	}
	if e.dimensions > 0 {
		reqBody.Dimensions = e.dimensions
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal embed request: %w", err)
	}

	endpoint := e.baseURL + "/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create embed request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed API call: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read embed response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embed API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result openaiEmbedResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal embed response: %w", err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("embed API error: %s", result.Error.Message)
	}

	// Sort by index to match input order.
	embeddings := make([][]float32, len(texts))
	for _, d := range result.Data {
		if d.Index < len(embeddings) {
			embeddings[d.Index] = d.Embedding
		}
	}

	return embeddings, nil
}

// Dimensions returns the output vector dimensionality.
func (e *OpenAIEmbedder) Dimensions() int { return e.dimensions }

// Name returns the provider name.
func (e *OpenAIEmbedder) Name() string { return "openai" }

// Model returns the model name.
func (e *OpenAIEmbedder) Model() string { return e.model }

// ---------- Null Embedding Provider ----------

// NullEmbedder is a no-op provider that disables semantic search.
// Used when no embedding provider is configured.
type NullEmbedder struct{}

// Embed returns nil (no embeddings).
func (e *NullEmbedder) Embed(_ context.Context, _ []string) ([][]float32, error) {
	return nil, nil
}

// Dimensions returns 0.
func (e *NullEmbedder) Dimensions() int { return 0 }

// Name returns "none".
func (e *NullEmbedder) Name() string { return "none" }

// Model returns "none".
func (e *NullEmbedder) Model() string { return "none" }

// NewEmbeddingProvider creates an embedding provider from config.
func NewEmbeddingProvider(cfg EmbeddingConfig) EmbeddingProvider {
	switch cfg.Provider {
	case "openai":
		return NewOpenAIEmbedder(cfg)
	default:
		return &NullEmbedder{}
	}
}
