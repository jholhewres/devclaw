// Package memory — tokenizer_unigram.go implements a pure-Go SentencePiece
// Unigram tokenizer for the multilingual MiniLM model
// (paraphrase-multilingual-MiniLM-L12-v2). It loads the HuggingFace
// tokenizer.json (token id = vocab array index), normalizes with NFKC +
// lowercase, applies Metaspace pre-tokenization (▁ per whitespace-split word),
// and runs Viterbi over the unigram scores. Output matches the reference
// `tokenizers` library on PT/EN text.
package memory

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const metaspace = "▁" // ▁ — SentencePiece space marker

type unigramEntry struct {
	id    int64
	score float64
}

// UnigramTokenizer tokenizes text for SentencePiece-Unigram BERT models.
type UnigramTokenizer struct {
	vocab         map[string]unigramEntry
	unkID         int64
	clsID         int64 // <s>
	sepID         int64 // </s>
	padID         int64 // <pad>
	maxLen        int
	maxPieceRunes int
	unkScore      float64
}

// tokenizerJSON is the subset of HuggingFace tokenizer.json we need.
type tokenizerJSON struct {
	Model struct {
		Type  string              `json:"type"`
		UnkID *int                `json:"unk_id"`
		Vocab [][]json.RawMessage `json:"vocab"`
	} `json:"model"`
}

// NewUnigramTokenizer loads a HuggingFace tokenizer.json (Unigram model).
func NewUnigramTokenizer(tokenizerPath string, maxLen int) (*UnigramTokenizer, error) {
	data, err := os.ReadFile(tokenizerPath)
	if err != nil {
		return nil, err
	}
	var tj tokenizerJSON
	if err := json.Unmarshal(data, &tj); err != nil {
		return nil, fmt.Errorf("parse tokenizer.json: %w", err)
	}
	if tj.Model.Type != "Unigram" {
		return nil, fmt.Errorf("unsupported tokenizer model type %q (want Unigram)", tj.Model.Type)
	}
	if len(tj.Model.Vocab) == 0 {
		return nil, fmt.Errorf("tokenizer.json has empty vocab")
	}

	t := &UnigramTokenizer{
		vocab:  make(map[string]unigramEntry, len(tj.Model.Vocab)),
		maxLen: maxLen,
		unkID:  3, // default; refined below from <unk>
	}
	minScore := math.MaxFloat64
	for i, entry := range tj.Model.Vocab {
		if len(entry) != 2 {
			return nil, fmt.Errorf("vocab entry %d malformed", i)
		}
		var piece string
		if err := json.Unmarshal(entry[0], &piece); err != nil {
			return nil, fmt.Errorf("vocab piece %d: %w", i, err)
		}
		var score float64
		if err := json.Unmarshal(entry[1], &score); err != nil {
			return nil, fmt.Errorf("vocab score %d: %w", i, err)
		}
		id := int64(i)
		t.vocab[piece] = unigramEntry{id: id, score: score}
		if score < minScore {
			minScore = score
		}
		if n := len([]rune(piece)); n > t.maxPieceRunes {
			t.maxPieceRunes = n
		}
		switch piece {
		case "<s>":
			t.clsID = id
		case "</s>":
			t.sepID = id
		case "<pad>":
			t.padID = id
		case "<unk>":
			t.unkID = id
		}
	}
	if tj.Model.UnkID != nil {
		t.unkID = int64(*tj.Model.UnkID)
	}
	// Cap piece length to keep Viterbi inner loop bounded; pieces longer than
	// this are vanishingly rare and never optimal for short memory text.
	if t.maxPieceRunes > 48 {
		t.maxPieceRunes = 48
	}
	// Reference (HF/sentencepiece) unk penalty: min score minus a fixed margin.
	t.unkScore = minScore - 10.0
	return t, nil
}

// Tokenize encodes text into padded input_ids, attention_mask, and
// token_type_ids (all zeros) of length maxLen. Signature matches
// WordPieceTokenizer so the two are interchangeable behind the embedder.
func (t *UnigramTokenizer) Tokenize(text string) (inputIDs, attentionMask, tokenTypeIDs []int64) {
	// The fast tokenizer's normalizer is Precompiled (≈ NFKC); it does NOT
	// lowercase (do_lower_case applies only to the legacy slow tokenizer).
	text = norm.NFKC.String(text)

	tokens := make([]int64, 0, t.maxLen)
	tokens = append(tokens, t.clsID)
	for _, word := range strings.FieldsFunc(text, unicode.IsSpace) {
		for _, id := range t.encodeWord(metaspace + word) {
			tokens = append(tokens, id)
		}
		if len(tokens) >= t.maxLen-1 {
			tokens = tokens[:t.maxLen-1]
			break
		}
	}
	tokens = append(tokens, t.sepID)

	inputIDs = make([]int64, t.maxLen)
	attentionMask = make([]int64, t.maxLen)
	tokenTypeIDs = make([]int64, t.maxLen)
	for i, id := range tokens {
		inputIDs[i] = id
		attentionMask[i] = 1
	}
	for i := len(tokens); i < t.maxLen; i++ {
		inputIDs[i] = t.padID
	}
	return inputIDs, attentionMask, tokenTypeIDs
}

// encodeWord runs Viterbi over the unigram scores for a single pre-token,
// falling back to <unk> for runes no piece covers (consecutive unks merged).
func (t *UnigramTokenizer) encodeWord(word string) []int64 {
	r := []rune(word)
	n := len(r)
	if n == 0 {
		return nil
	}
	const negInf = -math.MaxFloat64
	dp := make([]float64, n+1)
	prev := make([]int, n+1)
	pid := make([]int64, n+1)
	for i := 1; i <= n; i++ {
		dp[i] = negInf
	}
	for i := 0; i < n; i++ {
		if dp[i] == negInf {
			continue
		}
		maxL := t.maxPieceRunes
		if maxL > n-i {
			maxL = n - i
		}
		for l := 1; l <= maxL; l++ {
			if e, ok := t.vocab[string(r[i:i+l])]; ok {
				if s := dp[i] + e.score; s > dp[i+l] {
					dp[i+l] = s
					prev[i+l] = i
					pid[i+l] = e.id
				}
			}
		}
		// Single-rune <unk> fallback guarantees full coverage.
		if s := dp[i] + t.unkScore; s > dp[i+1] {
			dp[i+1] = s
			prev[i+1] = i
			pid[i+1] = t.unkID
		}
	}

	// Backtrack.
	ids := make([]int64, 0, n)
	for i := n; i > 0; i = prev[i] {
		ids = append(ids, pid[i])
	}
	for l, rgt := 0, len(ids)-1; l < rgt; l, rgt = l+1, rgt-1 {
		ids[l], ids[rgt] = ids[rgt], ids[l]
	}

	// Merge consecutive <unk> tokens into one (reference behavior).
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id == t.unkID && len(out) > 0 && out[len(out)-1] == t.unkID {
			continue
		}
		out = append(out, id)
	}
	return out
}
