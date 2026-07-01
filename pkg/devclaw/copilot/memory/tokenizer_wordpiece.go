// Package memory — tokenizer_wordpiece.go implements a pure-Go WordPiece
// tokenizer for BERT/MiniLM models. No external dependencies.
//
// The tokenizer loads a vocab.txt file (one token per line, line number = ID),
// lowercases input, splits on whitespace and punctuation, and greedily matches
// the longest vocab prefix for each word. Unknown sub-tokens get [UNK].
package memory

import (
	"bufio"
	"os"
	"strings"
	"unicode"
)

// WordPieceTokenizer tokenizes text for BERT-family models.
type WordPieceTokenizer struct {
	vocab   map[string]int32 // token → ID
	unkID   int32
	clsID   int32
	sepID   int32
	padID   int32
	maxLen  int
}

// NewWordPieceTokenizer loads a vocab.txt file and returns a tokenizer.
func NewWordPieceTokenizer(vocabPath string, maxLen int) (*WordPieceTokenizer, error) {
	f, err := os.Open(vocabPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	vocab := make(map[string]int32)
	scanner := bufio.NewScanner(f)
	var id int32
	for scanner.Scan() {
		token := scanner.Text()
		vocab[token] = id
		id++
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if maxLen <= 0 {
		maxLen = 128
	}

	return &WordPieceTokenizer{
		vocab:  vocab,
		unkID:  vocab["[UNK]"],
		clsID:  vocab["[CLS]"],
		sepID:  vocab["[SEP]"],
		padID:  vocab["[PAD]"],
		maxLen: maxLen,
	}, nil
}

// Tokenize converts text into token IDs, attention mask, and token type IDs.
// Output is padded/truncated to maxLen.
func (t *WordPieceTokenizer) Tokenize(text string) (inputIDs, attentionMask, tokenTypeIDs []int64) {
	// Lowercase and basic cleanup.
	text = strings.ToLower(strings.TrimSpace(text))

	// Split into words (whitespace + punctuation boundaries).
	words := splitOnPunctuation(text)

	// WordPiece encode each word.
	var tokens []int32
	tokens = append(tokens, t.clsID)
	for _, word := range words {
		word = strings.TrimSpace(word)
		if word == "" {
			continue
		}
		pieces := t.wordPieceEncode(word)
		tokens = append(tokens, pieces...)
		// Reserve space for [SEP] + padding.
		if len(tokens) >= t.maxLen-1 {
			tokens = tokens[:t.maxLen-1]
			break
		}
	}
	tokens = append(tokens, t.sepID)

	seqLen := len(tokens)

	// Pad to maxLen.
	inputIDs = make([]int64, t.maxLen)
	attentionMask = make([]int64, t.maxLen)
	tokenTypeIDs = make([]int64, t.maxLen)

	for i := 0; i < seqLen; i++ {
		inputIDs[i] = int64(tokens[i])
		attentionMask[i] = 1
	}
	for i := seqLen; i < t.maxLen; i++ {
		inputIDs[i] = int64(t.padID)
	}

	return inputIDs, attentionMask, tokenTypeIDs
}

// wordPieceEncode encodes a single word into WordPiece token IDs.
func (t *WordPieceTokenizer) wordPieceEncode(word string) []int32 {
	if _, ok := t.vocab[word]; ok {
		return []int32{t.vocab[word]}
	}

	var tokens []int32
	start := 0
	for start < len(word) {
		end := len(word)
		found := false
		for end > start {
			sub := word[start:end]
			if start > 0 {
				sub = "##" + sub
			}
			if id, ok := t.vocab[sub]; ok {
				tokens = append(tokens, id)
				found = true
				start = end
				break
			}
			end--
		}
		if !found {
			tokens = append(tokens, t.unkID)
			start++
		}
	}
	return tokens
}

// splitOnPunctuation splits text on whitespace and punctuation boundaries,
// keeping punctuation as separate tokens.
func splitOnPunctuation(text string) []string {
	var words []string
	var current strings.Builder
	for _, r := range text {
		if unicode.IsSpace(r) {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
			continue
		}
		if unicode.IsPunct(r) || unicode.IsSymbol(r) {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
			words = append(words, string(r))
			continue
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}
	return words
}
