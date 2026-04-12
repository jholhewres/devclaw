// Package memory — embeddings_onnx.go implements a local ONNX-based embedding
// provider using sentence-transformer models (all-MiniLM-L6-v2).
// Zero API calls — runs entirely on CPU via ONNX Runtime.
//
// Auto-downloads the ONNX runtime shared library and model files on first use
// to ~/.devclaw/lib/ and ~/.devclaw/models/. Gracefully degrades to
// NullEmbedder if the runtime is unavailable.
package memory

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	ort "github.com/yalue/onnxruntime_go"
)

const (
	onnxModelName      = "all-MiniLM-L6-v2"
	onnxModelDims      = 384
	onnxMaxSeqLen      = 128
	onnxRuntimeVersion = "1.24.1"

	// Download URLs.
	onnxRuntimeURLTemplate = "https://github.com/microsoft/onnxruntime/releases/download/v%s/onnxruntime-linux-x64-%s.tgz"
	onnxModelBaseURL       = "https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/onnx"
)

// ONNXEmbedder generates embeddings using a local ONNX sentence-transformer.
type ONNXEmbedder struct {
	session   *ort.AdvancedSession
	tokenizer *WordPieceTokenizer
	dims      int
	logger    *slog.Logger

	// Pre-allocated tensors (reused across calls).
	inputIDs      *ort.Tensor[int64]
	attentionMask *ort.Tensor[int64]
	tokenTypeIDs  *ort.Tensor[int64]
	output        *ort.Tensor[float32]

	mu sync.Mutex // session is not thread-safe for concurrent Run()
}

// ONNXPaths holds resolved paths for ONNX runtime and model files.
type ONNXPaths struct {
	RuntimeLib string // path to libonnxruntime.so
	ModelFile  string // path to model.onnx
	VocabFile  string // path to vocab.txt
}

// NewONNXEmbedder creates a local ONNX embedding provider.
// Auto-downloads runtime and model if not present.
func NewONNXEmbedder(cfg EmbeddingConfig) (*ONNXEmbedder, error) {
	logger := slog.Default().With("component", "onnx-embedder")

	paths, err := resolveONNXPaths()
	if err != nil {
		return nil, fmt.Errorf("onnx: resolve paths: %w", err)
	}

	// Ensure runtime and model files exist (auto-download if needed).
	if err := ensureONNXRuntime(paths, logger); err != nil {
		return nil, fmt.Errorf("onnx: runtime setup: %w", err)
	}
	if err := ensureONNXModel(paths, logger); err != nil {
		return nil, fmt.Errorf("onnx: model setup: %w", err)
	}

	// Initialize ONNX Runtime.
	ort.SetSharedLibraryPath(paths.RuntimeLib)
	if !ort.IsInitialized() {
		if err := ort.InitializeEnvironment(); err != nil {
			return nil, fmt.Errorf("onnx: init environment: %w", err)
		}
	}

	// Load tokenizer.
	tokenizer, err := NewWordPieceTokenizer(paths.VocabFile, onnxMaxSeqLen)
	if err != nil {
		return nil, fmt.Errorf("onnx: load tokenizer: %w", err)
	}

	// Create pre-allocated tensors.
	shape := ort.NewShape(1, int64(onnxMaxSeqLen))
	inputIDs, err := ort.NewEmptyTensor[int64](shape)
	if err != nil {
		return nil, fmt.Errorf("onnx: create input_ids tensor: %w", err)
	}
	attentionMask, err := ort.NewEmptyTensor[int64](shape)
	if err != nil {
		return nil, fmt.Errorf("onnx: create attention_mask tensor: %w", err)
	}
	tokenTypeIDs, err := ort.NewEmptyTensor[int64](shape)
	if err != nil {
		return nil, fmt.Errorf("onnx: create token_type_ids tensor: %w", err)
	}

	outputShape := ort.NewShape(1, int64(onnxMaxSeqLen), int64(onnxModelDims))
	output, err := ort.NewEmptyTensor[float32](outputShape)
	if err != nil {
		return nil, fmt.Errorf("onnx: create output tensor: %w", err)
	}

	// Create session.
	session, err := ort.NewAdvancedSession(
		paths.ModelFile,
		[]string{"input_ids", "attention_mask", "token_type_ids"},
		[]string{"last_hidden_state"},
		[]ort.Value{inputIDs, attentionMask, tokenTypeIDs},
		[]ort.Value{output},
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("onnx: create session: %w", err)
	}

	logger.Info("ONNX embedder initialized",
		"model", onnxModelName,
		"dims", onnxModelDims,
		"max_seq_len", onnxMaxSeqLen,
	)

	return &ONNXEmbedder{
		session:       session,
		tokenizer:     tokenizer,
		dims:          onnxModelDims,
		logger:        logger,
		inputIDs:      inputIDs,
		attentionMask: attentionMask,
		tokenTypeIDs:  tokenTypeIDs,
		output:        output,
	}, nil
}

// Embed generates embeddings for a batch of texts.
func (e *ONNXEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, text := range texts {
		vec, err := e.embedSingle(text)
		if err != nil {
			return nil, fmt.Errorf("onnx embed text %d: %w", i, err)
		}
		results[i] = vec
	}
	return results, nil
}

