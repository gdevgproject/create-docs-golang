package tokenizer

import (
	"testing"
)

func TestTokenizer_EstimateTokensHeuristic(t *testing.T) {
	tok := NewTokenizer(t.TempDir())

	tests := []struct {
		input    string
		minCount int
	}{
		{"", 0},
		{"Hello, world!", 3},
		{"func main() {\n\tfmt.Println(\"Hello\")\n}", 8},
	}

	for _, tt := range tests {
		got := tok.EstimateTokensHeuristic(tt.input)
		if tt.input == "" && got != 0 {
			t.Errorf("EstimateTokensHeuristic(%q) = %d; want 0", tt.input, got)
		}
		if tt.input != "" && got < tt.minCount {
			t.Errorf("EstimateTokensHeuristic(%q) = %d; want >= %d", tt.input, got, tt.minCount)
		}
	}
}

func TestTokenizer_CountTokensExact(t *testing.T) {
	tok := NewTokenizer(t.TempDir())
	mode := tok.Mode()

	if mode != "exact" {
		t.Logf("tokenizer operating in mode: %s", mode)
	}

	count := tok.CountTokens("package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello world\")\n}")
	if count <= 0 {
		t.Errorf("expected count > 0, got %d", count)
	}
}

func TestTokenizer_HeuristicFallbackForced(t *testing.T) {
	tok := NewTokenizer(t.TempDir())
	tok.mode = "estimate"
	tok.t = nil

	count := tok.CountTokens("hello world")
	if count <= 0 {
		t.Errorf("expected count > 0, got %d", count)
	}
}
