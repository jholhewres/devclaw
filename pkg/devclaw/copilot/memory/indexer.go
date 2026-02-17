// Package memory – indexer.go implements markdown chunking and hash-based
// delta sync for the memory index. Files are split into chunks, each chunk
// gets a SHA-256 hash, and only changed chunks are re-embedded.
package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Chunk represents an indexed text fragment from a memory file.
type Chunk struct {
	// FileID identifies the source file (relative path).
	FileID string

	// Index is the chunk position within the file (0-based).
	Index int

	// Text is the chunk content.
	Text string

	// Hash is the SHA-256 hex digest of the chunk text.
	Hash string
}

// ChunkConfig controls the chunking behavior.
type ChunkConfig struct {
	// MaxTokens is the approximate max tokens per chunk (default: 500).
	// Uses ~4 chars/token heuristic.
	MaxTokens int

	// Overlap is the number of characters to overlap between chunks (default: 100).
	Overlap int
}

// DefaultChunkConfig returns sensible defaults.
func DefaultChunkConfig() ChunkConfig {
	return ChunkConfig{
		MaxTokens: 500,
		Overlap:   100,
	}
}

// ChunkMarkdown splits markdown text into semantic chunks.
// Prefers splitting at heading boundaries, then paragraph boundaries,
// then sentence boundaries, respecting MaxTokens.
func ChunkMarkdown(text string, cfg ChunkConfig) []Chunk {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	maxChars := cfg.MaxTokens * 4 // ~4 chars per token
	if maxChars <= 0 {
		maxChars = 2000
	}

	// Split into sections by headings.
	sections := splitByHeadings(text)

	var chunks []Chunk
	for _, section := range sections {
		if len(section) <= maxChars {
			c := makeChunk(section, len(chunks))
			if c.Text != "" {
				chunks = append(chunks, c)
			}
			continue
		}

		// Section too large: split by paragraphs.
		paragraphs := strings.Split(section, "\n\n")
		var buf strings.Builder
		for _, para := range paragraphs {
			para = strings.TrimSpace(para)
			if para == "" {
				continue
			}

			// If adding this paragraph would exceed max, flush current buffer.
			if buf.Len()+len(para)+2 > maxChars && buf.Len() > 0 {
				c := makeChunk(buf.String(), len(chunks))
				if c.Text != "" {
					chunks = append(chunks, c)
				}

				// Keep overlap from the end of the flushed text.
				overlap := extractOverlap(buf.String(), cfg.Overlap)
				buf.Reset()
				if overlap != "" {
					buf.WriteString(overlap)
					buf.WriteString("\n\n")
				}
			}

			buf.WriteString(para)
			buf.WriteString("\n\n")
		}

		if buf.Len() > 0 {
			c := makeChunk(buf.String(), len(chunks))
			if c.Text != "" {
				chunks = append(chunks, c)
			}
		}
	}

	return chunks
}

// splitByHeadings splits markdown into sections by ## headings.
func splitByHeadings(text string) []string {
	lines := strings.Split(text, "\n")
	var sections []string
	var current strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") && current.Len() > 0 {
			sections = append(sections, current.String())
			current.Reset()
		}
		current.WriteString(line)
		current.WriteString("\n")
	}

	if current.Len() > 0 {
		sections = append(sections, current.String())
	}

	return sections
}

// makeChunk creates a Chunk from text with a SHA-256 hash.
func makeChunk(text string, index int) Chunk {
	text = strings.TrimSpace(text)
	if text == "" {
		return Chunk{}
	}

	hash := sha256.Sum256([]byte(text))
	return Chunk{
		Index: index,
		Text:  text,
		Hash:  hex.EncodeToString(hash[:]),
	}
}

// extractOverlap returns the last `n` characters of text for chunk overlap.
func extractOverlap(text string, n int) string {
	if n <= 0 || len(text) <= n {
		return ""
	}

	// Find a word boundary near the overlap point.
	start := len(text) - n
	for start > 0 && start < len(text) {
		r, _ := utf8.DecodeRuneInString(text[start:])
		if r == ' ' || r == '\n' {
			start++
			break
		}
		start++
	}

	if start >= len(text) {
		return ""
	}
	return strings.TrimSpace(text[start:])
}

// IndexDirectory scans a directory for .md files and chunks them.
// Returns a map of fileID → []Chunk. The fileID is the relative path.
func IndexDirectory(dir string, cfg ChunkConfig) (map[string][]Chunk, error) {
	result := make(map[string][]Chunk)

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		text := strings.TrimSpace(string(data))
		if text == "" {
			continue
		}

		chunks := ChunkMarkdown(text, cfg)
		for i := range chunks {
			chunks[i].FileID = entry.Name()
		}

		if len(chunks) > 0 {
			result[entry.Name()] = chunks
		}
	}

	// Also index MEMORY.md from parent dir if it exists.
	memFile := filepath.Join(dir, "..", "MEMORY.md")
	if data, err := os.ReadFile(memFile); err == nil {
		text := strings.TrimSpace(string(data))
		if text != "" {
			chunks := ChunkMarkdown(text, cfg)
			for i := range chunks {
				chunks[i].FileID = "MEMORY.md"
			}
			if len(chunks) > 0 {
				result["MEMORY.md"] = chunks
			}
		}
	}

	return result, nil
}

// FileHash computes the SHA-256 hash of a file's content.
func FileHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}