// embedSingle embeds a single text string.
func (e *ONNXEmbedder) embedSingle(text string) ([]float32, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Tokenize.
	ids, mask, typeIDs := e.tokenizer.Tokenize(text)

	// Copy into pre-allocated tensors.
	copy(e.inputIDs.GetData(), ids)
	copy(e.attentionMask.GetData(), mask)
	copy(e.tokenTypeIDs.GetData(), typeIDs)

	// Run inference.
	if err := e.session.Run(); err != nil {
		return nil, fmt.Errorf("session run: %w", err)
	}

	// Mean pooling over token embeddings (respecting attention mask).
	raw := e.output.GetData() // [1, seq_len, dims] flattened
	vec := meanPool(raw, mask, onnxMaxSeqLen, e.dims)

	// L2 normalize.
	l2Normalize(vec)

	// Return a copy (tensor data is reused).
	result := make([]float32, len(vec))
	copy(result, vec)
	return result, nil
}

// Dimensions returns the embedding dimensionality.
func (e *ONNXEmbedder) Dimensions() int { return e.dims }

// Name returns the provider name.
func (e *ONNXEmbedder) Name() string { return "onnx" }

// Model returns the model name.
func (e *ONNXEmbedder) Model() string { return onnxModelName }

// Close releases ONNX resources.
func (e *ONNXEmbedder) Close() error {
	if e.session != nil {
		e.session.Destroy()
	}
	if e.inputIDs != nil {
		e.inputIDs.Destroy()
	}
	if e.attentionMask != nil {
		e.attentionMask.Destroy()
	}
	if e.tokenTypeIDs != nil {
		e.tokenTypeIDs.Destroy()
	}
	if e.output != nil {
		e.output.Destroy()
	}
	return nil
}

// ---------- Math helpers ----------

// meanPool computes the mean of token embeddings weighted by attention mask.
// raw is flattened [1, seqLen, dims], mask is [seqLen].
func meanPool(raw []float32, mask []int64, seqLen, dims int) []float32 {
	result := make([]float32, dims)
	var count float32
	for i := 0; i < seqLen; i++ {
		if mask[i] == 0 {
			continue
		}
		count++
		offset := i * dims
		for j := 0; j < dims; j++ {
			result[j] += raw[offset+j]
		}
	}
	if count > 0 {
		for j := range result {
			result[j] /= count
		}
	}
	return result
}

// l2Normalize normalizes a vector to unit length in-place.
func l2Normalize(vec []float32) {
	var sum float64
	for _, v := range vec {
		sum += float64(v) * float64(v)
	}
	norm := float32(math.Sqrt(sum))
	if norm > 0 {
		for i := range vec {
			vec[i] /= norm
		}
	}
}

// ---------- Path resolution ----------

func resolveONNXPaths() (*ONNXPaths, error) {
	// Use the project's data directory (./data/onnx/), consistent with
	// paths.ResolveDataDir() and the rest of DevClaw's state layout.
	dataDir := "data"
	if d := os.Getenv("DEVCLAW_STATE_DIR"); d != "" {
		dataDir = filepath.Join(d, "data")
	}

	libDir := filepath.Join(dataDir, "onnx", "lib")
	modelDir := filepath.Join(dataDir, "onnx", "models", onnxModelName)

	libName := "libonnxruntime.so"
	if runtime.GOOS == "darwin" {
		libName = "libonnxruntime.dylib"
	}

	return &ONNXPaths{
		RuntimeLib: filepath.Join(libDir, libName),
		ModelFile:  filepath.Join(modelDir, "model.onnx"),
		VocabFile:  filepath.Join(modelDir, "vocab.txt"),
	}, nil
}

// ---------- Auto-download ----------

func ensureONNXRuntime(paths *ONNXPaths, logger *slog.Logger) error {
	if _, err := os.Stat(paths.RuntimeLib); err == nil {
		return nil // Already exists.
	}

	logger.Info("downloading ONNX Runtime (first run)...", "version", onnxRuntimeVersion)
	if err := os.MkdirAll(filepath.Dir(paths.RuntimeLib), 0o755); err != nil {
		return err
	}

	// Download the .tgz and extract just the .so file.
	url := fmt.Sprintf(onnxRuntimeURLTemplate, onnxRuntimeVersion, onnxRuntimeVersion)
	return downloadAndExtractLib(url, paths.RuntimeLib, logger)
}

func ensureONNXModel(paths *ONNXPaths, logger *slog.Logger) error {
	modelExists := fileExists(paths.ModelFile)
	vocabExists := fileExists(paths.VocabFile)

	if modelExists && vocabExists {
		return nil
	}

	logger.Info("downloading ONNX model (first run)...", "model", onnxModelName)
	if err := os.MkdirAll(filepath.Dir(paths.ModelFile), 0o755); err != nil {
		return err
	}

	if !modelExists {
		modelURL := onnxModelBaseURL + "/model.onnx"
		if err := downloadFile(modelURL, paths.ModelFile, logger); err != nil {
			return fmt.Errorf("download model: %w", err)
		}
	}

	if !vocabExists {
		vocabURL := "https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/vocab.txt"
		if err := downloadFile(vocabURL, paths.VocabFile, logger); err != nil {
			return fmt.Errorf("download vocab: %w", err)
		}
	}

	return nil
}

func downloadFile(url, destPath string, logger *slog.Logger) error {
	logger.Info("downloading", "url", url, "dest", destPath)

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}

	tmpPath := destPath + ".download"
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	_, err = io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		os.Remove(tmpPath)
		return err
	}

	return os.Rename(tmpPath, destPath)
}

func downloadAndExtractLib(tgzURL, destPath string, logger *slog.Logger) error {
	// Download the .tgz to a temp file.
	tmpTgz := destPath + ".tgz"
	if err := downloadFile(tgzURL, tmpTgz, logger); err != nil {
		return err
	}
	defer os.Remove(tmpTgz)

	// Extract the shared library from the archive.
	// The .so is at onnxruntime-linux-x64-VERSION/lib/libonnxruntime.so.VERSION
	return extractLibFromTgz(tmpTgz, destPath)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
