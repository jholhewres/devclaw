package memory

import (
	"os"
	"reflect"
	"testing"
)

// testdata/mlm_tokenizer.json is the HF tokenizer.json for
// paraphrase-multilingual-MiniLM-L12-v2 (~9MB, gitignored). When absent the
// test skips. Expected ids are ground truth from the Python `tokenizers` lib.
func TestUnigramTokenizerParity(t *testing.T) {
	const path = "testdata/mlm_tokenizer.json"
	if _, err := os.Stat(path); err != nil {
		t.Skip("testdata/mlm_tokenizer.json absent; download it to run parity test")
	}
	tk, err := NewUnigramTokenizer(path, 128)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	cases := []struct {
		text string
		want []int64
	}{
		{"me vê os códigos dos bilhetes", []int64{0, 163, 6985, 362, 55845, 7, 655, 2193, 1448, 90, 2}},
		{"tem alguma informações sobre meus vôos?", []int64{0, 1790, 45298, 22979, 1028, 26957, 11181, 232, 32, 2}},
		{"Viagem Florianópolis 23-26/06/2026 — Encontro Conexão Estratégica HostGator", []int64{0, 582, 23086, 62912, 66, 161808, 1105, 51372, 34340, 74957, 4046, 292, 151764, 1657, 3355, 3680, 111576, 2312, 65995, 100932, 724, 4597, 2}},
		{"Localizador: WGADGP", []int64{0, 24172, 25490, 42, 12, 601, 14849, 397, 32566, 2}},
		{"o que rolou na sexta", []int64{0, 36, 41, 6136, 796, 24, 46236, 2}},
		{"lista de compras casaco meias", []int64{0, 5875, 8, 85466, 2349, 587, 5362, 162, 2}},
		{"hello world", []int64{0, 33600, 31, 8999, 2}},
		{"São Paulo é ótimo!", []int64{0, 11182, 14281, 393, 115841, 38, 2}},
	}
	for _, c := range cases {
		ids, mask, typeIDs := tk.Tokenize(c.text)
		var got []int64
		for i, m := range mask {
			if m == 1 {
				got = append(got, ids[i])
			}
			if typeIDs[i] != 0 {
				t.Errorf("%q: token_type_ids[%d]=%d, want 0", c.text, i, typeIDs[i])
			}
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q:\n  got  %v\n  want %v", c.text, got, c.want)
		}
	}
}
