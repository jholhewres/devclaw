package memory

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestMeanPool(t *testing.T) {
	// 2 tokens, 3 dims. Token 0 active, token 1 masked.
	raw := []float32{1, 2, 3, 10, 20, 30}
	mask := []int64{1, 0}
	result := meanPool(raw, mask, 2, 3)

	expected := []float32{1, 2, 3} // only token 0
	for i, v := range result {
		if math.Abs(float64(v-expected[i])) > 1e-6 {
			t.Errorf("meanPool[%d] = %f, want %f", i, v, expected[i])
		}
	}
}

func TestMeanPool_AllActive(t *testing.T) {
	// 2 tokens, 3 dims. Both active → average.
	raw := []float32{1, 2, 3, 3, 4, 5}
	mask := []int64{1, 1}
	result := meanPool(raw, mask, 2, 3)

	expected := []float32{2, 3, 4} // average
	for i, v := range result {
		if math.Abs(float64(v-expected[i])) > 1e-6 {
			t.Errorf("meanPool[%d] = %f, want %f", i, v, expected[i])
		}
	}
}

func TestL2Normalize(t *testing.T) {
	vec := []float32{3, 4} // norm = 5
	l2Normalize(vec)

	if math.Abs(float64(vec[0])-0.6) > 1e-6 {
		t.Errorf("l2Normalize[0] = %f, want 0.6", vec[0])
	}
	if math.Abs(float64(vec[1])-0.8) > 1e-6 {
		t.Errorf("l2Normalize[1] = %f, want 0.8", vec[1])
	}

	// Verify unit length.
	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	if math.Abs(norm-1.0) > 1e-6 {
		t.Errorf("norm should be 1.0, got %f", norm)
	}
}

func TestL2Normalize_ZeroVector(t *testing.T) {
	vec := []float32{0, 0, 0}
	l2Normalize(vec) // should not panic or NaN
	for i, v := range vec {
		if v != 0 {
			t.Errorf("l2Normalize zero vec[%d] = %f, want 0", i, v)
		}
	}
}

func TestWordPieceTokenizer_BasicText(t *testing.T) {
	// Create a minimal vocab file for testing.
	dir := t.TempDir()
	vocabPath := filepath.Join(dir, "vocab.txt")

	// Minimal vocab: [PAD]=0, [UNK]=1, [CLS]=2, [SEP]=3, hello=4, world=5, ##ing=6
	vocab := "[PAD]\n[UNK]\n[CLS]\n[SEP]\nhello\nworld\n##ing\n"
	if err := os.WriteFile(vocabPath, []byte(vocab), 0o644); err != nil {
		t.Fatal(err)
	}

	tok, err := NewWordPieceTokenizer(vocabPath, 8)
	if err != nil {
		t.Fatal(err)
	}

	ids, mask, typeIDs := tok.Tokenize("Hello World")

	// Expected: [CLS]=2, hello=4, world=5, [SEP]=3, [PAD]=0, [PAD]=0, [PAD]=0, [PAD]=0
	if ids[0] != 2 { // [CLS]
		t.Errorf("ids[0] = %d, want 2 ([CLS])", ids[0])
	}
	if ids[1] != 4 { // hello
		t.Errorf("ids[1] = %d, want 4 (hello)", ids[1])
	}
	if ids[2] != 5 { // world
		t.Errorf("ids[2] = %d, want 5 (world)", ids[2])
	}
	if ids[3] != 3 { // [SEP]
		t.Errorf("ids[3] = %d, want 3 ([SEP])", ids[3])
	}
	if ids[4] != 0 { // [PAD]
		t.Errorf("ids[4] = %d, want 0 ([PAD])", ids[4])
	}

	// Attention mask: 4 real tokens, 4 padding.
	for i := 0; i < 4; i++ {
		if mask[i] != 1 {
			t.Errorf("mask[%d] = %d, want 1", i, mask[i])
		}
	}
	for i := 4; i < 8; i++ {
		if mask[i] != 0 {
			t.Errorf("mask[%d] = %d, want 0", i, mask[i])
		}
	}

	// Token type IDs should all be 0.
	for i := 0; i < 8; i++ {
		if typeIDs[i] != 0 {
			t.Errorf("typeIDs[%d] = %d, want 0", i, typeIDs[i])
		}
	}
}

func TestSplitOnPunctuation(t *testing.T) {
	words := splitOnPunctuation("hello, world! foo-bar")
	expected := []string{"hello", ",", "world", "!", "foo", "-", "bar"}
	if len(words) != len(expected) {
		t.Fatalf("got %v, want %v", words, expected)
	}
	for i, w := range words {
		if w != expected[i] {
			t.Errorf("words[%d] = %q, want %q", i, w, expected[i])
		}
	}
}

func TestResolveONNXPaths(t *testing.T) {
	paths, err := resolveONNXPaths()
	if err != nil {
		t.Fatal(err)
	}
	if paths.RuntimeLib == "" || paths.ModelFile == "" || paths.VocabFile == "" {
		t.Error("paths should not be empty")
	}
}
